package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/truefix/multicaster/internal/api"
	"github.com/truefix/multicaster/internal/config"
	"github.com/truefix/multicaster/internal/engine"
	rtmpingest "github.com/truefix/multicaster/internal/ingest/rtmp"
	"github.com/truefix/multicaster/internal/updates"
)

//go:embed all:web
var webFS embed.FS

func main() {
	if err := run(); err != nil {
		slog.Error("multicaster stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	pathFlag := flag.String("config", "", "optional configuration path (default: OS user configuration directory)")
	version := flag.Bool("version", false, "print build version")
	health := flag.Bool("healthcheck", false, "check the running server")
	reset := flag.Bool("reset-password", false, "reset dashboard password and open the create-password page")
	noBrowser := flag.Bool("no-browser", false, "print the setup URL without opening a browser")
	flag.Parse()
	if *version {
		fmt.Println(updates.Version)
		return nil
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	resolved, err := config.ResolvePath(*pathFlag)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(resolved), 0700); err != nil {
		return err
	}
	lock := flock.New(resolved + ".lock")
	if !*health {
		locked, e := lock.TryLock()
		if e != nil {
			return e
		}
		if !locked {
			if !*reset {
				return fmt.Errorf("another Multicaster instance is using %s", resolved)
			}
			c, _, _, e := config.LoadOrCreate(resolved)
			if e != nil {
				return e
			}
			setupURL, e := resetRunning(c)
			if e != nil {
				return fmt.Errorf("could not reset the running instance: %w", e)
			}
			fmt.Println("Create your new password:", setupURL)
			if !*noBrowser {
				openBrowser(setupURL)
			}
			return nil
		}
		defer lock.Close()
	}
	c, path, created, err := config.LoadOrCreate(*pathFlag)
	if err != nil {
		return err
	}
	address, err := localURL(c.Server.HTTPListen)
	if err != nil {
		return err
	}
	if *health {
		client := http.Client{Timeout: 3 * time.Second}
		res, e := client.Get(address + "/api/v1/health")
		if e != nil {
			return e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("health endpoint returned %d", res.StatusCode)
		}
		return nil
	}
	if *reset {
		token, e := config.NewSecret()
		if e != nil {
			return e
		}
		c.Auth.PasswordHash = ""
		c.Auth.SetupToken = token
		if e = c.Save(path); e != nil {
			return e
		}
	}
	if created {
		logger.Info("created configuration", "path", path)
	}
	httpListener, err := net.Listen("tcp", c.Server.HTTPListen)
	if err != nil {
		return fmt.Errorf("HTTP listener: %w", err)
	}
	defer httpListener.Close()
	rtmpListener, err := net.Listen("tcp", c.Server.RTMPListen)
	if err != nil {
		return fmt.Errorf("RTMP listener: %w", err)
	}
	defer rtmpListener.Close()
	eng := engine.New(c, path, logger)
	apiServer := api.New(eng)
	root, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/hls/", http.StripPrefix("/hls/", &eng.HLS))
	mux.Handle("/", apiServer.Router(http.FileServer(http.FS(root))))
	web := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	webDone := make(chan struct{})
	go func() {
		defer close(webDone)
		listener := httpListener
		for ctx.Err() == nil {
			e := web.Serve(listener)
			if e == nil || errors.Is(e, http.ErrServerClosed) || ctx.Err() != nil {
				return
			}
			logger.Error("dashboard listener failed; relay continues while dashboard retries", "error", e)
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				listener, e = net.Listen("tcp", c.Server.HTTPListen)
				if e == nil {
					break
				}
			}
		}
	}()
	ingestDone := make(chan error, 1)
	go func() { ingestDone <- rtmpingest.Run(ctx, rtmpListener, eng, logger) }()
	if c.Auth.PasswordHash == "" {
		setupURL := address + "/app/#setup=" + url.QueryEscape(c.Auth.SetupToken)
		fmt.Println("Create your dashboard password:", setupURL)
		if *reset && !*noBrowser {
			openBrowser(setupURL)
		}
	}
	logger.Info("multicaster ready", "dashboard", address+"/app/", "rtmp", rtmpListener.Addr(), "config", path)
	select {
	case <-ctx.Done():
	case err = <-ingestDone:
		cancel()
	}
	cancel()
	rtmpListener.Close()
	eng.OnPublishStop()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if e := web.Shutdown(shutdown); e != nil {
		web.Close()
	}
	<-webDone
	return err
}
func localURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", err
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}
func resetRunning(c *config.Config) (string, error) {
	base, err := localURL(c.Server.HTTPListen)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest("POST", base+"/api/v1/auth/reset", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+c.Auth.AdminToken)
	client := http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("reset endpoint returned %d", res.StatusCode)
	}
	var body struct {
		Token string `json:"setup_token"`
	}
	if err = json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	return base + "/app/#setup=" + url.QueryEscape(body.Token), nil
}
func openBrowser(address string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if command.Start() == nil {
		go command.Wait()
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"github.com/truefix/multicaster/internal/config"
	"github.com/truefix/multicaster/internal/engine"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordSetupResetAndSessionInvalidation(t *testing.T) {
	c := config.Default()
	c.Server.DataDir = t.TempDir()
	c.Auth = config.AuthConfig{AdminToken: "machine-key", StreamKey: "obs-key", SetupToken: "bootstrap"}
	path := filepath.Join(c.Server.DataDir, "config.yaml")
	e := engine.New(c, path, nil)
	server := httptest.NewServer(New(e).Router(nil))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	post := func(path string, data map[string]string, bearer bool) *http.Response {
		t.Helper()
		body, _ := json.Marshal(data)
		req, _ := http.NewRequest("POST", server.URL+path, bytes.NewReader(body))
		if bearer {
			req.Header.Set("Authorization", "Bearer machine-key")
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := post("/api/v1/auth/setup", map[string]string{"setup_token": "bootstrap", "password": "first-password"}, false)
	if res.StatusCode != 200 {
		t.Fatalf("setup: %d", res.StatusCode)
	}
	res.Body.Close()
	parsedURL, _ := url.Parse(server.URL)
	oldCookies := jar.Cookies(parsedURL)
	if e.Config().Auth.PasswordHash == "first-password" || !strings.HasPrefix(e.Config().Auth.PasswordHash, "$2") {
		t.Fatal("password was not hashed")
	}
	res, err := client.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatal("session did not authenticate")
	}
	res.Body.Close()
	res = post("/api/v1/auth/reset", nil, true)
	if res.StatusCode != 200 {
		t.Fatalf("reset: %d", res.StatusCode)
	}
	var reset struct {
		Token string `json:"setup_token"`
	}
	json.NewDecoder(res.Body).Decode(&reset)
	res.Body.Close()
	if reset.Token == "" {
		t.Fatal("reset code missing")
	}
	res, err = client.Get(server.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 401 {
		t.Fatal("old session survived password reset")
	}
	res.Body.Close()
	stolenOldSession, _ := http.NewRequest("GET", server.URL+"/api/v1/settings", nil)
	for _, cookie := range oldCookies {
		stolenOldSession.AddCookie(cookie)
	}
	replay, err := http.DefaultClient.Do(stolenOldSession)
	if err != nil {
		t.Fatal(err)
	}
	if replay.StatusCode != 401 {
		t.Fatal("replayed session survived password reset")
	}
	replay.Body.Close()
	res = post("/api/v1/auth/setup", map[string]string{"setup_token": reset.Token, "password": "new-password"}, false)
	if res.StatusCode != 200 {
		t.Fatalf("new password: %d", res.StatusCode)
	}
	res.Body.Close()
	loaded, _, _, err := config.LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.StreamKey != "obs-key" || len(loaded.Destinations) != 0 {
		t.Fatal("password reset changed streaming configuration")
	}
}

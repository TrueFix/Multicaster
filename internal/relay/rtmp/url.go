package rtmp

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Endpoint is parsed from a full RTMP publish URL (rtmp://host/app/streamKey).
type Endpoint struct {
	Host      string
	App       string
	StreamKey string
}

func ParseURL(raw string) (Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Endpoint{}, fmt.Errorf("empty RTMP URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Endpoint{}, err
	}
	if u.Scheme != "rtmp" && u.Scheme != "rtmps" {
		return Endpoint{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return Endpoint{}, fmt.Errorf("invalid RTMP host or URL")
	}
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "1935")
	}
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return Endpoint{}, fmt.Errorf("RTMP URL must be rtmp://host/app/streamKey")
	}
	app := parts[0]
	key := strings.Join(parts[1:], "/")
	if key == "" {
		return Endpoint{}, fmt.Errorf("stream key is required")
	}
	return Endpoint{Host: host, App: app, StreamKey: key}, nil
}

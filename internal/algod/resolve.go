package algod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Options struct{ URL, Token, TokenFile, DataDir string }

// Connection contains an admin credential. Never format this struct for display.
type Connection struct{ URL, Token string }

func (Connection) String() string   { return "algod connection (credentials hidden)" }
func (Connection) GoString() string { return "algod.Connection{credentials hidden}" }

type System interface {
	Getenv(string) string
	ReadFile(string) ([]byte, error)
	Inspect(context.Context, string) (string, error)
}
type OSSystem struct{}

func (OSSystem) Getenv(k string) string            { return os.Getenv(k) }
func (OSSystem) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }

// Inspect only reads bridge metadata. It never starts or execs a container.
func (OSSystem) Inspect(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .NetworkSettings.Networks}}", name).Output()
	if err != nil {
		return "", errors.New("docker inspect unavailable")
	}
	var networks map[string]struct {
		IPAddress string `json:"IPAddress"`
	}
	if json.Unmarshal(b, &networks) != nil {
		return "", errors.New("invalid docker network metadata")
	}
	n, ok := networks["bridge"]
	if !ok || net.ParseIP(n.IPAddress) == nil {
		return "", errors.New("container has no default bridge IP")
	}
	return n.IPAddress, nil
}
func Resolve(ctx context.Context, o Options, s System) (Connection, error) {
	if o.URL != "" || o.Token != "" || o.TokenFile != "" {
		if o.URL == "" || (o.Token == "" && o.TokenFile == "") {
			return Connection{}, errors.New("provide --algod-url and --token or --token-file together")
		}
		if o.Token != "" && o.TokenFile != "" {
			return Connection{}, errors.New("use only one of --token and --token-file")
		}
		token := o.Token
		if o.TokenFile != "" {
			b, err := s.ReadFile(o.TokenFile)
			if err != nil {
				return Connection{}, errors.New("cannot read token file")
			}
			token = string(b)
		}
		return connection(o.URL, token)
	}
	if u, t := s.Getenv("ALGOD_URL"), s.Getenv("ALGOD_TOKEN"); u != "" || t != "" {
		if u == "" || t == "" {
			return Connection{}, errors.New("set ALGOD_URL and ALGOD_TOKEN together")
		}
		return connection(u, t)
	}
	dir := o.DataDir
	if dir == "" {
		dir = s.Getenv("ALGORAND_DATA")
	}
	if dir != "" {
		return fromDir(dir, "", s)
	}
	const swarmDir = "/var/lib/voi/algod/data"
	for _, name := range []string{"voinetwork_algod", "voinetwork_relay", "voinetwork_developer", "voinetwork_archiver"} {
		ip, err := s.Inspect(ctx, name)
		if err == nil {
			return fromDir(swarmDir, ip, s)
		}
	}
	return Connection{}, errors.New("algod not found; provide connection flags, environment, or --data-dir")
}
func fromDir(dir, override string, s System) (Connection, error) {
	b, err := s.ReadFile(filepath.Join(dir, "algod.net"))
	if err != nil {
		return Connection{}, errors.New("cannot read algod.net")
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(string(b)))
	if err != nil || port == "" {
		return Connection{}, errors.New("algod.net must contain host:port")
	}
	if override != "" {
		host = override
	} else if host == "::" || host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	b, err = s.ReadFile(filepath.Join(dir, "algod.admin.token"))
	if err != nil {
		return Connection{}, errors.New("cannot read algod.admin.token; admin access is required")
	}
	return connection("http://"+net.JoinHostPort(host, port), string(b))
}
func connection(u, t string) (Connection, error) {
	if err := validateURL(u); err != nil {
		return Connection{}, err
	}
	t = strings.TrimSpace(t)
	if t == "" || strings.ContainsAny(t, "\r\n") {
		return Connection{}, errors.New("admin token is empty or invalid")
	}
	return Connection{URL: strings.TrimRight(u, "/"), Token: t}, nil
}
func validateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("algod URL must be an http(s) URL without credentials, query, or fragment")
	}
	return nil
}

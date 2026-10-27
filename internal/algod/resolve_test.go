package algod

import (
	"context"
	"errors"
	"testing"
)

type fakeSystem struct {
	env   map[string]string
	files map[string]string
	calls []string
}

func (f *fakeSystem) Getenv(k string) string { return f.env[k] }
func (f *fakeSystem) ReadFile(p string) ([]byte, error) {
	v, ok := f.files[p]
	if !ok {
		return nil, errors.New("missing")
	}
	return []byte(v), nil
}
func (f *fakeSystem) Inspect(_ context.Context, name string) (string, error) {
	f.calls = append(f.calls, name)
	if name != "voinetwork_relay" {
		return "", errors.New("not found")
	}
	return "172.17.0.2", nil
}
func TestResolutionOrder(t *testing.T) {
	s := &fakeSystem{env: map[string]string{"ALGOD_URL": "http://env:2", "ALGOD_TOKEN": "env-token", "ALGORAND_DATA": "/envdata"}, files: map[string]string{"/flagtoken": "file-token\n", "/data/algod.net": "[::]:8080\n", "/data/algod.admin.token": "data-token\n", "/envdata/algod.net": "127.0.0.1:3", "/envdata/algod.admin.token": "envdata-token", "/var/lib/voi/algod/data/algod.net": "[::]:8081", "/var/lib/voi/algod/data/algod.admin.token": "swarm-token"}}
	for _, tc := range []struct {
		name       string
		opts       Options
		url, token string
	}{
		{"flags", Options{URL: "http://flags:1", Token: "flag-token", DataDir: "/data"}, "http://flags:1", "flag-token"},
		{"token file", Options{URL: "http://flags:1", TokenFile: "/flagtoken"}, "http://flags:1", "file-token"},
		{"environment", Options{DataDir: "/data"}, "http://env:2", "env-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Resolve(context.Background(), tc.opts, s)
			if err != nil || c.URL != tc.url || c.Token != tc.token {
				t.Fatalf("resolution failed: %v", err)
			}
		})
	}
	delete(s.env, "ALGOD_URL")
	delete(s.env, "ALGOD_TOKEN")
	c, err := Resolve(context.Background(), Options{DataDir: "/data"}, s)
	if err != nil || c.URL != "http://127.0.0.1:8080" {
		t.Fatal(c.URL, err)
	}
	c, err = Resolve(context.Background(), Options{}, s)
	if err != nil || c.URL != "http://127.0.0.1:3" {
		t.Fatal(c.URL, err)
	}
	delete(s.env, "ALGORAND_DATA")
	c, err = Resolve(context.Background(), Options{}, s)
	if err != nil || c.URL != "http://172.17.0.2:8081" || len(s.calls) != 2 {
		t.Fatal(c.URL, s.calls, err)
	}
	if _, err = Resolve(context.Background(), Options{URL: "http://partial"}, s); err == nil {
		t.Fatal("partial flags must fail")
	}
	if _, err = Resolve(context.Background(), Options{URL: "http://x", Token: "a", TokenFile: "/flagtoken"}, s); err == nil {
		t.Fatal("conflicting credentials")
	}
}

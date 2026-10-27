package algod

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientIgnoresHTTPProxy(t *testing.T) {
	// ProxyFromEnvironment caches its first environment. Use a fresh process so
	// this test works regardless of which other HTTP tests ran first.
	if os.Getenv("VNODE_PROXY_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestClientIgnoresHTTPProxy$")
		cmd.Env = append(os.Environ(), "VNODE_PROXY_TEST=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("proxy regression: %v\n%s", err, out)
		}
		return
	}
	var direct, proxied atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		if r.Header.Get("X-Algo-API-Token") != "fake-admin" {
			t.Error("missing authentication")
		}
		if r.URL.Path == "/v2/status" {
			fmt.Fprint(w, `{"last-round":42}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	t.Setenv("REQUEST_METHOD", "")
	// Go bypasses proxies for loopback URLs. Map a synthetic hostname to the
	// local fixture so the regression exercises a proxy-eligible origin.
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "algod.test:80" {
			addr = srv.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = base
	defer func() { http.DefaultTransport = previous }()
	c, err := New(Connection{URL: "http://algod.test", Token: "fake-admin"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.http.Transport.(transport).base.(*http.Transport).CloseIdleConnections()
	_, statusErr := c.Status(context.Background())
	_, keysErr := c.Keys(context.Background())
	if statusErr != nil || keysErr != nil || direct.Load() != 2 || proxied.Load() != 0 {
		t.Fatalf("direct=%d proxy=%d status=%v keys=%v", direct.Load(), proxied.Load(), statusErr, keysErr)
	}
}

func TestWaitForBlockAndSafeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/status/wait-for-block-after/42" {
			fmt.Fprint(w, `{"last-round":43}`)
			return
		}
		http.Error(w, "sensitive-admin-token", http.StatusForbidden)
	}))
	defer srv.Close()
	c, err := New(Connection{URL: srv.URL, Token: "sensitive-admin-token"}, false)
	if err != nil {
		t.Fatal("client creation failed")
	}
	s, err := c.Wait(context.Background(), 42)
	if err != nil || s.LastRound != 43 {
		t.Fatal("long poll failed", err)
	}
	_, err = c.Keys(context.Background())
	if err == nil || strings.Contains(err.Error(), "sensitive-admin-token") {
		t.Fatal("unsafe or missing error")
	}
}

func TestAccountRejectsIncompleteOrMismatchedState(t *testing.T) {
	for _, body := range []string{`{}`, `{"address":"wrong","status":"Offline"}`, `{"address":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ","status":"unknown"}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c, err := New(Connection{URL: srv.URL, Token: "fake"}, false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Account(context.Background(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ")
		srv.Close()
		if err == nil {
			t.Fatal("accepted invalid on-chain state")
		}
	}
}

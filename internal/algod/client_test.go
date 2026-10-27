package algod

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

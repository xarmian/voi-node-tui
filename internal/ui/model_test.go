package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testAddress = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
func TestFakeAlgodGenerateQRDelete(t *testing.T) {
	var mu sync.Mutex
	generated, deleted, polls := false, false, 0
	pk := algod.ParticipationKey{Id: "new-key", Address: testAddress, Key: models.AccountParticipation{VoteParticipationKey: make([]byte, 32), SelectionParticipationKey: make([]byte, 32), VoteFirstValid: 100, VoteLastValid: 5000100, VoteKeyDilution: 2237}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("X-Algo-API-Token") != "fake-admin" {
			t.Error("missing authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v2/status":
			fmt.Fprint(w, `{"last-round":100,"time-since-last-round":1000000000}`)
		case r.URL.Path == "/versions":
			fmt.Fprint(w, `{"genesis_id":"voimain-v1.0","build":{"major":5,"minor":0,"build_number":4}}`)
		case r.URL.Path == "/v2/participation" && r.Method == "GET":
			if generated && !deleted {
				polls++
				if polls >= 2 {
					json.NewEncoder(w).Encode([]algod.ParticipationKey{pk})
					return
				}
			}
			fmt.Fprint(w, `[]`)
		case r.URL.Path == "/v2/accounts/"+testAddress:
			if r.URL.Query().Get("exclude") != "all" {
				t.Error("missing exclude=all")
			}
			json.NewEncoder(w).Encode(models.Account{Address: testAddress, Status: "Offline", Amount: 1234567})
		case r.Method == "POST" && r.URL.Path == "/v2/participation/generate/"+testAddress:
			if r.URL.Query().Get("first") != "100" || r.URL.Query().Get("last") != "5000100" || r.URL.Query().Has("dilution") {
				t.Error("incorrect generation range")
			}
			generated = true
			fmt.Fprint(w, `{}`)
		case r.Method == "DELETE" && r.URL.Path == "/v2/participation/new-key":
			deleted = true
			fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, err := algod.New(algod.Connection{URL: srv.URL, Token: "fake-admin"}, false)
	if err != nil {
		t.Fatal(err)
	}
	store := accounts.Store{Path: filepath.Join(t.TempDir(), "accounts.json")}
	if err = store.Add(testAddress, "test"); err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), c, store, false)
	m.applyRefresh(m.refresh()().(refreshMsg))
	m.Update(key("g"))
	if generated {
		t.Fatal("write before confirmation")
	}
	m.input.SetValue("5000000")
	m.Update(key("enter"))
	if generated {
		t.Fatal("write before y")
	}
	_, cmd := m.Update(key("y"))
	if cmd == nil {
		t.Fatal("no generate command")
	}
	m.Update(cmd())
	for i := 0; i < 2; i++ {
		m.Update(m.refresh()())
	}
	if m.pending != nil {
		t.Fatal("generation did not finish")
	}
	m.Update(key("q"))
	if !strings.Contains(m.View(), "voi://net:voimain-v1.0/"+testAddress) || m.qr == "" {
		t.Fatal("missing QR URI")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(key("d"))
	if deleted {
		t.Fatal("delete before confirmation")
	}
	_, cmd = m.Update(key("y"))
	if cmd == nil {
		t.Fatal("no delete command")
	}
	m.Update(cmd())
	m.Update(m.refresh()())
	if !deleted || len(m.keys) != 0 {
		t.Fatal("delete failed")
	}
}
func TestReadOnlyAndNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("read-only write")
		}
		http.Error(w, "failure", 500)
	}))
	defer srv.Close()
	c, err := algod.New(algod.Connection{URL: srv.URL, Token: "fake"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Generate(context.Background(), testAddress, 1, 10) == nil || c.Delete(context.Background(), "key") == nil {
		t.Fatal("client read-only guard")
	}
	m := New(context.Background(), c, accounts.Store{Path: filepath.Join(t.TempDir(), "watch.json")}, true)
	for _, s := range []string{"a", "g", "d", "r"} {
		_, cmd := m.Update(key(s))
		if cmd != nil || m.mode != "" {
			t.Fatalf("read-only %s", s)
		}
	}
	m.Update(m.refresh()())
	if !strings.Contains(m.View(), "failed") {
		t.Fatal(m.View())
	}
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("no retry scheduled")
	}
}

func TestOfflineQRHasNoKeys(t *testing.T) {
	c, err := algod.New(algod.Connection{URL: "http://127.0.0.1:1", Token: "fake"}, true)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), c, accounts.Store{Path: filepath.Join(t.TempDir(), "watch.json")}, true)
	m.rows = []row{{Account: models.Account{Address: testAddress, Status: "Offline"}}}
	m.Update(key("o"))
	want := "voi://net:voimain-v1.0/" + testAddress + "?type=keyreg"
	if m.uri != want || !strings.Contains(m.View(), "WARNING") {
		t.Fatal("offline URI or visible fallback warning missing")
	}
}

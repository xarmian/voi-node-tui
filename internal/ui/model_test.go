package ui

import (
	"bytes"

	"context"
	"encoding/json"
	"fmt"
	"github.com/algorand/go-algorand-sdk/v2/types"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
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
		case strings.HasPrefix(r.URL.Path, "/v2/blocks/"):
			http.NotFound(w, r)
		case r.URL.Path == "/v2/ledger/supply":
			fmt.Fprint(w, `{"current_round":100,"online-money":1000000000000,"total-money":10000000000000}`)
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
	// An unidentified network gets no URI at all.
	m.Update(key("o"))
	if m.uri != "" || !strings.Contains(m.message, "unknown network") {
		t.Fatalf("unknown network must be refused: uri %q message %q", m.uri, m.message)
	}
	m.version.GenesisID = "voimain-v1.0"
	m.Update(key("o"))
	if want := "voi://net:voimain-v1.0/" + testAddress + "?type=keyreg"; m.uri != want {
		t.Fatalf("offline URI %q; want %q", m.uri, want)
	}
	m.Update(key("esc"))
	m.version.GenesisID = "mainnet-v1.0"
	m.Update(key("o"))
	if want := "algorand://" + testAddress + "?type=keyreg"; m.uri != want {
		t.Fatalf("algorand offline URI %q; want %q", m.uri, want)
	}
	m.Update(key("f"))
	if strings.Contains(m.uri, "fee=") {
		t.Fatal("go-offline must not carry the incentive fee")
	}
}

func TestDeleteRechecksOnlineRegistration(t *testing.T) {
	pk := algod.ParticipationKey{Id: "guarded", Address: testAddress, Key: models.AccountParticipation{VoteParticipationKey: []byte{1}}}
	deletes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == "/v2/accounts/"+testAddress {
			json.NewEncoder(w).Encode(models.Account{Address: testAddress, Status: "Online", Participation: pk.Key})
			return
		}
		if r.Method == "DELETE" {
			deletes++
			fmt.Fprint(w, `{}`)
			return
		}
		t.Errorf("unexpected request: %s", r.URL.Path)
	}))
	defer srv.Close()
	c, err := algod.New(algod.Connection{URL: srv.URL, Token: "fake"}, false)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), c, accounts.Store{}, false)
	m.rows = []row{{Account: models.Account{Address: testAddress, Status: "Offline"}}}
	m.keys = []algod.ParticipationKey{pk}
	m.Update(key("d"))
	_, cmd := m.Update(key("y"))
	if cmd == nil {
		t.Fatal("no guarded delete command")
	}
	m.Update(cmd())
	if deletes != 0 || !strings.Contains(m.message, "full address") {
		t.Fatal("stale Offline snapshot bypassed guard")
	}
	m.rows[0].Account.Status = "Online"
	m.rows[0].Account.Participation = pk.Key
	m.Update(key("d"))
	m.input.SetValue("y")
	_, cmd = m.Update(key("enter"))
	if cmd != nil || deletes != 0 {
		t.Fatal("short confirmation accepted")
	}
	m.input.SetValue(testAddress)
	_, cmd = m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("full address refused")
	}
	m.Update(cmd())
	if deletes != 1 {
		t.Fatal("confirmed deletion did not happen")
	}
}

func TestAddRemoveAndGuardedDeleteOffer(t *testing.T) {
	c, err := algod.New(algod.Connection{URL: "http://127.0.0.1:1", Token: "fake"}, false)
	if err != nil {
		t.Fatal(err)
	}
	store := accounts.Store{Path: filepath.Join(t.TempDir(), "accounts.json")}
	m := New(context.Background(), c, store, false)
	m.Update(key("a"))
	m.input.SetValue(testAddress)
	m.Update(key("enter"))
	m.input.SetValue("colo")
	_, cmd := m.Update(key("enter"))
	m.Update(cmd())
	xs, err := store.Load()
	if err != nil || len(xs) != 1 || xs[0].Label != "colo" || m.mode != "offer-generate" {
		t.Fatal("account add and generation offer", xs, err)
	}
	m.Update(key("n"))
	pk := algod.ParticipationKey{Id: "registered", Address: testAddress, Key: models.AccountParticipation{VoteParticipationKey: []byte{1}}}
	m.rows = []row{{Account: models.Account{Address: testAddress, Status: "Online", Participation: pk.Key}}}
	m.keys = []algod.ParticipationKey{pk}
	m.Update(key("r"))
	_, cmd = m.Update(key("y"))
	m.Update(cmd())
	xs, err = store.Load()
	if err != nil || len(xs) != 0 || m.mode != "remove-keys" {
		t.Fatal("remove offer", err)
	}
	m.Update(key("y"))
	if m.mode != "delete-address" {
		t.Fatal("registered key removal bypassed guard")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != "" || len(m.removalKeys) != 0 {
		t.Fatal("delete queue not cancelled")
	}
}

func TestLiveRoundsEstimateAndRetry(t *testing.T) {
	c, err := algod.New(algod.Connection{URL: "http://127.0.0.1:1", Token: "fake"}, true)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), c, accounts.Store{}, true)
	now := time.Now()
	for i := 0; i < 4; i++ {
		m.Update(blockMsg{status: models.NodeStatus{LastRound: uint64(100 + i)}, at: now.Add(time.Duration(i) * 2 * time.Second)})
	}
	if m.average != 2*time.Second || len(m.samples) != 3 {
		t.Fatalf("block average %s (%d samples)", m.average, len(m.samples))
	}
	before := m.status.LastRound
	m.Update(refreshMsg{status: &models.NodeStatus{LastRound: 50}, at: now})
	if m.status.LastRound != before {
		t.Fatal("stale refresh moved round backwards")
	}
	m.Update(blockMsg{err: fmt.Errorf("temporary failure")})
	if !strings.Contains(m.View(), "temporary failure") {
		t.Fatal("missing network error")
	}
	_, cmd := m.Update(waitAgainMsg{})
	if cmd == nil {
		t.Fatal("long poll did not retry")
	}
}

func TestGenerationTimeout(t *testing.T) {
	m := New(context.Background(), nil, accounts.Store{}, false)
	start := time.Now()
	m.rows = []row{{Account: models.Account{Address: testAddress}}}
	m.statusAt = start
	m.refreshing = true // A stalled or failed refresh must not prevent expiry.
	m.pending = &generation{address: testAddress, started: start}
	m.Update(tickMsg(start.Add(20*time.Minute - time.Second)))
	if m.pending == nil {
		t.Fatal("generation expired early")
	}
	m.Update(tickMsg(start.Add(20 * time.Minute)))
	if m.pending != nil || !strings.Contains(m.message, "No new key appeared after 20m") || !strings.Contains(m.message, "algod logs") {
		t.Fatalf("generation did not time out clearly: %s", m.message)
	}
	m.Update(key("g"))
	if m.mode != "generate-range" {
		t.Fatal("generation remained blocked after timeout")
	}
}

func TestGenerationMatchesNewClampedKey(t *testing.T) {
	for _, tc := range []struct {
		name, id, address string
		first, last       uint64
		wantMatch         bool
	}{
		{"clamped range", "new", testAddress, 110, 180, true},
		{"same first different last", "new", testAddress, 100, 180, true},
		{"existing id", "old", testAddress, 100, 200, false},
		{"earlier first", "new", testAddress, 99, 200, false},
		{"different account", "new", "other", 100, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil, accounts.Store{}, false)
			m.rows = []row{{Account: models.Account{Address: testAddress}}}
			m.pending = &generation{address: testAddress, first: 100, last: 200, existing: map[string]bool{"old": true}, started: time.Now()}
			m.Update(refreshMsg{keysOK: true, keys: []algod.ParticipationKey{{Id: tc.id, Address: tc.address, Key: models.AccountParticipation{VoteFirstValid: tc.first, VoteLastValid: tc.last}}}})
			if (m.pending == nil) != tc.wantMatch {
				t.Fatalf("match = %t, want %t", m.pending == nil, tc.wantMatch)
			}
		})
	}
}

func TestAbandonGeneration(t *testing.T) {
	m := New(context.Background(), nil, accounts.Store{}, false)
	m.rows = []row{{Account: models.Account{Address: testAddress}}}
	m.statusAt = time.Now()
	m.pending = &generation{address: testAddress, started: time.Now()}
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	m.Update(key("?"))
	m.Update(esc)
	if m.mode != "" || m.pending == nil {
		t.Fatal("Esc should close help while keeping the wait")
	}
	for _, cancel := range []tea.KeyMsg{key("n"), key("enter"), esc} {
		m.Update(esc)
		if m.mode == "" || m.pending == nil || !strings.Contains(m.prompt, "y/N") || !strings.Contains(m.prompt, "does not cancel generation inside algod") {
			t.Fatal("Esc must ask before abandoning generation")
		}
		m.Update(cancel)
		if m.pending == nil || m.mode != "" {
			t.Fatal("default answer must keep waiting")
		}
	}
	m.Update(esc)
	_, cmd := m.Update(key("y"))
	if cmd != nil || m.pending != nil || m.mode != "" || !strings.Contains(m.message, "does not cancel generation inside algod") {
		t.Fatal("confirmed abandonment must clear only the UI wait")
	}
	// A POST reply arriving after abandonment must not claim polling continues.
	m.Update(operationMsg{action: "generate"})
	if strings.Contains(m.message, "polling") {
		t.Fatal("late POST reply restored the waiting message")
	}
	m.Update(key("g"))
	if m.mode != "generate-range" {
		t.Fatal("generation remained blocked after abandonment")
	}
	if !strings.Contains(helpText, "does not cancel generation inside algod") {
		t.Fatal("help must explain abandonment")
	}
}

func TestRedactDisguisesOnChainIdentifiers(t *testing.T) {
	r := newRedactor()
	vote := bytes.Repeat([]byte{7}, 32)
	sprf := bytes.Repeat([]byte{9}, 64)
	in := row{Account: models.Account{Address: testAddress, Amount: 5_000_000_000, Participation: models.AccountParticipation{VoteParticipationKey: vote, StateProofKey: sprf}}, Label: "home"}
	k := algod.ParticipationKey{Id: "REALID", Address: testAddress, Key: models.AccountParticipation{VoteParticipationKey: vote, StateProofKey: sprf}}
	out, ko := r.row(in), r.key(k)
	if out.Address == testAddress || ko.Address != out.Address {
		t.Fatalf("address not disguised consistently: %s / %s", out.Address, ko.Address)
	}
	if _, err := types.DecodeAddress(out.Address); err != nil {
		t.Fatalf("fake address is not a valid address: %v", err)
	}
	if out.Amount == in.Amount || out.Label == "home" || ko.Id == "REALID" {
		t.Fatal("balance, label or key id leaked")
	}
	if bytes.Equal(ko.Key.VoteParticipationKey, vote) || len(ko.Key.StateProofKey) != 64 {
		t.Fatal("participation keys not replaced at the same length")
	}
	if !accounts.Registered(out.Account, ko) {
		t.Fatal("redacted key no longer matches its redacted account")
	}
	k.LastBlockProposal = 1_000_000
	if got := r.key(k).LastBlockProposal; got >= k.LastBlockProposal || k.LastBlockProposal-got < 100 || k.LastBlockProposal-got >= 1500 {
		t.Fatalf("proposal age not lagged: %d -> %d", k.LastBlockProposal, got)
	}
	if r.round(100) == 100 || r.round(0) != 0 {
		t.Fatal("round offset wrong")
	}
}

func TestAlgorandIncentiveFeeToggle(t *testing.T) {
	m := New(context.Background(), nil, accounts.Store{}, true)
	m.version.GenesisID = "mainnet-v1.0"
	m.rows = []row{{Account: models.Account{Address: testAddress, Status: "Offline"}}}
	m.keys = []algod.ParticipationKey{{Id: "K", Address: testAddress, Key: models.AccountParticipation{
		VoteParticipationKey: bytes.Repeat([]byte{1}, 32), SelectionParticipationKey: bytes.Repeat([]byte{2}, 32),
		VoteFirstValid: 10, VoteLastValid: 20, VoteKeyDilution: 3}}}
	m.Update(key("enter"))
	if !strings.HasPrefix(m.uri, "algorand://"+testAddress+"?type=keyreg&votekey=") || strings.Contains(m.uri, "fee=") {
		t.Fatalf("default URI %q", m.uri)
	}
	m.Update(key("f"))
	if !strings.HasSuffix(m.uri, "&fee=2000000") || m.mode != "qr" {
		t.Fatalf("fee not added: %q mode %q", m.uri, m.mode)
	}
	m.Update(key("f"))
	if strings.Contains(m.uri, "fee=") {
		t.Fatalf("fee not removed: %q", m.uri)
	}
	m.Update(key("esc"))
	m.version.GenesisID = "voimain-v1.0"
	m.Update(key("enter"))
	m.Update(key("f"))
	if strings.Contains(m.uri, "fee=") {
		t.Fatal("Voi must never get the Algorand incentive fee")
	}
}

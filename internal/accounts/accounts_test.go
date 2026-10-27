package accounts

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"

	"github.com/xarmian/voi-node-tui/internal/algod"
)

const address = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ"

func TestAddressChecksum(t *testing.T) {
	for _, tc := range []struct {
		a  string
		ok bool
	}{{address, true}, {address[:57] + "A", false}, {"bad", false}} {
		if (ValidateAddress(tc.a) == nil) != tc.ok {
			t.Errorf("validation %q", tc.a)
		}
	}
}
func TestDeriveWarningsAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	k := algod.ParticipationKey{Id: "registered", Address: address, Key: models.AccountParticipation{VoteParticipationKey: []byte{1}, VoteLastValid: 200}}
	a := models.Account{Address: address, Status: "Online", Participation: models.AccountParticipation{VoteParticipationKey: []byte{1}}}
	d := Derive(a, []algod.ParticipationKey{k}, 100, 3*time.Second, now)
	if d.RegisteredID != "registered" || d.RoundsLeft != 100 || !d.ExpiresAt.Equal(now.Add(300*time.Second)) || !has(d.Warnings, WarningExpiring) {
		t.Fatalf("derived %+v", d)
	}
	a.Participation.VoteParticipationKey = []byte{2}
	d = Derive(a, []algod.ParticipationKey{k}, 100, time.Second, now)
	if !has(d.Warnings, WarningMissing) {
		t.Fatal(d)
	}
	a.Participation = k.Key
	d = Derive(a, []algod.ParticipationKey{k}, 201, time.Second, now)
	if !has(d.Warnings, WarningExpired) || d.RoundsLeft != 0 || !d.ExpiresAt.Equal(now.Add(-time.Second)) {
		t.Fatal(d)
	}
	a.Status = "Offline"
	d = Derive(a, []algod.ParticipationKey{k}, 201, time.Second, now)
	if len(d.Warnings) != 0 {
		t.Fatal(d)
	}
	d = Derive(a, nil, 100, 0, now)
	if !d.ExpiresAt.IsZero() {
		t.Fatal("invented block time")
	}
}
func has(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func TestDeleteGuard(t *testing.T) {
	k := algod.ParticipationKey{Address: address, Key: models.AccountParticipation{VoteParticipationKey: []byte{1}}}
	a := models.Account{Address: address, Status: "Online", Participation: k.Key}
	for _, tc := range []struct {
		confirm string
		ok      bool
	}{{"", false}, {"y", false}, {address, true}} {
		if (CanDelete(a, k, tc.confirm) == nil) != tc.ok {
			t.Errorf("confirmation %q", tc.confirm)
		}
	}
	a.Status = "Offline"
	if CanDelete(a, k, "y") != nil || CanDelete(a, k, "") == nil {
		t.Fatal("ordinary confirmation")
	}
	for _, tc := range []struct {
		name   string
		status string
		vote   []byte
	}{
		{"now offline", "Offline", k.Key.VoteParticipationKey},
		{"now registered to another key", "Online", []byte{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.Status = tc.status
			a.Participation.VoteParticipationKey = tc.vote
			if err := CanDelete(a, k, address); err != nil {
				t.Fatalf("full address refused after registration changed: %v", err)
			}
			if CanDelete(a, k, "wrong address") == nil {
				t.Fatal("incorrect address accepted")
			}
		})
	}
}
func TestWatchList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config", "accounts.json")
	s := Store{Path: p}
	if err := s.Add(address, "node"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(address, "renamed"); err != nil {
		t.Fatal(err)
	}
	xs, err := s.Load()
	if err != nil || len(xs) != 1 || xs[0].Label != "renamed" {
		t.Fatal(xs, err)
	}
	if err := s.Remove(address); err != nil {
		t.Fatal(err)
	}
	xs, err = s.Load()
	if err != nil || len(xs) != 0 {
		t.Fatal(xs, err)
	}
}

func TestUnrelatedExpiredKeyDoesNotWarn(t *testing.T) {
	a := models.Account{Address: address, Status: "Online", Participation: models.AccountParticipation{VoteParticipationKey: []byte{1}}}
	registered := algod.ParticipationKey{Id: "active", Address: address, Key: models.AccountParticipation{VoteParticipationKey: []byte{1}, VoteLastValid: 2000000}}
	old := algod.ParticipationKey{Id: "old", Address: address, Key: models.AccountParticipation{VoteParticipationKey: []byte{2}, VoteLastValid: 50}}
	d := Derive(a, []algod.ParticipationKey{old, registered}, 100, time.Second, time.Now())
	if len(d.Warnings) != 0 || d.RegisteredID != "active" {
		t.Fatalf("unrelated key affected registration: %+v", d)
	}
}

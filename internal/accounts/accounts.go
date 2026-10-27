// Package accounts owns the local watch list and derived participation state.
package accounts

import (
	"bytes"
	"errors"
	"math"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/xarmian/voi-node-tui/internal/algod"
)

const (
	WarningMissing  = "Online with no matching local key: votes are being missed"
	WarningExpiring = "Registered key expires within 14 days"
	WarningExpired  = "Online with a registered local key past its last-valid"
)

func ValidateAddress(s string) error {
	_, err := types.DecodeAddress(s)
	if err != nil {
		return errors.New("invalid address or checksum")
	}
	return nil
}
func Registered(a models.Account, k algod.ParticipationKey) bool {
	return a.Address == k.Address && len(a.Participation.VoteParticipationKey) > 0 && bytes.Equal(a.Participation.VoteParticipationKey, k.Key.VoteParticipationKey)
}

type State struct {
	RegisteredID            string
	First, Last, RoundsLeft uint64
	ExpiresAt               time.Time
	Warnings                []string
	LastVote, LastProposal  uint64
}

func Derive(a models.Account, keys []algod.ParticipationKey, round uint64, blockTime time.Duration, now time.Time) State {
	d := State{First: a.Participation.VoteFirstValid, Last: a.Participation.VoteLastValid}
	for _, k := range keys {
		if k.Address != a.Address {
			continue
		}
		d.LastVote = max(d.LastVote, k.LastVote)
		d.LastProposal = max(d.LastProposal, k.LastBlockProposal)
		if Registered(a, k) {
			d.RegisteredID = k.Id
			if d.Last == 0 {
				d.First = k.Key.VoteFirstValid
				d.Last = k.Key.VoteLastValid
			}
			if a.Status == "Online" && round > k.Key.VoteLastValid {
				d.Warnings = append(d.Warnings, WarningExpired)
			}
		}
	}
	if d.Last > round {
		d.RoundsLeft = d.Last - round
	}
	if blockTime > 0 && d.Last > 0 {
		delta := d.RoundsLeft
		if round > d.Last {
			delta = round - d.Last
		}
		if delta <= uint64(math.MaxInt64/int64(blockTime)) {
			estimate := time.Duration(delta) * blockTime
			if round > d.Last {
				estimate = -estimate
			}
			d.ExpiresAt = now.Add(estimate)
		}
	}
	if a.Status == "Online" {
		if d.RegisteredID == "" {
			d.Warnings = append(d.Warnings, WarningMissing)
		}
		if d.Last >= round && d.Last > 0 && blockTime > 0 && d.RoundsLeft <= uint64((14*24*time.Hour)/blockTime) {
			d.Warnings = append(d.Warnings, WarningExpiring)
		}
	}
	return d
}
func CanDelete(a models.Account, k algod.ParticipationKey, confirmation string) error {
	if a.Address != k.Address {
		return errors.New("account does not match key")
	}
	if a.Status == "Online" && Registered(a, k) {
		if confirmation != a.Address {
			return errors.New("node stops voting for this account: type the full address to confirm")
		}
		return nil
	}
	if confirmation != "y" && confirmation != "Y" && confirmation != a.Address {
		return errors.New("deletion requires y confirmation")
	}
	return nil
}

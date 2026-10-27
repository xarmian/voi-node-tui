package ui

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"

	"github.com/algorand/go-algorand-sdk/v2/types"

	"github.com/xarmian/voi-node-tui/internal/algod"
)

// redactor disguises live data for shareable screenshots. Everything that can be looked up
// on-chain is replaced consistently for the session: addresses, participation keys and key
// ids are keyed hashes of the real values, balances are scaled per account, and every round
// shown on screen is shifted by a fixed offset (so a block number or a keyreg's validity range
// can't be matched to a transaction). The live node is still read; nothing is written.
type redactor struct {
	salt   [32]byte
	offset uint64
}

func newRedactor() *redactor {
	r := &redactor{}
	_, _ = rand.Read(r.salt[:])
	var b [8]byte
	_, _ = rand.Read(b[:])
	r.offset = 1_000_000 + binary.LittleEndian.Uint64(b[:])%9_000_000
	return r
}

func (r *redactor) sum(tag string, v []byte) [32]byte {
	return sha256.Sum256(append(append(append([]byte(tag), r.salt[:]...), 0), v...))
}

func (r *redactor) address(a string) string {
	if a == "" {
		return a
	}
	return types.Address(r.sum("addr", []byte(a))).String()
}

func (r *redactor) bytes(tag string, v []byte) []byte {
	if len(v) == 0 {
		return v
	}
	if len(v) > 32 {
		h := sha512.Sum512(append(append([]byte(tag), r.salt[:]...), v...))
		return h[:min(len(v), 64)]
	}
	h := r.sum(tag, v)
	return h[:len(v)]
}

// amount scales a balance by a per-account factor in [0.45, 1.55) so neither exact balances
// nor their ratios match the chain.
func (r *redactor) amount(addr string, micro uint64) uint64 {
	h := r.sum("amt", []byte(addr))
	f := 0.45 + float64(binary.LittleEndian.Uint16(h[:2]))/65536*1.1
	return uint64(float64(micro) * f)
}

func (r *redactor) round(v uint64) uint64 {
	if v == 0 {
		return 0
	}
	return v + r.offset
}

func (r *redactor) row(x row) row {
	real := x.Address
	x.Account.Address = r.address(real)
	x.Account.Amount = r.amount(real, x.Account.Amount)
	x.Account.AmountWithoutPendingRewards = r.amount(real, x.Account.AmountWithoutPendingRewards)
	p := &x.Account.Participation
	p.VoteParticipationKey = r.bytes("vote", p.VoteParticipationKey)
	p.SelectionParticipationKey = r.bytes("sel", p.SelectionParticipationKey)
	p.StateProofKey = r.bytes("sprf", p.StateProofKey)
	if x.Label != "" {
		x.Label = "node"
	}
	return x
}

func (r *redactor) key(k algod.ParticipationKey) algod.ParticipationKey {
	k.Address = r.address(k.Address)
	id := r.sum("id", []byte(k.Id))
	k.Id = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])[:52]
	k.Key.VoteParticipationKey = r.bytes("vote", k.Key.VoteParticipationKey)
	k.Key.SelectionParticipationKey = r.bytes("sel", k.Key.SelectionParticipationKey)
	k.Key.StateProofKey = r.bytes("sprf", k.Key.StateProofKey)
	return k
}

// shown is the round as displayed: shifted when redacting, unchanged otherwise.
func (m *Model) shown(round uint64) uint64 {
	if m.redact == nil {
		return round
	}
	return m.redact.round(round)
}

// Redact switches the model into screenshot mode. It implies read-only.
func (m *Model) Redact() {
	m.redact = newRedactor()
	m.readOnly = true
}

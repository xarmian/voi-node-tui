// Package arc90 builds wallet-signable ARC-90 key registration URIs for Voi and Algorand.
package arc90

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// Network describes how a chain is presented and how its keyreg URIs are addressed.
type Network struct {
	Name     string // display name
	Unit     string // native asset ticker
	Wallet   string // what to scan the QR with
	Algorand bool   // Algorand networks support the incentive-eligibility fee
	prefix   string // scheme plus ARC-90 authority, up to the address
}

// Networks known by genesis ID. Voi wallets address the network by its genesis ID;
// Algorand follows ARC-90: MainNet omits the authority, other networks use an alias.
var networks = map[string]Network{
	"voimain-v1.0": {Name: "Voi MainNet", Unit: "VOI", Wallet: "Voi Wallet", prefix: "voi://net:voimain-v1.0/"},
	"voitest-v1.1": {Name: "Voi TestNet", Unit: "VOI", Wallet: "Voi Wallet", prefix: "voi://net:voitest-v1.1/"},
	"voibeta-v1.0": {Name: "Voi BetaNet", Unit: "VOI", Wallet: "Voi Wallet", prefix: "voi://net:voibeta-v1.0/"},
	"mainnet-v1.0": {Name: "Algorand MainNet", Unit: "ALGO", Wallet: "an ARC-90 wallet", Algorand: true, prefix: "algorand://"},
	"testnet-v1.0": {Name: "Algorand TestNet", Unit: "ALGO", Wallet: "an ARC-90 wallet", Algorand: true, prefix: "algorand://net:testnet/"},
	"betanet-v1.0": {Name: "Algorand BetaNet", Unit: "ALGO", Wallet: "an ARC-90 wallet", Algorand: true, prefix: "algorand://net:betanet/"},
}

// Lookup returns the network for a genesis ID. Unknown networks are not guessed: a key
// registered on the wrong chain takes that account online with keys no node is voting with.
func Lookup(genesis string) (Network, bool) {
	n, ok := networks[genesis]
	return n, ok
}

// IncentiveFee is the keyreg fee (microAlgos) that requests Algorand participation incentives.
const IncentiveFee = 2_000_000

// Build returns the keyreg URI. Query order matches Voi Swarm's show-partkey-qr; algod's
// standard-base64 key bytes are emitted base64url without padding, as ARC-90 requires.
// fee > 0 adds a static fee (Algorand networks only).
func Build(genesis, address string, key models.AccountParticipation, offline bool, fee uint64) (string, error) {
	if _, err := types.DecodeAddress(address); err != nil {
		return "", fmt.Errorf("invalid account checksum")
	}
	n, ok := Lookup(genesis)
	if !ok {
		return "", fmt.Errorf("unknown network %q: no keyreg URI is built for a network vnode can't identify", genesis)
	}
	if fee > 0 && !n.Algorand {
		return "", fmt.Errorf("a keyreg fee is only offered on Algorand networks")
	}
	uri := n.prefix + address + "?type=keyreg"
	if !offline {
		if len(key.VoteParticipationKey) != 32 || len(key.SelectionParticipationKey) != 32 || key.VoteLastValid < key.VoteFirstValid || key.VoteKeyDilution == 0 {
			return "", fmt.Errorf("invalid participation key or validity range")
		}
		if len(key.StateProofKey) != 0 && len(key.StateProofKey) != 64 {
			return "", fmt.Errorf("invalid state proof key length")
		}
		enc := base64.RawURLEncoding.EncodeToString
		uri += "&votekey=" + enc(key.VoteParticipationKey) + "&selkey=" + enc(key.SelectionParticipationKey)
		if len(key.StateProofKey) > 0 {
			uri += "&sprfkey=" + enc(key.StateProofKey)
		}
		uri += fmt.Sprintf("&votefst=%d&votelst=%d&votekd=%d", key.VoteFirstValid, key.VoteLastValid, key.VoteKeyDilution)
	}
	if fee > 0 {
		uri += fmt.Sprintf("&fee=%d", fee)
	}
	return uri, nil
}

// Package arc90 builds wallet-signable Voi key registration URIs.
package arc90

import (
	"encoding/base64"
	"fmt"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/v2/types"
)

// Build preserves the query order used by show-partkey-qr. SDK byte slices are
// decoded from algod's standard base64 JSON; RawURLEncoding makes them URL-safe.
func Build(genesis, address string, key models.AccountParticipation, offline bool) (uri, warning string, err error) {
	if _, err = types.DecodeAddress(address); err != nil {
		return "", "", fmt.Errorf("invalid account checksum")
	}
	switch genesis {
	case "voimain-v1.0", "voitest-v1.1", "voibeta-v1.0":
	default:
		genesis = "voimain-v1.0"
		warning = "WARNING: unknown genesis ID; assuming voimain-v1.0. Verify the network before signing."
	}
	uri = "voi://net:" + genesis + "/" + address + "?type=keyreg"
	if offline {
		return uri, warning, nil
	}
	if len(key.VoteParticipationKey) != 32 || len(key.SelectionParticipationKey) != 32 || key.VoteLastValid < key.VoteFirstValid || key.VoteKeyDilution == 0 {
		return "", warning, fmt.Errorf("invalid participation key or validity range")
	}
	if len(key.StateProofKey) != 0 && len(key.StateProofKey) != 64 {
		return "", warning, fmt.Errorf("invalid state proof key length")
	}
	enc := base64.RawURLEncoding.EncodeToString
	uri += "&votekey=" + enc(key.VoteParticipationKey) + "&selkey=" + enc(key.SelectionParticipationKey)
	if len(key.StateProofKey) > 0 {
		uri += "&sprfkey=" + enc(key.StateProofKey)
	}
	uri += fmt.Sprintf("&votefst=%d&votelst=%d&votekd=%d", key.VoteFirstValid, key.VoteLastValid, key.VoteKeyDilution)
	return uri, warning, nil
}

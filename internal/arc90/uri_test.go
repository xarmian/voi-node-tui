package arc90

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/xarmian/voi-node-tui/internal/algod"
)

func TestGolden(t *testing.T) {
	b, err := os.ReadFile("testdata/partkey.json")
	if err != nil {
		t.Fatal(err)
	}
	var k algod.ParticipationKey
	if err = json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	prefix := "voi://net:voitest-v1.1/" + k.Address + "?type=keyreg"
	suffix := "&votefst=12345&votelst=5012345&votekd=2237"
	keys := "&votekey=-_8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&selkey=__4AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for _, tc := range []struct {
		name    string
		proof   bool
		offline bool
		want    string
	}{
		{"with proof", true, false, prefix + keys + "&sprfkey=-_8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" + suffix},
		{"without proof", false, false, prefix + keys + suffix},
		{"offline", false, true, prefix},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := k.Key
			if !tc.proof {
				p.StateProofKey = nil
			}
			got, err := Build("voitest-v1.1", k.Address, p, tc.offline, 0)
			if err != nil || got != tc.want {
				t.Fatalf("got %q error %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := Build("", k.Address, k.Key, true, 0); err == nil {
		t.Fatal("unknown network must be refused, not guessed")
	}
	if got, err := Build("voimain-v1.0", k.Address, k.Key, false, IncentiveFee); err != nil || got[len(got)-12:] != "&fee=2000000" {
		t.Fatalf("Voi incentive fee: %q %v", got, err)
	}
	if _, err := Build("mainnet-v1.0", k.Address, k.Key, true, IncentiveFee); err == nil {
		t.Fatal("a go-offline URI must refuse the fee")
	}
	for net, want := range map[string]string{
		"voimain-v1.0": "voi://net:voimain-v1.0/" + k.Address + "?type=keyreg",
		"voibeta-v1.0": "voi://net:voibeta-v1.0/" + k.Address + "?type=keyreg",
		"mainnet-v1.0": "algorand://" + k.Address + "?type=keyreg",
		"testnet-v1.0": "algorand://net:testnet/" + k.Address + "?type=keyreg",
		"betanet-v1.0": "algorand://net:betanet/" + k.Address + "?type=keyreg",
	} {
		got, err := Build(net, k.Address, k.Key, true, 0)
		if err != nil || got != want {
			t.Fatalf("%s: got %q %v; want %q", net, got, err, want)
		}
	}
	got, err := Build("mainnet-v1.0", k.Address, k.Key, false, IncentiveFee)
	if err != nil || got != "algorand://"+k.Address+"?type=keyreg"+keys+"&sprfkey=-_8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"+suffix+"&fee=2000000" {
		t.Fatalf("algorand fee URI: %q %v", got, err)
	}
}

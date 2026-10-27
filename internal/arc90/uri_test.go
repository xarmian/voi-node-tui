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
			got, warn, err := Build("voitest-v1.1", k.Address, p, tc.offline)
			if err != nil || warn != "" || got != tc.want {
				t.Fatalf("got %q warning %q error %v; want %q", got, warn, err, tc.want)
			}
		})
	}
	got, warn, err := Build("", k.Address, k.Key, true)
	if err != nil || warn == "" || got != "voi://net:voimain-v1.0/"+k.Address+"?type=keyreg" {
		t.Fatalf("fallback: %s %s %v", got, warn, err)
	}
	for _, net := range []string{"voimain-v1.0", "voibeta-v1.0"} {
		_, warn, err := Build(net, k.Address, k.Key, false)
		if err != nil || warn != "" {
			t.Fatal(net, warn, err)
		}
	}
}

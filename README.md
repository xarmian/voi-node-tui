# voi-node-tui

`vnode` is an SSH-friendly dashboard for a Voi participation node. It shows live
rounds, sync health, participation accounts, and local keys. It can generate and
delete participation keys; registration is signed separately in a wallet.

## Build and install

Requires Go 1.26. Build a static Linux amd64 binary locally, then copy it to your
node host (no Go installation needed there):

```sh
make build
file bin/vnode
scp bin/vnode your-node:~/vnode
ssh -t your-node './vnode --read-only'
```

Run over an interactive SSH terminal as a user who can read the admin token.
For Voi Swarm autodetection, that user also needs permission to run `docker
inspect`. The app only inspects metadata; it never starts containers or runs
commands inside them. Remove `--read-only` when you want to manage keys.

### Screenshots

`./vnode --redact` shows your live node with everything that could be looked up
on-chain disguised: addresses, participation keys and key ids become keyed
hashes, balances are scaled per account, labels are hidden, and every round on
screen (the block number, key validity ranges, the QR's `votefst`/`votelst`) is
shifted by a random offset. The disguise changes each run. `--redact` implies
`--read-only`.

## Connection and flags

The first configured source wins. A partial explicit source fails with an error
rather than silently connecting to a different node.

1. `--algod-url` plus either `--token` or `--token-file` (mutually exclusive).
2. `ALGOD_URL` plus `ALGOD_TOKEN`.
3. `--data-dir`, or `ALGORAND_DATA`: reads `algod.net` and `algod.admin.token`.
   Wildcard `[::]` and `0.0.0.0` listeners resolve to `127.0.0.1`.
4. Voi Swarm: inspect `voinetwork_algod`, then `voinetwork_relay`,
   `voinetwork_developer`, `voinetwork_archiver`. Use its default bridge IP and
   read the port and admin token from `/var/lib/voi/algod/data/`.

```sh
./vnode --algod-url http://127.0.0.1:8080 --token-file /path/to/algod.admin.token
./vnode --data-dir /path/to/algod/data --read-only
./vnode --help
```

| Flag | Meaning |
| --- | --- |
| `--algod-url` | HTTP(S) algod endpoint; requires an explicit token option |
| `--token-file` | Admin-token file (preferred) |
| `--token` | Admin token; may be visible in shell history and process lists |
| `--data-dir` | Directory containing `algod.net` and `algod.admin.token` |
| `--read-only` | Disable **all** node and watch-list writes |

The admin token is needed for participation endpoints, including listing keys.
Tokens and server response bodies are never printed or logged. Redirects are
rejected to keep credentials at the configured endpoint.

## Keys and workflow

| Key | Action |
| --- | --- |
| Up/Down or `j`/`k` | Select account; full address appears below the table |
| Left/Right or `[`/`]` | Select a local participation key |
| `h` / `l` | Pan wide account tables left/right |
| PageUp/PageDown, Home/End | Scroll details and help |
| `a` | Add checksum-validated address and optional label; offer generation |
| `g` | Generate/renew; edit validity span, then confirm with `y` |
| `q` or Enter | Selected key's QR with ARC-90 URI beneath it |
| `c` | Copy URI via OSC 52 and print it for manual copy |
| `o` | Offline keyreg QR (no participation keys) |
| `d` | Delete selected key with the required confirmation |
| `r` | Remove watch-list entry; offer individually confirmed key deletions |
| `R` | Refresh everything |
| `?` | Help |
| Esc | Close a screen/prompt; cancel remaining queued deletions |
| Ctrl+C | Quit |

`q` shows a QR; it does not quit. In QR/URI screens, `c` copies the displayed URI,
including an offline URI. OSC 52 requires terminal support/permission and may
need configuration in tmux/screen; the URI is also displayed for manual copy.
Resize the terminal if the complete QR does not fit. QR pixels use explicit
black/white colors and a quiet zone, independent of the terminal theme.

Generation starts at the current displayed round and defaults to a span of
5,000,000 rounds. Dilution is omitted so algod chooses it. After confirmation,
the app polls for a new key ID with the requested address and range. This can
take minutes; there is no fabricated percentage and writes are never retried
automatically. If a request fails, inspect/refresh keys before retrying. Leaving
the app does not cancel generation already accepted by algod.

Scan the QR in Voi Wallet or paste the URI into voirewards to sign registration.
Generating a key alone does **not** register it online. The network label comes
from `/versions`: `voimain-v1.0`, `voitest-v1.1`, or `voibeta-v1.0`. An unknown
genesis ID falls back to `voimain-v1.0` with a visible warning; verify the network
before signing. State-proof keys are included when present, and all public keys
use unpadded base64url. An offline URI has only `?type=keyreg`.

Accounts are the union of local participation-key addresses and the watch list
at `~/.config/vnode/accounts.json`. Adding an existing address updates its label.
The list is saved atomically with private file permissions:

```json
[
  {"address": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ", "label": "example"}
]
```

The example is the checksum-valid zero address, for illustration only. An account
with keys remains visible after removal from the watch list. Corrupt watch-list
files cause an error and are never silently overwritten.

Registered keys match the on-chain vote public key. The table shows balance,
on-chain status, incentive eligibility, registered key/range, remaining rounds,
estimated UTC expiry, last vote, and last proposal. Details show every local
key's dilution, effective range, and last vote/proposal/state-proof rounds.
Expiry estimates use up to 20 recent observed block intervals; dates remain
unknown until live samples exist. Warnings identify Online accounts with a
missing local registered key, registered keys within 14 days of expiry, and
Online accounts whose registered local key has expired. These are estimates,
not a renewal schedule.

## Safety and validation

Every generation/deletion requires explicit confirmation. Deleting the registered
key of an Online account requires typing the full address and warns that the node
stops voting. The app re-fetches on-chain account state just before deletion, so a
stale Offline screen cannot bypass this guard. Other deletions require `y`.
Deleting a local key does not submit an offline transaction; use `o` and sign the
URI externally when you intend to take an account offline.

`--read-only` blocks node generation/deletion and watch-list additions/removals.
Viewing and copying online/offline URIs is available in this mode because it does
not write anything to algod; signing an offline URI elsewhere changes account
status. The app holds no spending keys, mnemonics, wallets, or signing capability.
It has no automatic renewal, kmd, relay, or telemetry management.

Network errors remain visible, reads keep retrying, and input stays responsive.
The header stays visible across screens, visibly ticks on blocks, and shows
catchup/unsupported-protocol/stalled status. Writes do not auto-retry.

```sh
make test
make vet
make build
file bin/vnode
```

Tests use only `httptest` fake algod servers and injected filesystem/docker
interfaces. Development and validation never require a real node or Docker.

The algod SDK v2 supplies public REST calls, account types, and checksum
validation. Participation endpoints missing from that SDK use `net/http`, matching
the [official REST contract](https://dev.algorand.co/reference/rest-api/algod/operations/getparticipationkeys/).
The pinned Bubble Tea v1 / Bubbles v1 / Lip Gloss v1 versions are compatible with
each other. [qrterminal/v3](https://pkg.go.dev/github.com/mdp/qrterminal/v3@v3.2.1)
renders pure-Go half-block QR codes with no subprocess or C dependency.

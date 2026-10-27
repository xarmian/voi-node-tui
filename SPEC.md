# voi-node-tui — spec v1

An interactive terminal dashboard for a Voi (algod) participation node. It shows node state and manages participation keys. Single static Go binary, run over SSH on the node host (first target: a Voi Swarm participation node).

## Stack

- Go (1.26 locally), Bubble Tea + Bubbles + Lip Gloss.
- `github.com/algorand/go-algorand-sdk/v2` for the algod REST client and types, or plain `net/http` where the SDK lacks an endpoint.
- QR rendering in pure Go (e.g. `github.com/mdp/qrterminal/v3` or `skip2/go-qrcode` plus half-block rendering). No `qrencode`, no docker exec for the QR.
- Build: `make build` → `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` static binary `bin/vnode`.

## Connecting to algod

Resolution order, first match wins:
1. Flags `--algod-url` + `--token` / `--token-file`.
2. Env `ALGOD_URL` + `ALGOD_TOKEN`.
3. `--data-dir <dir>` (or env `ALGORAND_DATA`): read `algod.net` (host:port; `[::]` → `127.0.0.1`) and `algod.admin.token`.
4. Voi Swarm autodetect: `docker inspect voinetwork_algod` (also `voinetwork_relay`/`_developer`/`_archiver`) for the container's bridge IP; port from `/var/lib/voi/algod/data/algod.net` (port only); admin token from `/var/lib/voi/algod/data/algod.admin.token`.

On a typical Voi Swarm host: the container is `voinetwork_algod` on the default bridge (172.17.0.2), algod listens on `[::]:8080` inside it, no host port is published, and `/var/lib/voi/algod/data/algod.admin.token` is readable by the login user. algod 5.0.4, genesis `voimain-v1.0`.

The admin token is required: the participation endpoints need it. Never print or log the token.

## Views

### 1. Node (header, always visible)
Current round (live: long-poll `GET /v2/status/wait-for-block-after/{round}`), time since last round, catchup state, last version / network (genesis id from `GET /versions`), sync health. Block arrival should visibly tick.

### 2. Accounts (main view)
One row per participation account. An account is any address with at least one participation key on the node, plus addresses on a local watch list (`~/.config/vnode/accounts.json`) that may have none yet.
Columns: address (shortened, full on select), optional label (from the watch list), balance, on-chain status (Online/Offline from `GET /v2/accounts/{addr}?exclude=all`), incentive eligible, which local key is the registered one (match the on-chain `participation.vote-participation-key` with the key's vote key), key valid range, rounds until the registered key expires and an estimated date (average block time from recent rounds), last vote, last proposal.
Warnings: online with no matching local key (votes are being missed); registered key expiring within ~14 days at current block time; online with a local key that is past its last-valid.

### 3. Keys (detail for the selected account)
All participation keys for the account from `GET /v2/participation`: id, first/last valid, key dilution, effective range, last vote / proposal / state proof, and a "registered on-chain" marker.

## Actions

| Key | Action | Endpoint / behaviour |
|---|---|---|
| a | Add account | Prompt for address (validate checksum) and optional label; adds to the watch list. Offers to generate a key now. |
| g | Generate key / renew | `POST /v2/participation/generate/{address}?first=<current round>&last=<first+N>` (N default 5,000,000, editable; dilution default: let algod pick, i.e. omit, or ceil(sqrt(N)) if required). Generation is async and can take minutes: show progress by polling `GET /v2/participation` until the new key appears. Renew = generate for an account that already has a key. |
| q / Enter | Show keyreg QR | Build the ARC-90 keyreg URI for the selected key and render the QR full-screen, with the URI text under it. |
| c | Copy URI | Copy the keyreg URI to the local clipboard via OSC 52 (works over SSH in modern terminals), also print it for manual copy. |
| o | Go-offline QR | ARC-90 keyreg URI with no keys (marks the account offline when signed). |
| d | Delete key | `DELETE /v2/participation/{id}`. Guard: if this key is the one registered on-chain and the account is Online, refuse unless the user types the full address to confirm, with the warning that the node stops voting for it. Any other delete needs a y/N confirm. |
| r | Remove account | Remove from the watch list. If it still has keys, offer to delete them (each delete follows the guard above). |
| R | Refresh | Re-fetch everything. |
| ? | Help | Key bindings. |

The node never holds spending keys in v1. Registering a key online is done by signing the keyreg URI in a wallet (Voi Wallet scans the QR) or by pasting the URI into voirewards.

## ARC-90 keyreg URI

Must match Voi Swarm's `~/voi/bin/show-partkey-qr` exactly:

```
voi://net:<net-label>/<address>?type=keyreg&votekey=<b64url>&selkey=<b64url>[&sprfkey=<b64url>]&votefst=<first>&votelst=<last>&votekd=<dilution>
```

- Keys are converted from standard base64 (as algod returns them) to base64url, unpadded.
- Net label: the genesis id (`voimain-v1.0`, `voitest-v1.1`, `voibeta-v1.0`). Fall back to `voimain-v1.0` only if the genesis id is unknown, with a visible warning.
- `sprfkey` is included when the key has a state proof key.
- Go-offline form: `voi://net:<net-label>/<address>?type=keyreg`.

## Non-goals (v1)
kmd wallets, mnemonics, signing, relay/telemetry settings, auto-renew.

## Safety
- Every write (generate, delete) asks for confirmation, and deletes follow the guard above.
- A `--read-only` flag disables every write action.
- Network errors never crash the UI; show them in a status line and keep retrying.

## Tests
- URI builder: golden tests, including a fixture of real-shaped `partkeyinfo` / REST values, base64→base64url conversion, with and without sprfkey, and the offline form.
- Address checksum validation.
- Connection resolution order (flags > env > data dir > docker autodetect), with docker inspect and file reads behind interfaces.
- Expiry / warning logic: matching the registered key, estimating expiry, all three warnings.
- Delete guard: refuses the registered key for an Online account without a typed address.
- A fake algod (`httptest`) driving the model through generate → appears → QR → delete.
- `go vet` and `go test ./...` pass; `make build` produces a static binary.

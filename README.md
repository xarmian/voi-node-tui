# voi-node-tui

`vnode` is a terminal dashboard for an algod participation node on **Voi** or
**Algorand**. It shows the live chain, your participation accounts and their keys,
and lets you generate, renew and delete participation keys. Registering a key
on-chain is always signed elsewhere: `vnode` shows an ARC-90 keyreg QR code and
URI that you sign in a wallet. The node never holds spending keys.

It's a single static binary that you run over SSH on the node host.

## What it shows

- **Header:** the network, algod version and node health (healthy, catching up,
  stalled, connection error). It also shows the current block with a pulse that
  flashes on each new block, the average block time with a sparkline of recent
  blocks, and the stake online across the whole network.
- **Summary tiles:** accounts online, their combined stake and share of online
  stake, expected block proposals per day, and the soonest key expiry.
- **Accounts:** one row per participation account. Each row has a status icon, a
  balance, a key-life gauge (green, then amber under 30 days, then red under 14),
  the time until the registered key expires, the last vote ("live" while it's
  voting) and the last proposal.
- **Account detail:** the full address, exact balance, online status, incentive
  eligibility, share of online stake, expected blocks per day, any warnings, and
  every local participation key. The key registered on-chain is marked
  REGISTERED.

Warnings flag an Online account with no matching local key (it's missing votes),
a registered key within 14 days of expiry, and an Online account whose
registered key has already expired.

Dates are estimates. The block time is averaged over the last 1,000 blocks'
timestamps and refreshed every 10 minutes.

## Install

Build locally with Go 1.26 and copy the binary to the node. The node itself needs
no Go toolchain.

```sh
make build            # bin/vnode, static linux/amd64
make build-arm64      # bin/vnode-arm64, static linux/arm64 (e.g. Raspberry Pi)
scp bin/vnode node:~/bin/vnode
ssh -t node '~/bin/vnode --read-only'
```

Run it as a user who can read the node's **admin** token. The participation-key
endpoints, including listing keys, require it. Start with `--read-only` and drop
it when you want to manage keys.

## Finding the node

The first source that is configured wins. A half-configured source (a URL with no
token, for example) is an error. It never falls through to a different node.

1. **Flags:** `--algod-url` plus `--token-file` (preferred) or `--token`.
2. **Environment:** `ALGOD_URL` plus `ALGOD_TOKEN`.
3. **Data directory:** `--data-dir` or `$ALGORAND_DATA`. It reads `algod.net` and
   `algod.admin.token`, and a wildcard listener (`[::]`, `0.0.0.0`) is reached at
   `127.0.0.1`.
4. **Voi Swarm:** `docker inspect` on `voinetwork_algod`, then the `_relay`,
   `_developer` and `_archiver` variants. It uses the container's IP (Docker's
   default bridge, or else the first network that has one), because Swarm doesn't
   publish algod's port on the host. The port and admin token come from
   `/var/lib/voi/algod/data/`. It only reads container metadata and never runs
   anything inside the container.
5. **Standard Algorand installs:** `/var/lib/algorand` (the Debian/RPM package),
   then `~/node/data` (the updater script). If algod is found there but its admin
   token is unreadable, `vnode` names the directory and stops. Run it as the
   node's user (often `algorand`), or pass `--token-file`.

```sh
vnode                                          # autodetect
vnode --data-dir /var/lib/algorand
vnode --algod-url http://127.0.0.1:8080 --token-file ~/algod.admin.token
```

| Flag | Meaning |
| --- | --- |
| `--algod-url` | algod HTTP(S) endpoint; needs a token option |
| `--token-file` | file holding the admin token (preferred) |
| `--token` | admin token inline; visible in shell history and process lists |
| `--data-dir` | algod data directory (`algod.net`, `algod.admin.token`) |
| `--read-only` | disable every write: key generation and deletion, watch-list edits |
| `--redact` | screenshot mode (see below); implies `--read-only` |

The token is never printed or logged. Requests go straight to algod: HTTP proxy
settings are ignored and redirects are refused, so the token can't leave the
configured endpoint.

## Networks

The network comes from algod's genesis ID.

| Genesis ID | Network | Keyreg URI prefix | Units |
| --- | --- | --- | --- |
| `voimain-v1.0` | Voi MainNet | `voi://net:voimain-v1.0/` | VOI |
| `voitest-v1.1` | Voi TestNet | `voi://net:voitest-v1.1/` | VOI |
| `voibeta-v1.0` | Voi BetaNet | `voi://net:voibeta-v1.0/` | VOI |
| `mainnet-v1.0` | Algorand MainNet | `algorand://` | ALGO |
| `testnet-v1.0` | Algorand TestNet | `algorand://net:testnet/` | ALGO |
| `betanet-v1.0` | Algorand BetaNet | `algorand://net:betanet/` | ALGO |

On Algorand this follows [ARC-90](https://github.com/algorandfoundation/ARCs/blob/main/ARCs/arc-0090.md):
MainNet omits the network authority and other networks use their alias. Voi
wallets address the network by genesis ID, and the format matches Voi Swarm's
`show-partkey-qr`.

On any **other** network, `vnode` still shows the dashboard but refuses to build a
keyreg URI. A key registered on the wrong chain takes that account online there
with keys no node is voting with, so the account gets suspended for missing
votes.

**Algorand incentives.** On Algorand, press `f` in the QR view to add the
2 ALGO fee (`fee=2000000`) that requests participation incentives on a keyreg. It
is off by default, so you never pay it by surprise, and your wallet shows the fee
before you sign. Voi never gets this option.

## Keys

| Key | Action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | select an account |
| `←`/`→`, `[`/`]` | select one of its participation keys |
| Enter or `q` | registration QR and URI for the selected key |
| `c` | copy the URI (OSC 52) and show it |
| `o` | go-offline QR for the account (a keyreg with no keys) |
| `f` | in the QR view on Algorand: toggle the incentive fee |
| `g` | generate or renew a key; edit the round span, then confirm |
| `a` | add an address (checksum-validated) with an optional label |
| `d` | delete the selected key |
| `r` | remove an address from the watch list |
| `R` | refresh everything |
| `h`/`l`, PgUp/PgDn, Home/End | pan and scroll |
| `?` | help |
| Esc | close a screen, cancel a prompt, or offer to abandon a generation wait |
| Ctrl+C | quit |

`q` opens the QR. It does not quit.

## Registering a key

1. **Generate:** press `g`. The key starts at the current round and spans 5,000,000
   rounds by default (about 160 days at 2.8 s blocks), editable before you
   confirm. algod chooses the dilution. Generation runs inside algod and can take
   minutes. `vnode` polls until the new key appears, for up to 20 minutes, and
   Esc offers to stop waiting (generation inside algod carries on).
2. **Register:** select the key and press Enter. Scan the QR with Voi Wallet (or
   paste the URI into voirewards) on Voi, or with a wallet that supports ARC-90
   keyreg URIs on Algorand. **Generating a key does not register it.** Until the
   keyreg is signed and confirmed, the account keeps voting with its old key, or
   not at all.
3. **Retire the old key:** once the new key shows REGISTERED, delete the old one.

The QR needs about 40 terminal rows. When space is tight the URI text is dropped
first (`c` still copies it), then the title. If the QR can't fit at all, the view
says exactly how many columns and rows it needs. QR modules are drawn in explicit
black on white, so it scans on light and dark themes.

## Safety

- **Confirmation:** every generation and deletion needs explicit confirmation.
  Deleting the key an **Online** account is registered with needs its full address
  typed, and warns that the node will stop voting for it. The account is
  re-fetched from the chain just before the delete, so stale screen state can't
  bypass the check.
- **Deleting a key** does not take the account offline. Use `o` and sign the
  go-offline URI for that.
- **`--read-only`** blocks every write. Viewing and copying URIs still works,
  because building them writes nothing.
- **Retries:** reads retry automatically and errors stay in the status line.
  Writes are never retried automatically.
- **Out of scope:** no spending keys, mnemonics, kmd wallets, signing, automatic
  renewal, or relay and telemetry settings.

## Watch list

Accounts are the union of addresses that have a local participation key and
those on the watch list at `~/.config/vnode/accounts.json`. Add an address with
`a` before generating its first key. Removing an address that still has keys
leaves it visible, and `r` offers to delete those keys one at a time. The file is
written atomically with owner-only permissions. A corrupt file is reported, never
overwritten.

```json
[
  {"address": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAY5HFKQ", "label": "example"}
]
```

## Screenshots: `--redact`

`vnode --redact` reads your live node and disguises everything that could be
looked up on-chain:

- addresses, participation keys and key IDs become salted hashes, consistent
  within the run;
- each balance is scaled by its own random factor, so neither amounts nor their
  ratios match the chain;
- labels are hidden;
- each account's proposal history is pushed back by its own random 5-70 minutes,
  so the proposal ages can't be matched to recent blocks;
- every round on screen is shifted by one random offset: the block number, key
  validity ranges, and the QR's `votefst`/`votelst`, so neither can be looked up.

The disguise changes every run. Account count, expiry countdowns, the network's
online stake and the block time stay real. `--redact` implies `--read-only` and
hides the READ ONLY badge.

## Development

```sh
make test
make vet
make build
```

Tests run against `httptest` fake algod servers and injected filesystem and Docker
interfaces. They never need a real node or Docker.

The algod Go SDK v2 supplies the REST client and types. The participation
endpoints it lacks use `net/http`, per the
[algod REST reference](https://dev.algorand.co/reference/rest-api/algod/operations/getparticipationkeys/).
The UI is Bubble Tea, Bubbles and Lip Gloss (v1). QR codes come from
[qrterminal/v3](https://pkg.go.dev/github.com/mdp/qrterminal/v3), pure Go with no
subprocess. `SPEC.md` is the original v1 spec, kept for history.

package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarmian/voi-node-tui/internal/accounts"
)

var titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
var warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func short(s string) string {
	if len(s) > 16 {
		return s[:8] + "…" + s[len(s)-6:]
	}
	return s
}
func (m *Model) header() string {
	health := "syncing"
	since := "unknown"
	catchup := "none"
	if !m.roundAt.IsZero() {
		elapsed := max(time.Duration(0), time.Since(m.roundAt))
		since = elapsed.Round(time.Second).String()
		health = "healthy"
		if elapsed > 30*time.Second {
			health = "stalled / catching up"
		}
	}
	if m.statusAt.IsZero() {
		health = "connecting"
	}
	if m.status.CatchupTime > 0 || m.status.Catchpoint != "" {
		health = "catching up"
		catchup = fmt.Sprintf("%s, %.1fs", clean(m.status.Catchpoint), float64(m.status.CatchupTime)/1e9)
	}
	if m.status.StoppedAtUnsupportedRound {
		health = "stopped: unsupported protocol"
	}
	if m.netErr != "" || m.waitErr != "" {
		health = "connection error / stale data"
	}
	pulse := "·"
	if m.pulse {
		pulse = "*"
	}
	readonly := ""
	if m.readOnly {
		readonly = " [READ ONLY]"
	}
	network := clean(m.version.GenesisID)
	if network == "" {
		network = "unknown"
	}
	lines := []string{
		titleStyle.Render(fmt.Sprintf("vnode%s  %s round %d  last %s  %s", readonly, pulse, m.status.LastRound, since, health)),
		fmt.Sprintf("Network %s  algod %d.%d.%d", network, m.version.Build.Major, m.version.Build.Minor, m.version.Build.BuildNumber),
		fmt.Sprintf("Catchup: %s  Protocol: %s", catchup, clean(m.status.LastVersion)),
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(1, m.width), "…")
	}
	return strings.Join(lines, "\n")
}
func (m *Model) View() string {
	header := m.header()
	status := clean(m.message)
	if m.working {
		status = "Working… " + status
	}
	if m.pending != nil {
		status = fmt.Sprintf("Generating for %s — waiting for key (%s); algod may take minutes. %s", short(m.pending.address), time.Since(m.pending.started).Round(time.Second), status)
	}
	if m.netErr != "" {
		status = clean(m.netErr) + " | " + status
	}
	if m.waitErr != "" {
		status = clean(m.waitErr) + " | " + status
	}
	footer := ansi.Truncate(status, max(1, m.width), "…") + "\n" + ansi.Truncate("↑/↓ accounts  ←/→ keys  h/l pan  PgUp/PgDn scroll  ? help  Esc close  Ctrl+C quit", max(1, m.width), "…")
	body := ""
	switch m.mode {
	case "help":
		body = helpText
	case "qr", "uri":
		if m.networkWarning != "" {
			body += warningStyle.Render(ansi.Hardwrap(m.networkWarning, max(1, m.width), false)) + "\n"
		}
		tail := "\n" + ansi.Hardwrap(m.uri, max(1, m.width), false) + "\n\nc: copy via OSC 52  Esc: return"
		if m.qr != "" {
			candidate := body + m.qr + tail
			requiredHeight := lipgloss.Height(candidate) + lipgloss.Height(header) + lipgloss.Height(footer) + 2
			requiredWidth := lipgloss.Width(m.qr)
			if requiredWidth <= m.width && requiredHeight <= m.height {
				body += m.qr
			} else {
				body += ansi.Hardwrap(fmt.Sprintf("Enlarge the terminal to at least %d columns × %d rows to scan the complete QR.", requiredWidth, requiredHeight), max(1, m.width), false) + "\n"
			}
		}
		body += tail
	default:
		if m.mode != "" {
			body = ansi.Hardwrap(cleanPrompt(m.prompt), max(1, m.width), false) + "\n"
			switch m.mode {
			case "add-address", "add-label", "generate-range", "delete-address":
				body += m.input.View() + "\nEnter: submit  Esc: cancel"
			default:
				body += "y: confirm  any other key: cancel"
			}
		} else {
			body = m.dashboard()
		}
	}
	m.viewport.Height = max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
	m.viewport.Width = max(1, m.width)

	m.viewport.SetContent(body)
	return header + "\n\n" + m.viewport.View() + "\n" + footer
}
func cleanPrompt(s string) string {
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = clean(parts[i])
	}
	return strings.Join(parts, "\n")
}
func (m *Model) dashboard() string {
	if len(m.rows) == 0 {
		return "No participation accounts. Press a to add an address to the watch list."
	}
	var b strings.Builder
	count := max(3, (m.height-13)/2)
	start := max(0, min(m.selected-count/2, len(m.rows)-count))
	end := min(len(m.rows), start+count)
	fmt.Fprintf(&b, "ACCOUNTS %d–%d / %d  (h/l scroll columns; ! = warning or stale data)\n", start+1, end, len(m.rows))
	fmt.Fprintln(&b, "  ADDRESS          LABEL         BALANCE VOI  STATUS   INCENTIVE  REGISTERED KEY   VALID RANGE            ROUNDS LEFT  EXPIRY (EST. UTC)     LAST VOTE  LAST PROPOSAL")
	now := time.Now()
	for i := start; i < end; i++ {
		r := m.rows[i]
		d := accounts.Derive(r.Account, m.keys, m.status.LastRound, m.average, now)
		mark := " "
		if i == m.selected {
			mark = ">"
		}
		expiry, left, valid := "unknown", "—", "—"
		if d.Last > 0 {
			left = fmt.Sprint(d.RoundsLeft)
			valid = fmt.Sprintf("%d–%d", d.First, d.Last)
		}
		if !d.ExpiresAt.IsZero() {
			expiry = d.ExpiresAt.UTC().Format("2006-01-02 15:04")
		}
		id := short(d.RegisteredID)
		if id == "" {
			id = "none"
		}
		state := r.Status
		if r.Err != nil || len(d.Warnings) > 0 {
			state += "!"
		}
		fmt.Fprintf(&b, "%s %-16s %-12s %12s %-8s %-10t %-16s %-22s %11s  %-19s %9d %14d\n", mark, short(r.Address), ansi.Truncate(clean(r.Label), 12, "…"), fmt.Sprintf("%d.%06d", r.Amount/1000000, r.Amount%1000000), state, r.IncentiveEligible, id, valid, left, expiry, d.LastVote, d.LastProposal)
	}
	a := m.current()
	if a == nil {
		return b.String()
	}
	fmt.Fprintf(&b, "\nSelected: %s  %s\n", a.Address, clean(a.Label))
	if a.Err != nil {
		fmt.Fprintln(&b, warningStyle.Render("Account data is stale: "+a.Err.Error()))
	}
	d := accounts.Derive(a.Account, m.keys, m.status.LastRound, m.average, now)
	for _, w := range d.Warnings {
		fmt.Fprintln(&b, warningStyle.Render("WARNING: "+w))
	}
	if m.average > 0 {
		fmt.Fprintf(&b, "Estimated expiry uses recent average block time: %s (%d samples).\n", m.average.Round(time.Millisecond), len(m.samples))
	} else {
		fmt.Fprintln(&b, "Expiry date unknown until live block-time samples are available.")
	}
	fmt.Fprintln(&b, "\nLOCAL KEYS (←/→ select; Enter shows registration QR)")
	ks := m.accountKeys()
	if len(ks) == 0 {
		fmt.Fprintln(&b, "No local keys. Press g to generate one.")
	}
	for i, k := range ks {
		mark := " "
		if i == m.keyIndex {
			mark = ">"
		}
		registered := ""
		if accounts.Registered(a.Account, k) {
			registered = " [registered on-chain]"
		}
		effective := "not reported"
		if k.EffectiveFirstValid != nil && k.EffectiveLastValid != nil {
			effective = fmt.Sprintf("%d–%d", *k.EffectiveFirstValid, *k.EffectiveLastValid)
		}
		fmt.Fprintf(&b, "%s %s%s\n  valid %d–%d  dilution %d  effective %s\n  last vote %d  proposal %d  state proof %d\n", mark, clean(k.Id), registered, k.Key.VoteFirstValid, k.Key.VoteLastValid, k.Key.VoteKeyDilution, effective, k.LastVote, k.LastBlockProposal, k.LastStateProof)
	}
	return b.String()
}

const helpText = `ACCOUNTS / KEYS
↑/↓ or j/k   Select account (full address shown below the table)
←/→ or [/]   Select its participation key
h/l          Pan wide tables left/right
PgUp/PgDn    Scroll details; Home/End jump

a            Add checksum-validated address and optional label; offer generation
g            Generate / renew; edit round span (default 5,000,000), then confirm
q / Enter    Show selected key's registration QR and URI
c            Copy URI with OSC 52 and show it for manual copy
o            Show offline registration QR (no keys)
d            Delete selected key after confirmation
r            Remove watch-list entry; offer individually guarded key deletions
R            Refresh everything
?            Help
Esc          Close QR/help/prompt, cancel delete queue, or offer to abandon generation
Ctrl+C       Quit

Generation and deletion require explicit confirmation. An Online registered key
requires typing its full account address; deleting it stops voting.
Generation waits up to 20 minutes. Esc on the dashboard offers to abandon the wait;
abandoning does not cancel generation inside algod.
Read-only mode blocks all node and watch-list writes.
QRs are signed externally in Voi Wallet or pasted into voirewards.
This node holds no spending keys, mnemonics, or signing capability.
Network errors stay in the status line; reads retry automatically. Writes do not.`

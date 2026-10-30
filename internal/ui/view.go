package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
	"github.com/xarmian/voi-node-tui/internal/arc90"
)

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

// roundsAgo turns a past round into a human age, or a round count before block time is known.
func (m *Model) roundsAgo(round uint64) string {
	if round == 0 {
		return "never"
	}
	if round >= m.status.LastRound {
		return "just now"
	}
	n := m.status.LastRound - round
	if m.average <= 0 {
		return fmt.Sprintf("%d rnd", n)
	}
	return span(time.Duration(n)*m.average) + " ago"
}

// health classifies the node for the header pill.
func (m *Model) health() (string, lipgloss.Color) {
	switch {
	case m.netErr != "" || m.waitErr != "":
		return "CONNECTION ERROR", cBad
	case m.status.StoppedAtUnsupportedRound:
		return "STOPPED: UNSUPPORTED PROTOCOL", cBad
	case m.statusAt.IsZero():
		return "CONNECTING", cMuted
	case m.status.CatchupTime > 0 || m.status.Catchpoint != "":
		return "CATCHING UP", cWarn
	case !m.roundAt.IsZero() && time.Since(m.roundAt) > 30*time.Second:
		return "STALLED", cWarn
	}
	return "● HEALTHY", cOK
}

// network describes the connected chain; an unknown genesis gets neutral wording.
func (m *Model) network() arc90.Network {
	if n, ok := arc90.Lookup(m.version.GenesisID); ok {
		return n
	}
	return arc90.Network{Name: "unknown network", Unit: "units", Wallet: "a wallet"}
}

func (m *Model) header() string {
	w := max(20, m.width)
	network := clean(m.version.GenesisID)
	if network == "" {
		network = "unknown network"
	}
	left := gradient("◆ vnode") + "  " + pill(network, cViolet) + " " + sMuted.Render(fmt.Sprintf("algod %d.%d.%d", m.version.Build.Major, m.version.Build.Minor, m.version.Build.BuildNumber))
	if m.readOnly && m.redact == nil {
		left += " " + pill("READ ONLY", cWarn)
	}
	label, color := m.health()
	line1 := spread(left, pill(label, color), w-4)

	// The dot flashes on each new block and cools as the block ages.
	dot := sDim.Render("●")
	since := "—"
	if !m.roundAt.IsZero() {
		age := max(0, time.Since(m.roundAt))
		since = span(age)
		if since == "now" {
			since = "just now"
		} else {
			since += " ago"
		}
		switch {
		case age < 1500*time.Millisecond:
			dot = sOK.Bold(true).Render("●")
		case age < 4*time.Second:
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#059669")).Render("●")
		}
	}
	block := sMuted.Render("BLOCK ") + sBold.Render(commas(m.shown(m.status.LastRound))) + " " + dot + sMuted.Render("  last ") + sText.Render(since)
	if m.average > 0 {
		block += sMuted.Render("  avg ") + sText.Render(fmt.Sprintf("%.2fs", m.average.Seconds())) + "  " + spark(m.samples, 24)
	}
	right := ""
	if m.supply.OnlineMoney > 0 {
		right = sMuted.Render("network online ") + sText.Render(compactVOI(m.supply.OnlineMoney)+" "+m.network().Unit)
	}
	lines := []string{line1, spread(block, right, w-4)}
	if m.status.CatchupTime > 0 || m.status.Catchpoint != "" {
		lines = append(lines, sWarn.Render(fmt.Sprintf("catchup %s  %.1fs", clean(m.status.Catchpoint), float64(m.status.CatchupTime)/1e9)))
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w-4, "…")
	}
	return panel(cPanel, w).Render(strings.Join(lines, "\n"))
}

func (m *Model) statusLine() string {
	icon, style := "›", sMuted
	status := clean(m.message)
	if m.working {
		icon, style = "◌", sAccent
		status = "Working… " + status
	}
	if m.pending != nil {
		icon, style = "◌", sAccent
		status = fmt.Sprintf("Generating for %s — waiting for key (%s); algod may take minutes. %s", short(m.pending.address), time.Since(m.pending.started).Round(time.Second), status)
	}
	if m.netErr != "" {
		icon, style = "✖", sBad
		status = clean(m.netErr) + " | " + status
	}
	if m.waitErr != "" {
		icon, style = "✖", sBad
		status = clean(m.waitErr) + " | " + status
	}
	return ansi.Truncate(" "+style.Render(icon+" "+status), max(1, m.width), "…")
}

func (m *Model) hints() string {
	var h string
	switch m.mode {
	case "":
		h = keycaps("↑↓", "account", "←→", "key", "⏎", "qr", "c", "copy", "o", "offline", "g", "renew", "a", "add", "d", "delete", "?", "help", "^C", "quit")
	case "qr", "uri":
		h = keycaps("c", "copy uri", "esc", "back", "PgUp/PgDn", "scroll", "^C", "quit")
		if _, ok := arc90.Lookup(m.version.GenesisID); ok && !m.uriOffline {
			h = keycaps("c", "copy uri", "f", "incentive fee", "esc", "back", "^C", "quit")
		}
	case "help":
		h = keycaps("esc", "back", "PgUp/PgDn", "scroll", "^C", "quit")
	default:
		h = keycaps("esc", "cancel", "^C", "quit")
	}
	return ansi.Truncate(" "+h, max(1, m.width), "…")
}

func (m *Model) View() string {
	header := m.header()
	if m.mode == "qr" {
		// The QR needs every row it can get; a one-line header still shows the node is live.
		label, color := m.health()
		header = ansi.Truncate(" "+gradient("◆ vnode")+"  "+sMuted.Render("block ")+sBold.Render(commas(m.shown(m.status.LastRound)))+"  "+pill(label, color), max(1, m.width), "…")
	}
	footer := m.statusLine() + "\n" + m.hints()
	w := max(20, m.width)
	body := ""
	switch m.mode {
	case "help":
		body = m.helpView(w)
	case "qr", "uri":
		body = m.qrView(w, max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1))
	case "":
		body = m.dashboard()
	default:
		body = m.dialog(w)
	}
	m.viewport.Height = max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1)
	m.viewport.Width = max(1, m.width)
	m.viewport.SetContent(body)
	return header + "\n" + m.viewport.View() + "\n" + footer
}

func cleanPrompt(s string) string {
	parts := strings.Split(s, "\n")
	for i := range parts {
		parts[i] = clean(parts[i])
	}
	return strings.Join(parts, "\n")
}

func (m *Model) dialog(w int) string {
	border, title := cViolet, "Confirm"
	switch m.mode {
	case "delete-confirm", "delete-address", "remove-confirm", "remove-keys":
		border, title = cBad, "Destructive action"
	case "abandon-generation":
		border, title = cWarn, "Abandon wait"
	case "add-address", "add-label":
		title = "Add account"
	case "generate-range", "generate-confirm", "offer-generate":
		title = "Generate participation key"
	}
	inner := min(w-4, 84)
	content := lipgloss.NewStyle().Foreground(border).Bold(true).Render(title) + "\n\n" + sText.Render(ansi.Hardwrap(cleanPrompt(m.prompt), inner-4, false)) + "\n\n"
	switch m.mode {
	case "add-address", "add-label", "generate-range", "delete-address":
		content += m.input.View() + "\n\n" + keycaps("⏎", "submit", "esc", "cancel")
	default:
		content += keycaps("y", "confirm", "any other key", "cancel")
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(1, 2).Width(inner).Render(content)
	return "\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, box)
}

// qrView fits the QR into avail rows, shedding the URI text and then the title before
// giving up on the QR; the URI is always one keypress (c) away.
func (m *Model) qrView(w, avail int) string {
	wallet := m.network().Wallet
	title := "Scan with " + wallet + " to register this key"
	if m.uriOffline {
		title = "Scan with " + wallet + " to take this account offline"
	}
	head := lipgloss.PlaceHorizontal(w, lipgloss.Center, sSection.Render(title))
	if a := m.current(); a != nil {
		head += "\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, sMuted.Render(a.Address))
	}
	warn := ""
	if _, ok := arc90.Lookup(m.version.GenesisID); ok && !m.uriOffline {
		state := sMuted.Render("incentive fee ") + sDim.Render("off") + sMuted.Render(" · press ") + sKeycap.Render("f") + sMuted.Render(" to add the 2 "+m.network().Unit+" fee that requests participation incentives")
		if m.incentive {
			state = sMuted.Render("incentive fee ") + sOK.Render("2 "+m.network().Unit) + sMuted.Render(" · press ") + sKeycap.Render("f") + sMuted.Render(" to remove it")
		}
		warn = lipgloss.PlaceHorizontal(w, lipgloss.Center, state) + "\n"
	}
	if m.uriOffline && m.network().Unit == "VOI" {
		warn = lipgloss.PlaceHorizontal(w, lipgloss.Center, sBad.Bold(true).Render(ansi.Wordwrap(OfflineWalletWarning, w-4, " "))) + "\n"
	}
	uri := sDim.Render(ansi.Hardwrap(m.uri, w, false))
	if m.qr == "" {
		return "\n" + head + "\n" + warn + "\n" + uri + "\n"
	}
	qr := strings.TrimRight(m.qr, "\n")
	qrW, qrH := lipgloss.Width(qr), lipgloss.Height(qr)
	warnH := lipgloss.Height(warn) - 1
	if qrW > w || qrH+warnH > avail {
		msg := fmt.Sprintf("The QR needs %d columns × %d rows; this view has %d × %d. Enlarge the terminal or zoom out to scan it.", qrW, qrH+warnH, w, avail)
		return "\n" + head + "\n" + warn + sWarn.Render(ansi.Hardwrap(msg, w, false)) + "\n\n" + uri + "\n"
	}
	centred := lipgloss.PlaceHorizontal(w, lipgloss.Center, qr)
	hint := lipgloss.PlaceHorizontal(w, lipgloss.Center, sMuted.Render("press ")+sKeycap.Render("c")+sMuted.Render(" to copy the URI"))
	headH, uriH := lipgloss.Height(head), lipgloss.Height(uri)
	switch {
	case 1+headH+1+warnH+qrH+1+uriH <= avail:
		return "\n" + head + "\n" + warn + "\n" + centred + "\n\n" + uri + "\n"
	case headH+warnH+qrH+1 <= avail:
		return head + "\n" + warn + centred + "\n" + hint + "\n"
	case 1+warnH+qrH <= avail:
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, sSection.Render(title)) + "\n" + warn + centred + "\n"
	}
	return warn + centred + "\n"
}

type tile struct{ label, value, sub string }

func (m *Model) tiles(w int) string {
	var online, total, stake uint64
	var lastProp uint64
	nextLeft, haveNext := time.Duration(0), false
	nextRounds := uint64(0)
	dots := ""
	for _, r := range m.rows {
		d := accounts.Derive(r.Account, m.keys, m.status.LastRound, m.average, time.Now())
		total++
		switch {
		case r.Status == "Online" && len(d.Warnings) == 0 && r.Err == nil:
			online++
			stake += r.Amount
			dots += sOK.Render("●")
		case r.Status == "Online":
			online++
			stake += r.Amount
			dots += sWarn.Render("▲")
		default:
			dots += sDim.Render("○")
		}
		lastProp = max(lastProp, d.LastProposal)
		if r.Status == "Online" && d.Last > 0 && (!haveNext || d.RoundsLeft < nextRounds) {
			haveNext, nextRounds = true, d.RoundsLeft
			nextLeft = time.Duration(d.RoundsLeft) * m.average
		}
	}

	onlineValue := fmt.Sprintf("%d / %d", online, total)
	if online == total && total > 0 {
		onlineValue = sOK.Bold(true).Render(onlineValue)
	} else {
		onlineValue = sWarn.Bold(true).Render(onlineValue)
	}
	stakeSub := sMuted.Render("held by online accounts")
	propValue, propSub := sBold.Render("—"), sMuted.Render("needs network stake")
	if m.supply.OnlineMoney > 0 && stake > 0 {
		share := float64(stake) / float64(m.supply.OnlineMoney)
		stakeSub = sCyan.Render(fmt.Sprintf("%.3f%%", share*100)) + sMuted.Render(" of online stake")
		if m.average > 0 {
			perDay := share * float64(24*time.Hour) / float64(m.average)
			figure := fmt.Sprintf("%.1f", perDay)
			if perDay >= 100 {
				figure = commas(uint64(perDay + 0.5))
			}
			propValue = sBold.Render("~" + figure + " / day")
		}
		propSub = sMuted.Render("last ") + sText.Render(m.roundsAgo(lastProp))
	}
	expValue, expSub := sBold.Render("—"), sMuted.Render("no registered keys")
	if haveNext {
		if m.average > 0 {
			expValue = lipgloss.NewStyle().Foreground(lifeColor(nextLeft, true)).Bold(true).Render(span(nextLeft))
			expSub = sMuted.Render("soonest, ") + sText.Render(time.Now().Add(nextLeft).UTC().Format("2006-01-02"))
		} else {
			expValue = sBold.Render(commas(nextRounds) + " rnd")
			expSub = sMuted.Render("soonest key expiry")
		}
	}
	ts := []tile{
		{"ONLINE", onlineValue, dots},
		{"STAKE", sBold.Render(compactVOI(stake) + " " + m.network().Unit), stakeSub},
		{"EXPECTED PROPOSALS", propValue, propSub},
		{"NEXT KEY EXPIRY", expValue, expSub},
	}
	perRow := 4
	if w < 88 {
		perRow = 2
	}
	tw := w / perRow
	var rows []string
	for i := 0; i < len(ts); i += perRow {
		var cells []string
		for _, t := range ts[i:min(len(ts), i+perRow)] {
			body := sColHead.Render(t.label) + "\n" + t.value + "\n" + ansi.Truncate(t.sub, tw-4, "…")
			cells = append(cells, panel(cDim, tw).Render(body))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) dashboard() string {
	w := max(20, m.width)
	if len(m.rows) == 0 {
		msg := sSection.Render("No participation accounts yet") + "\n\n" + sMuted.Render("Press ") + sKeycap.Render("a") + sMuted.Render(" to add an address, then ") + sKeycap.Render("g") + sMuted.Render(" to generate its participation key.")
		return "\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, panel(cPanel, min(w, 72)).Padding(1, 2).Render(msg))
	}
	var b strings.Builder
	b.WriteString(m.tiles(w) + "\n")

	showLabel, showProp, showBar := w >= 118, w >= 100, w >= 84
	barW := 14
	head := "    " + fmt.Sprintf("%-17s", "ACCOUNT")
	if showLabel {
		head += fmt.Sprintf("%-14s", "LABEL")
	}
	head += fmt.Sprintf("%10s  ", "BALANCE")
	if showBar {
		head += fmt.Sprintf("%-*s ", barW, "KEY LIFE")
	}
	head += fmt.Sprintf("%-11s%-11s", "EXPIRES IN", "LAST VOTE")
	if showProp {
		head += "LAST PROPOSAL"
	}
	b.WriteString("\n" + sSection.Render(" ACCOUNTS ") + sDim.Render(strings.Repeat("─", max(0, w-11))) + "\n")
	b.WriteString(sColHead.Render(head) + "\n")

	count := max(3, m.height-26)
	start := max(0, min(m.selected-count/2, len(m.rows)-count))
	end := min(len(m.rows), start+count)
	now := time.Now()
	for i := start; i < end; i++ {
		r := m.rows[i]
		d := accounts.Derive(r.Account, m.keys, m.status.LastRound, m.average, now)
		sel := i == m.selected
		bg := func(s lipgloss.Style) lipgloss.Style {
			if sel {
				return s.Background(cSelBg)
			}
			return s
		}
		cell := func(s lipgloss.Style, text string) string { return bg(s).Render(text) }

		marker := cell(sDim, "  ")
		if sel {
			marker = cell(sAccent.Bold(true), "▌ ")
		}
		icon, iconStyle := "○", sDim
		switch {
		case r.Err != nil:
			icon, iconStyle = "✖", sBad
		case r.Status == "Online" && len(d.Warnings) > 0:
			icon, iconStyle = "▲", sWarn
		case r.Status == "Online":
			icon, iconStyle = "●", sOK
		}
		addrStyle := sText
		if sel {
			addrStyle = sBold
		}
		line := marker + cell(iconStyle, icon+" ") + cell(addrStyle, fmt.Sprintf("%-17s", short(r.Address)))
		if showLabel {
			line += cell(sAccent, fmt.Sprintf("%-14s", ansi.Truncate(clean(r.Label), 13, "…")))
		}
		line += cell(sText, fmt.Sprintf("%10s  ", compactVOI(r.Amount)))
		left := time.Duration(d.RoundsLeft) * m.average
		known := m.average > 0
		if showBar {
			if d.Last > d.First && d.Last > 0 {
				frac := float64(d.RoundsLeft) / float64(d.Last-d.First)
				var rowBg lipgloss.TerminalColor = lipgloss.NoColor{}
				if sel {
					rowBg = cSelBg
				}
				line += barOn(frac, barW, lifeColor(left, known), rowBg) + cell(sText, " ")
			} else {
				line += cell(sDim, fmt.Sprintf("%-*s ", barW, "no key"))
			}
		}
		expires := "—"
		if d.Last > 0 {
			expires = fmt.Sprintf("%d rnd", d.RoundsLeft)
			if known {
				expires = span(left)
			}
		}
		line += cell(lipgloss.NewStyle().Foreground(lifeColor(left, known && d.Last > 0)), fmt.Sprintf("%-11s", expires))
		vote, voteStyle := m.roundsAgo(d.LastVote), sMuted
		if d.LastVote > 0 && m.status.LastRound-min(m.status.LastRound, d.LastVote) <= 3 {
			vote, voteStyle = "● live", sOK
		} else if r.Status == "Online" {
			voteStyle = sWarn
		}
		line += cell(voteStyle, fmt.Sprintf("%-11s", vote))
		if showProp {
			line += cell(sMuted, fmt.Sprintf("%-13s", m.roundsAgo(d.LastProposal)))
		}
		if sel {
			line += bg(lipgloss.NewStyle()).Render(strings.Repeat(" ", max(0, w-lipgloss.Width(line))))
		}
		b.WriteString(ansi.Truncate(line, w, "…") + "\n")
	}
	if len(m.rows) > end-start {
		b.WriteString(sDim.Render(fmt.Sprintf("    %d–%d of %d", start+1, end, len(m.rows))) + "\n")
	}
	b.WriteString(m.detail(w))
	return b.String()
}

func (m *Model) detail(w int) string {
	a := m.current()
	if a == nil {
		return ""
	}
	now := time.Now()
	d := accounts.Derive(a.Account, m.keys, m.status.LastRound, m.average, now)
	var b strings.Builder
	name := "Account"
	if l := clean(a.Label); l != "" {
		name = l
	}
	b.WriteString(sSection.Render(name) + "  " + sCyan.Render(a.Address) + "\n")

	status := sDim.Render(a.Status)
	if a.Status == "Online" {
		status = sOK.Render("Online")
	}
	incentive := sDim.Render("✗ not incentive eligible")
	if a.IncentiveEligible {
		incentive = sOK.Render("✓ incentive eligible")
	}
	facts := []string{sMuted.Render("balance ") + sText.Render(exactVOI(a.Amount)+" "+m.network().Unit), status, incentive}
	if m.supply.OnlineMoney > 0 && a.Status == "Online" {
		share := float64(a.Amount) / float64(m.supply.OnlineMoney)
		facts = append(facts, sCyan.Render(fmt.Sprintf("%.4f%%", share*100))+sMuted.Render(" of online stake"))
		if m.average > 0 {
			facts = append(facts, sText.Render(fmt.Sprintf("~%.2f", share*float64(24*time.Hour)/float64(m.average)))+sMuted.Render(" blocks/day"))
		}
	}
	b.WriteString(flow(facts, sDim.Render("  ·  "), w-4) + "\n")
	if a.Err != nil {
		b.WriteString(sWarn.Render("▲ Account data is stale: "+clean(a.Err.Error())) + "\n")
	}
	for _, warn := range d.Warnings {
		b.WriteString(sWarn.Bold(true).Render("▲ WARNING: "+warn) + "\n")
	}

	ks := m.accountKeys()
	b.WriteString("\n" + sColHead.Render(fmt.Sprintf("PARTICIPATION KEYS (%d)", len(ks))) + "\n")
	if len(ks) == 0 {
		b.WriteString(sMuted.Render("No keys on this node. Press ") + sKeycap.Render("g") + sMuted.Render(" to generate one.") + "\n")
	}
	for i, k := range ks {
		b.WriteString(m.keyLine(a, k, i == m.keyIndex, w-4))
	}
	switch {
	case m.baseline > 0:
		b.WriteString(sDim.Render(fmt.Sprintf("Dates estimated at %.2fs per block, averaged over the last %s blocks.", m.average.Seconds(), commas(baselineWindow))))
	case m.average > 0:
		b.WriteString(sDim.Render(fmt.Sprintf("Dates estimated from %d live blocks at %.2fs; refining…", len(m.samples), m.average.Seconds())))
	default:
		b.WriteString(sDim.Render("Dates appear once live block times are sampled."))
	}
	return "\n" + panel(cPanel, w).Render(b.String())
}

func (m *Model) keyLine(a *row, k algod.ParticipationKey, selected bool, w int) string {
	marker := "  "
	idStyle := sMuted
	if selected {
		marker = sAccent.Bold(true).Render("▸ ")
		idStyle = sBold
	}
	line := marker + idStyle.Render(short(clean(k.Id)))
	if accounts.Registered(a.Account, k) {
		line += " " + pill("REGISTERED", cOK)
	}
	round := m.status.LastRound
	first, last := k.Key.VoteFirstValid, k.Key.VoteLastValid
	var left uint64
	if last > round {
		left = last - round
	}
	dur := time.Duration(left) * m.average
	if last > first {
		line += "  " + bar(float64(left)/float64(last-first), 20, lifeColor(dur, m.average > 0))
	}
	line += sMuted.Render(fmt.Sprintf("  rounds %s – %s", commas(m.shown(first)), commas(m.shown(last))))
	switch {
	case left == 0:
		line += "  " + sBad.Render("expired")
	case m.average > 0:
		line += "  " + lipgloss.NewStyle().Foreground(lifeColor(dur, true)).Render(span(dur)+" left")
	}
	sub := fmt.Sprintf("    dilution %d  ·  last vote %s  ·  last proposal %s  ·  state proof %s", k.Key.VoteKeyDilution, m.roundsAgo(k.LastVote), m.roundsAgo(k.LastBlockProposal), m.roundsAgo(k.LastStateProof))
	if k.EffectiveFirstValid == nil {
		sub += "  ·  not yet effective"
	}
	return ansi.Truncate(line, w, "…") + "\n" + ansi.Truncate(sDim.Render(sub), w, "…") + "\n"
}

func (m *Model) helpView(w int) string {
	groups := []struct {
		title string
		keys  [][2]string
	}{
		{"NAVIGATE", [][2]string{{"↑ ↓  j k", "Select account"}, {"← →  [ ]", "Select participation key"}, {"h l", "Pan wide content"}, {"PgUp PgDn", "Scroll; Home/End jump"}, {"R", "Refresh everything"}}},
		{"REGISTER", [][2]string{{"⏎  q", "Registration QR + URI for the selected key"}, {"c", "Copy the URI (OSC 52) and show it"}, {"o", "Go-offline QR (no keys)"}}},
		{"MANAGE", [][2]string{{"a", "Add an address (checksum-validated) and label"}, {"g", "Generate / renew a key; edit the round span"}, {"d", "Delete the selected key (confirmed)"}, {"r", "Remove from the watch list"}}},
		{"ALWAYS", [][2]string{{"?", "This help"}, {"esc", "Close, cancel, or offer to abandon a generation wait"}, {"ctrl+c", "Quit"}}},
	}
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(sSection.Render(g.title) + "\n")
		for _, k := range g.keys {
			b.WriteString("  " + sKeycap.Render(fmt.Sprintf("%-10s", k[0])) + "  " + sText.Render(k[1]) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(sMuted.Render(helpText))
	return "\n" + panel(cPanel, min(w, 96)).Padding(1, 2).Render(b.String())
}

const helpText = `Generation and deletion require explicit confirmation. An Online registered key
requires typing its full account address; deleting it stops voting.
Generation waits up to 20 minutes. Esc on the dashboard offers to abandon the wait;
abandoning does not cancel generation inside algod.
Read-only mode blocks all node and watch-list writes.
QRs are signed externally: on Voi in Voi Wallet or voirewards, on Algorand in any
wallet that supports ARC-90 keyreg URIs. f in the QR view adds the 2 VOI / 2 ALGO
fee that requests participation incentives.
This node holds no spending keys, mnemonics, or signing capability.
Network errors stay in the status line; reads retry automatically. Writes do not.`

package ui

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mdp/qrterminal/v3"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/arc90"
)

func (m *Model) showURI(offline, qr bool) bool {
	a := m.current()
	if a == nil {
		m.message = "Select an account first."
		return false
	}
	var p models.AccountParticipation
	if !offline {
		k := m.currentKey()
		if k == nil {
			m.message = "No local key; press g to generate one."
			return false
		}
		p = k.Key
		if m.redact != nil {
			p.VoteFirstValid, p.VoteLastValid = m.shown(p.VoteFirstValid), m.shown(p.VoteLastValid)
		}
	}
	net, _ := arc90.Lookup(m.version.GenesisID)
	var fee uint64
	if m.incentive && net.Algorand && !offline {
		fee = arc90.IncentiveFee
	}
	uri, err := arc90.Build(m.version.GenesisID, a.Address, p, offline, fee)
	if err != nil {
		m.message = err.Error()
		return false
	}
	m.resetScroll()
	m.uri = uri
	m.uriOffline = offline
	m.qr = ""
	m.mode = "uri"
	if qr {
		var b bytes.Buffer
		// Explicit colors preserve contrast with either a light or dark terminal.
		qrterminal.GenerateWithConfig(uri, qrterminal.Config{Writer: &b, Level: qrterminal.L, HalfBlocks: true, QuietZone: 2, BlackChar: "\x1b[30;47m█\x1b[0m", WhiteChar: "\x1b[30;47m \x1b[0m", BlackWhiteChar: "\x1b[30;47m▀\x1b[0m", WhiteBlackChar: "\x1b[30;47m▄\x1b[0m"})
		m.qr = b.String()
		m.mode = "qr"
	}
	m.message = "Sign this URI in " + net.Wallet + ". No spending keys are held here."
	if net.Unit == "VOI" {
		m.message = "Sign this URI in Voi Wallet or paste it into voirewards. No spending keys are held here."
	}
	if offline {
		m.message = "Offline keyreg: signing this URI marks the account Offline."
	}
	return true
}
func (m *Model) beginGenerate() tea.Cmd {
	if m.pending != nil {
		m.message = "Generation is already pending; press Esc on the dashboard to abandon the wait."
		return nil
	}
	if m.statusAt.IsZero() || time.Since(m.statusAt) > 30*time.Second {
		m.message = "Refresh node status before generating a key."
		return nil
	}
	return m.ask("generate-range", "Validity span in rounds (N); dilution is chosen by algod:", "5000000")
}
func (m *Model) beginDelete() {
	m.resetScroll()
	m.mode = "delete-confirm"
	m.prompt = "Delete key " + m.target.Id + "? y/N"
	for _, r := range m.rows {
		if r.Address == m.target.Address && r.Status == "Online" && accounts.Registered(r.Account, m.target) {
			m.ask("delete-address", "WARNING: node stops voting for this account. Type the full address to delete:\n"+m.target.Address, "")
			return
		}
	}
}
func (m *Model) nextRemoval() {
	m.target = m.removalKeys[0]
	m.removalKeys = m.removalKeys[1:]
	m.beginDelete()
}
func (m *Model) modalKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if (m.mode == "help" || m.mode == "qr" || m.mode == "uri") && s == "home" {
		m.viewport.GotoTop()
		return nil
	}
	if (m.mode == "help" || m.mode == "qr" || m.mode == "uri") && s == "end" {
		m.viewport.GotoBottom()
		return nil
	}
	if s == "pgup" || s == "pgdown" {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(k)
		return cmd
	}
	if m.mode == "help" {
		if s == "?" || s == "enter" {
			m.mode = ""
		}
		return nil
	}
	if m.mode == "qr" || m.mode == "uri" {
		if s == "c" {
			m.message = "OSC 52 copy requested; URI remains below for manual copy."
			return clipboard(m.uri)
		}
		if s == "f" {
			if net, _ := arc90.Lookup(m.version.GenesisID); net.Algorand && !m.uriOffline {
				m.incentive = !m.incentive
				m.showURI(false, m.mode == "qr")
				if m.incentive {
					m.message = "Added the 2 ALGO fee that requests participation incentives; the wallet shows it before signing."
				} else {
					m.message = "Removed the incentive fee; the wallet picks the fee."
				}
			}
		}
		return nil
	}
	// Text-entry modes must pass literal characters (including q/g/d) to Bubbles.
	switch m.mode {
	case "add-address", "add-label", "generate-range", "delete-address":
		if s != "enter" {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(k)
			return cmd
		}
		value := strings.TrimSpace(m.input.Value())
		switch m.mode {
		case "add-address":
			if err := accounts.ValidateAddress(value); err != nil {
				m.message = err.Error()
				return nil
			}
			m.address = value
			return m.ask("add-label", "Optional label (Enter to save):", "")
		case "add-label":
			ctxStore, addr, label := m.store, m.address, value
			m.working = true
			m.mode = ""
			return func() tea.Msg { return operationMsg{"add", ctxStore.Add(addr, label)} }
		case "generate-range":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil || n == 0 || n > math.MaxUint64-m.status.LastRound {
				m.message = "Enter a positive round span that fits in uint64."
				return nil
			}
			m.span = n
			m.mode = "generate-confirm"
			m.prompt = fmt.Sprintf("Generate for %s for %d rounds? y/N", m.address, n)
		case "delete-address":
			if value != m.target.Address {
				m.message = "Full address did not match; deletion refused."
				return nil
			}
			return m.delete(value)
		}
		return nil
	}
	if s != "y" && s != "Y" {
		m.mode = ""
		m.removalKeys = nil
		m.message = "Cancelled."
		return nil
	}
	switch m.mode {
	case "abandon-generation":
		m.pending = nil
		m.mode = ""
		m.message = "Generation wait abandoned; this does not cancel generation inside algod. Check the keys before retrying."
	case "offer-generate":
		return m.beginGenerate()
	case "generate-confirm":
		first := m.status.LastRound
		if m.span > math.MaxUint64-first {
			m.mode = ""
			m.message = "Round range overflow; enter a smaller span."
			return nil
		}
		p := &generation{address: m.address, first: first, last: first + m.span, existing: map[string]bool{}, started: time.Now()}
		for _, k := range m.keys {
			p.existing[k.Id] = true
		}
		m.pending = p
		m.working = true
		m.mode = ""
		ctx, c, addr, last := m.ctx, m.client, m.address, p.last
		return func() tea.Msg { return operationMsg{"generate", c.Generate(ctx, addr, first, last)} }
	case "delete-confirm":
		return m.delete("y")
	case "remove-confirm":
		store, addr := m.store, m.address
		m.working = true
		m.mode = ""
		return func() tea.Msg { return operationMsg{"remove", store.Remove(addr)} }
	case "remove-keys":
		m.nextRemoval()
	}
	return nil
}

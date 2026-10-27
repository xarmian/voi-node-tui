// Package ui implements vnode's Bubble Tea dashboard. Commands capture snapshots;
// network commands never mutate the model or block terminal input.
package ui

import (
	"context"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
)

type row struct {
	models.Account
	Label string
	Err   error
}
type generation struct {
	address     string
	first, last uint64
	existing    map[string]bool
	started     time.Time
}
type Model struct {
	ctx                           context.Context
	client                        *algod.Client
	store                         accounts.Store
	readOnly                      bool
	status                        models.NodeStatus
	version                       models.Version
	supply                        models.Supply
	statusAt, roundAt, observedAt time.Time
	samples                       []time.Duration
	average                       time.Duration
	baseline                      time.Duration
	baselineAt                    time.Time
	seeding                       bool
	pulse                         bool
	rows                          []row
	keys                          []algod.ParticipationKey
	selected, keyIndex            int
	width, height                 int
	viewport                      viewport.Model
	mode                          string
	prompt                        string
	message                       string
	netErr, waitErr               string
	uri, qr                       string
	networkWarning                string
	input                         textinput.Model
	address                       string
	target                        algod.ParticipationKey
	removalKeys                   []algod.ParticipationKey
	pending                       *generation
	span                          uint64
	refreshing, working           bool
	lastRefresh                   time.Time
}
type tickMsg time.Time
type waitAgainMsg struct{}
type blockMsg struct {
	status models.NodeStatus
	err    error
	at     time.Time
}
type refreshMsg struct {
	status  *models.NodeStatus
	version *models.Version
	supply  *models.Supply
	keys    []algod.ParticipationKey
	keysOK  bool
	rows    []row
	rowsOK  bool
	err     error
	at      time.Time
}

// baselineMsg carries the mean block time over a long window of real block timestamps.
type baselineMsg struct {
	avg time.Duration
	ok  bool
}
type operationMsg struct {
	action string
	err    error
}

func New(ctx context.Context, c *algod.Client, s accounts.Store, readOnly bool) *Model {
	input := textinput.New()
	input.CharLimit = 200
	input.Width = 72
	return &Model{ctx: ctx, client: c, store: s, readOnly: readOnly, input: input, viewport: viewport.New(120, 25), width: 120, height: 32, message: "Connecting…", span: 5000000}
}
func (m *Model) Init() tea.Cmd { m.refreshing = true; return tea.Batch(m.refresh(), m.wait(), tick()) }
func tick() tea.Cmd            { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = x.Width
		m.height = x.Height
		m.viewport.Width = max(1, x.Width)
		m.viewport.Height = max(1, x.Height-7)
		m.input.Width = max(10, min(72, x.Width-4))
	case refreshMsg:
		m.refreshing = false
		m.applyRefresh(x)
	case blockMsg:
		if x.err != nil {
			m.waitErr = x.err.Error()
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return waitAgainMsg{} })
		}
		m.waitErr = ""
		m.acceptStatus(x.status, x.at, true)
		return m, m.wait()
	case baselineMsg:
		m.seeding = false
		m.baselineAt = time.Now()
		if x.ok {
			m.baseline = x.avg
			m.average = x.avg
		}
	case waitAgainMsg:
		return m, m.wait()
	case tickMsg:
		if p := m.pending; p != nil && time.Time(x).Sub(p.started) >= 20*time.Minute {
			m.pending = nil
			m.message = "No new key appeared after 20m; check the keys list or algod logs."
			if m.mode == "abandon-generation" {
				m.mode = ""
			}
		}
		cmds := []tea.Cmd{tick()}
		if !m.seeding && m.status.LastRound > baselineWindow && (m.baselineAt.IsZero() || time.Since(m.baselineAt) > 10*time.Minute) {
			m.seeding = true
			cmds = append(cmds, m.seedBaseline())
		}
		interval := 5 * time.Second
		if m.pending != nil {
			interval = 2 * time.Second
		}
		if !m.refreshing && time.Since(m.lastRefresh) >= interval {
			m.refreshing = true
			cmds = append(cmds, m.refresh())
		}
		return m, tea.Batch(cmds...)
	case copyMsg:
		if x.err != nil {
			m.message = "Clipboard output failed; copy the displayed URI manually."
		} else {
			m.message = "OSC 52 copy requested; URI is also printed for manual copy."
		}
	case operationMsg:
		m.working = false
		if x.err != nil {
			m.message = x.err.Error()
			if x.action == "generate" {
				m.pending = nil
				m.message += "; check the keys before retrying"
			}
			m.removalKeys = nil
			return m, nil
		}
		switch x.action {
		case "generate":
			if m.pending != nil {
				m.message = "Generation accepted; polling for the new key (can take minutes)."
			}
		case "add":
			m.message = "Account saved."
			m.mode = "offer-generate"
			m.prompt = "Generate a key now? y/N"
		case "remove":
			m.message = "Removed from watch list; accounts with keys remain visible."
			if len(m.removalKeys) > 0 {
				m.mode = "remove-keys"
				m.prompt = "Also delete its local keys? Each key requires confirmation. y/N"
			}
		case "delete":
			m.message = "Key deleted."
			if len(m.removalKeys) > 0 {
				m.nextRemoval()
			}
		}
		if !m.refreshing {
			m.refreshing = true
			return m, m.refresh()
		}
	case tea.KeyMsg:
		if x.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if x.String() == "esc" {
			if m.mode == "" && m.pending != nil {
				m.mode = "abandon-generation"
				m.resetScroll()
				m.prompt = "Abandon waiting for the new key? This does not cancel generation inside algod. y/N"
				return m, nil
			}
			m.mode = ""
			m.removalKeys = nil
			m.input.Blur()
			return m, nil
		}
		if m.mode != "" {
			return m, m.modalKey(x)
		}
		switch x.String() {
		case "up", "k":
			m.selected = max(0, m.selected-1)
			m.keyIndex = 0
			m.viewport.GotoTop()
		case "down", "j":
			m.selected = min(max(0, len(m.rows)-1), m.selected+1)
			m.keyIndex = 0
			m.viewport.GotoTop()
		case "left", "[":
			m.keyIndex = max(0, m.keyIndex-1)
		case "right", "]":
			m.keyIndex = min(max(0, len(m.accountKeys())-1), m.keyIndex+1)
		case "home":
			m.viewport.GotoTop()
		case "end":
			m.viewport.GotoBottom()
		case "pgdown", "pgup":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(x)
			return m, cmd
		case "h":
			m.viewport.ScrollLeft(8)
		case "l":
			m.viewport.ScrollRight(8)
		case "?":
			m.mode = "help"
			m.resetScroll()
		case "R":
			if !m.refreshing {
				m.refreshing = true
				return m, m.refresh()
			}
		case "q", "enter":
			m.showURI(false, true)
		case "c":
			if m.showURI(false, false) {
				return m, clipboard(m.uri)
			}
		case "o":
			m.showURI(true, true)
		case "a", "g", "d", "r":
			if m.readOnly {
				m.message = "Read-only: watch-list and node writes are disabled."
				return m, nil
			}
			if m.working {
				m.message = "A confirmed operation is still running."
				return m, nil
			}
			switch x.String() {
			case "a":
				return m, m.ask("add-address", "Account address:", "")
			case "g":
				if a := m.current(); a != nil {
					m.address = a.Address
					return m, m.beginGenerate()
				}
			case "d":
				if k := m.currentKey(); k != nil {
					m.target = *k
					m.beginDelete()
				}
			case "r":
				if a := m.current(); a != nil {
					m.address = a.Address
					m.removalKeys = append([]algod.ParticipationKey(nil), m.accountKeys()...)
					m.mode = "remove-confirm"
					m.prompt = "Remove " + a.Address + " from the watch list? y/N"
				}
			}
		}
	}
	return m, nil
}
func (m *Model) current() *row {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	return &m.rows[m.selected]
}
func (m *Model) accountKeys() []algod.ParticipationKey {
	a := m.current()
	if a == nil {
		return nil
	}
	var out []algod.ParticipationKey
	for _, k := range m.keys {
		if k.Address == a.Address {
			out = append(out, k)
		}
	}
	return out
}
func (m *Model) currentKey() *algod.ParticipationKey {
	ks := m.accountKeys()
	if len(ks) == 0 {
		return nil
	}
	i := min(m.keyIndex, len(ks)-1)
	return &ks[i]
}
func (m *Model) ask(mode, prompt, value string) tea.Cmd {
	m.mode = mode
	m.resetScroll()
	m.prompt = prompt
	m.input.SetValue(value)
	return m.input.Focus()
}
func (m *Model) acceptStatus(s models.NodeStatus, at time.Time, live bool) {
	if !m.statusAt.IsZero() && s.LastRound < m.status.LastRound {
		return
	}
	if s.LastRound > m.status.LastRound {
		if live && !m.observedAt.IsZero() && s.LastRound-m.status.LastRound == 1 && s.CatchupTime == 0 && m.status.CatchupTime == 0 {
			elapsed := at.Sub(m.observedAt)
			if elapsed > 0 && elapsed < time.Minute {
				m.samples = append(m.samples, elapsed)
				if len(m.samples) > 20 {
					m.samples = m.samples[len(m.samples)-20:]
				}
				var total time.Duration
				for _, d := range m.samples {
					total += d
				}
				m.average = total / time.Duration(len(m.samples))
				if m.baseline > 0 {
					m.average = m.baseline
				}
			}
		}
		m.pulse = !m.pulse
		m.roundAt = at.Add(-time.Duration(min(s.TimeSinceLastRound, uint64(1<<63-1))))
		m.observedAt = at
	}
	if m.roundAt.IsZero() {
		m.roundAt = at.Add(-time.Duration(min(s.TimeSinceLastRound, uint64(1<<63-1))))
	}
	m.status = s
	m.statusAt = at
}

func (m *Model) resetScroll() { m.viewport.GotoTop(); m.viewport.SetXOffset(0) }

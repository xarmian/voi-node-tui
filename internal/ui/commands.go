package ui

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/algorand/go-algorand-sdk/v2/client/v2/common/models"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
)

func (m *Model) wait() tea.Cmd {
	ctx, c, round := m.ctx, m.client, m.status.LastRound
	return func() tea.Msg { s, err := c.Wait(ctx, round); return blockMsg{s, err, time.Now()} }
}
func (m *Model) refresh() tea.Cmd {
	ctx, c, store := m.ctx, m.client, m.store
	oldKeys := append([]algod.ParticipationKey(nil), m.keys...)
	oldRows := append([]row(nil), m.rows...)
	return func() tea.Msg {
		out := refreshMsg{at: time.Now()}
		var errs []error
		s, err := c.Status(ctx)
		if err == nil {
			out.status = &s
		} else {
			errs = append(errs, err)
		}
		v, err := c.Versions(ctx)
		if err == nil {
			out.version = &v
		} else {
			errs = append(errs, err)
		}
		ks, err := c.Keys(ctx)
		if err == nil {
			out.keys = ks
			out.keysOK = true
		} else {
			errs = append(errs, err)
			ks = oldKeys
		}
		watch, err := store.Load()
		if err != nil {
			errs = append(errs, err)
		} else {
			addresses := map[string]string{}
			for _, w := range watch {
				addresses[w.Address] = w.Label
			}
			for _, k := range ks {
				if _, exists := addresses[k.Address]; !exists {
					addresses[k.Address] = ""
				}
			}
			names := make([]string, 0, len(addresses))
			for addr := range addresses {
				names = append(names, addr)
			}
			sort.Strings(names)
			out.rows = make([]row, 0, len(names))
			out.rowsOK = true
			for _, addr := range names {
				a, e := c.Account(ctx, addr)
				if e != nil {
					errs = append(errs, e)
					a = models.Account{Address: addr, Status: "Unknown"}
					for _, old := range oldRows {
						if old.Address == addr {
							a = old.Account
							break
						}
					}
				}
				a.Address = addr
				out.rows = append(out.rows, row{Account: a, Label: addresses[addr], Err: e})
			}
		}
		if len(errs) > 0 {
			out.err = fmt.Errorf("refresh failed: %w", errors.Join(errs...))
		}
		return out
	}
}
func (m *Model) applyRefresh(x refreshMsg) {
	m.lastRefresh = time.Now()
	if x.status != nil {
		m.acceptStatus(*x.status, x.at, false)
	}
	if x.version != nil {
		m.version = *x.version
	}
	addr, keyID := "", ""
	if a := m.current(); a != nil {
		addr = a.Address
	}
	if k := m.currentKey(); k != nil {
		keyID = k.Id
	}
	if x.keysOK {
		m.keys = x.keys
		sort.Slice(m.keys, func(i, j int) bool {
			a, b := m.keys[i], m.keys[j]
			if a.Key.VoteFirstValid != b.Key.VoteFirstValid {
				return a.Key.VoteFirstValid < b.Key.VoteFirstValid
			}
			return a.Id < b.Id
		})
	}
	if x.rowsOK {
		m.rows = x.rows
		m.selected = min(m.selected, max(0, len(m.rows)-1))
		for i := range m.rows {
			if m.rows[i].Address == addr {
				m.selected = i
			}
		}
	}
	m.keyIndex = 0
	for i, k := range m.accountKeys() {
		if k.Id == keyID {
			m.keyIndex = i
		}
	}
	if x.err != nil {
		m.netErr = x.err.Error()
	} else {
		m.netErr = ""
		if m.message == "Connecting…" {
			m.message = "Connected. Select an account; ? for help."
		}
	}
	if p := m.pending; p != nil && x.keysOK {
		for _, k := range m.keys {
			if k.Address == p.address && !p.existing[k.Id] && k.Key.VoteFirstValid == p.first && k.Key.VoteLastValid == p.last {
				m.pending = nil
				m.message = "New key appeared. Select it and press Enter to register it in your wallet."
				for i, r := range m.rows {
					if r.Address == p.address {
						m.selected = i
					}
				}
				for i, key := range m.accountKeys() {
					if key.Id == k.Id {
						m.keyIndex = i
					}
				}
				break
			}
		}
	}
}
func (m *Model) delete(confirmation string) tea.Cmd {
	ctx, c, target := m.ctx, m.client, m.target
	m.working = true
	m.mode = ""
	return func() tea.Msg {
		a, err := c.Account(ctx, target.Address)
		if err == nil {
			err = accounts.CanDelete(a, target, confirmation)
		}
		if err == nil {
			err = c.Delete(ctx, target.Id)
		}
		return operationMsg{"delete", err}
	}
}

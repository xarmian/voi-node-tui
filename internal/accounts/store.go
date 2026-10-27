package accounts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Watch struct {
	Address string `json:"address"`
	Label   string `json:"label,omitempty"`
}
type Store struct{ Path string }

func (s Store) Load() ([]Watch, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []Watch{}, nil
	}
	if err != nil {
		return nil, errors.New("cannot read watch list")
	}
	var xs []Watch
	if json.Unmarshal(b, &xs) != nil {
		return nil, errors.New("invalid accounts.json; repair it before editing the watch list")
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if ValidateAddress(x.Address) != nil || seen[x.Address] {
			return nil, errors.New("invalid or duplicate address in watch list")
		}
		seen[x.Address] = true
	}
	return xs, nil
}
func (s Store) save(xs []Watch) error {
	b, err := json.MarshalIndent(xs, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return errors.New("cannot create watch-list directory")
	}
	f, err := os.CreateTemp(dir, ".accounts-*")
	if err != nil {
		return errors.New("cannot create watch-list file")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.New("cannot save watch list")
	}
	if os.Rename(f.Name(), s.Path) != nil {
		return errors.New("cannot replace watch list")
	}
	return nil
}
func (s Store) Add(address, label string) error {
	if err := ValidateAddress(address); err != nil {
		return err
	}
	xs, err := s.Load()
	if err != nil {
		return err
	}
	for i := range xs {
		if xs[i].Address == address {
			xs[i].Label = label
			return s.save(xs)
		}
	}
	return s.save(append(xs, Watch{Address: address, Label: label}))
}
func (s Store) Remove(address string) error {
	xs, err := s.Load()
	if err != nil {
		return err
	}
	out := make([]Watch, 0, len(xs))
	for _, x := range xs {
		if x.Address != address {
			out = append(out, x)
		}
	}
	return s.save(out)
}

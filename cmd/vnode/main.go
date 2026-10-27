package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/xarmian/voi-node-tui/internal/accounts"
	"github.com/xarmian/voi-node-tui/internal/algod"
	"github.com/xarmian/voi-node-tui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "vnode:", err)
		os.Exit(1)
	}
}
func run() error {
	var o algod.Options
	var readOnly bool
	flag.StringVar(&o.URL, "algod-url", "", "algod HTTP(S) URL (requires --token or --token-file)")
	flag.StringVar(&o.Token, "token", "", "algod admin token; prefer --token-file to avoid process-list exposure")
	flag.StringVar(&o.TokenFile, "token-file", "", "file containing the algod admin token")
	flag.StringVar(&o.DataDir, "data-dir", "", "directory containing algod.net and algod.admin.token")
	flag.BoolVar(&readOnly, "read-only", false, "disable all node and watch-list writes")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments; use --help")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	conn, err := algod.Resolve(ctx, o, algod.OSSystem{})
	if err != nil {
		return err
	}
	c, err := algod.New(conn, readOnly)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot locate home directory")
	}
	store := accounts.Store{Path: filepath.Join(home, ".config", "vnode", "accounts.json")}
	_, err = tea.NewProgram(ui.New(ctx, c, store, readOnly), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if err != nil && ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminal UI failed")
	}
	return nil
}

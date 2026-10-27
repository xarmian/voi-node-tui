package ui

import (
	"encoding/base64"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

type copyMsg struct{ err error }
type clipboardOutput struct {
	uri string
	out io.Writer
}

func (c *clipboardOutput) SetStdin(io.Reader)    {}
func (c *clipboardOutput) SetStderr(io.Writer)   {}
func (c *clipboardOutput) SetStdout(w io.Writer) { c.out = w }
func (c *clipboardOutput) Run() error {
	_, err := fmt.Fprintf(c.out, "\x1b]52;c;%s\a%s\n", base64.StdEncoding.EncodeToString([]byte(c.uri)), c.uri)
	return err
}

// Exec serializes terminal I/O with the renderer. Printf is suppressed in the
// alternate screen, so a short in-process writer emits OSC 52 and a manual copy.
func clipboard(uri string) tea.Cmd {
	return tea.Exec(&clipboardOutput{uri: uri}, func(err error) tea.Msg { return copyMsg{err} })
}

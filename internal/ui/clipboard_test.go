package ui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestClipboardOSC52AndManualURI(t *testing.T) {
	uri := "voi://net:voimain-v1.0/" + testAddress + "?type=keyreg"
	var b bytes.Buffer
	c := &clipboardOutput{uri: uri}
	c.SetStdout(&b)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(uri)) + "\a"
	if !strings.HasPrefix(b.String(), want) || !strings.HasSuffix(b.String(), uri+"\n") {
		t.Fatal("missing OSC 52 or manual URI")
	}
}

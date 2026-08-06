package supervisor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefixStreamsLargeLines(t *testing.T) {
	line := strings.Repeat("x", 128<<10)
	var out bytes.Buffer
	prefix(strings.NewReader(line+"\n"), &out, "api")
	if !strings.Contains(out.String(), "[api] ") || !strings.Contains(out.String(), line[len(line)-1024:]) {
		t.Fatal("large line was truncated")
	}
}

func TestRotatingLogRotatesWhileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.log")
	l, e := newRotatingLog(path, 32, 2)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.Write([]byte(strings.Repeat("a", 24))); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Write([]byte(strings.Repeat("b", 24))); e != nil {
		t.Fatal(e)
	}
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path + ".1"); e != nil {
		t.Fatal("rotated file missing")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != strings.Repeat("b", 24) {
		t.Fatalf("unexpected current log %q", b)
	}
}

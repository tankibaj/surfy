package supervisor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHooksSequentiallyInProjectRoot(t *testing.T) {
	d := t.TempDir()
	var out bytes.Buffer
	err := runHooks(context.Background(), "before_start", []string{
		"printf first > order.txt",
		"printf -- '-second' >> order.txt",
	}, d, &out, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "first-second" {
		t.Fatalf("hook order = %q", b)
	}
	if !strings.Contains(out.String(), "before_start command 2/2 completed") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestRunHooksStopsAfterBeforeStartFailure(t *testing.T) {
	d := t.TempDir()
	var out bytes.Buffer
	err := runHooks(context.Background(), "before_start", []string{
		"exit 7",
		"touch should-not-exist",
	}, d, &out, false)
	if err == nil || !strings.Contains(err.Error(), "before_start command 1/2 failed") {
		t.Fatalf("unexpected error %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(d, "should-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("later hook ran: %v", statErr)
	}
}

func TestRunHooksContinuesAfterShutdownFailure(t *testing.T) {
	d := t.TempDir()
	var out bytes.Buffer
	err := runHooks(context.Background(), "after_stop", []string{
		"exit 9",
		"touch cleaned-up",
	}, d, &out, true)
	if err == nil || !strings.Contains(err.Error(), "after_stop command 1/2 failed") {
		t.Fatalf("unexpected error %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(d, "cleaned-up")); statErr != nil {
		t.Fatalf("later cleanup hook did not run: %v", statErr)
	}
}

func TestPrefixStreamsLargeLines(t *testing.T) {
	line := strings.Repeat("x", 128<<10)
	var out bytes.Buffer
	prefix(strings.NewReader(line+"\n"), &out, "api", false)
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

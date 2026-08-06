package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tankibaj/server-them/internal/manifest"
)

func TestViteDetection(t *testing.T) {
	d := t.TempDir()
	p := `{"scripts":{"dev":"vite"},"devDependencies":{"vite":"latest"}}`
	if e := os.WriteFile(filepath.Join(d, "package.json"), []byte(p), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Service(&manifest.Service{Path: d}, 43121)
	if e != nil {
		t.Fatal(e)
	}
	if r.Runtime != "React/Vite" || !strings.Contains(r.Command, "--port 43121") {
		t.Fatalf("unexpected result: %#v", r)
	}
}
func TestCustomPortExpansion(t *testing.T) {
	r, e := Service(&manifest.Service{Path: t.TempDir(), Run: "./server --port ${PORT}"}, 43210)
	if e != nil {
		t.Fatal(e)
	}
	if r.Command != "./server --port 43210" {
		t.Fatal(r.Command)
	}
}
func TestAmbiguousPython(t *testing.T) {
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, "requirements.txt"), []byte("fastapi"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Service(&manifest.Service{Path: d}, 1234); e == nil || !strings.Contains(e.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity, got %v", e)
	}
}

func TestRejectsUnsafePackageManagerMetadata(t *testing.T) {
	d := t.TempDir()
	p := `{"packageManager":"npm;touch /tmp/owned","scripts":{"dev":"vite"}}`
	if e := os.WriteFile(filepath.Join(d, "package.json"), []byte(p), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Service(&manifest.Service{Path: d}, 43121); e == nil || !strings.Contains(e.Error(), "unsupported packageManager") {
		t.Fatalf("expected validation error, got %v", e)
	}
}

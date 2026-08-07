package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tankibaj/surfy/internal/manifest"
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
func TestMissingPythonEntryPoint(t *testing.T) {
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, "requirements.txt"), []byte("fastapi"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Service(&manifest.Service{Path: d}, 1234); e == nil || !strings.Contains(e.Error(), "entry point") {
		t.Fatalf("expected entry-point error, got %v", e)
	}
}

func TestAmbiguousPythonEntryPoints(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "requirements.txt"), "fastapi\n", 0600)
	mustWrite(t, filepath.Join(d, "main.py"), "app = FastAPI()\n", 0600)
	mustWrite(t, filepath.Join(d, "app", "main.py"), "app = FastAPI()\n", 0600)
	if _, e := Service(&manifest.Service{Path: d}, 1234); e == nil || !strings.Contains(e.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity, got %v", e)
	}
}

func TestFastAPIPackageUsesLocalVenv(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "pyproject.toml"), "[project]\ndependencies = ['fastapi', 'uvicorn']\n", 0600)
	mustWrite(t, filepath.Join(d, "app", "main.py"), "from fastapi import FastAPI\napp = FastAPI()\n", 0600)
	mustWrite(t, filepath.Join(d, ".venv", "bin", "uvicorn"), "#!/bin/sh\n", 0755)
	r, e := Service(&manifest.Service{Path: d}, 43122)
	if e != nil {
		t.Fatal(e)
	}
	want := "./.venv/bin/uvicorn app.main:app --host 127.0.0.1 --port 43122 --reload"
	if r.Runtime != "FastAPI" || r.Command != want {
		t.Fatalf("got %#v, want command %q", r, want)
	}
}

func TestPythonDiscoveryIgnoresVirtualEnvironment(t *testing.T) {
	d := t.TempDir()
	mustWrite(t, filepath.Join(d, "requirements.txt"), "fastapi\n", 0600)
	mustWrite(t, filepath.Join(d, ".venv", "lib", "site-packages", "fake.py"), "app = FastAPI()\n", 0600)
	if _, e := Service(&manifest.Service{Path: d}, 43122); e == nil || !strings.Contains(e.Error(), "entry point") {
		t.Fatalf("expected no application entry point, got %v", e)
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(content), mode); e != nil {
		t.Fatal(e)
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

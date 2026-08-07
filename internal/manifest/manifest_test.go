package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadShorthandAndMapping(t *testing.T) {
	d := t.TempDir()
	for _, x := range []string{"frontend", "backend"} {
		if err := os.Mkdir(filepath.Join(d, x), 0755); err != nil {
			t.Fatal(err)
		}
	}
	f := filepath.Join(d, "surfy.yaml")
	data := "name: demo-app\nweb: ./frontend\napi:\n  path: ./backend\n  run: go run . --port ${PORT}\n  env:\n    APP_ENV: local\n"
	if err := os.WriteFile(f, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	m, e := Load(f)
	if e != nil {
		t.Fatal(e)
	}
	if m.Name != "demo-app" || m.Web.Path != filepath.Join(d, "frontend") || m.API.Env["APP_ENV"] != "local" {
		t.Fatalf("unexpected manifest: %#v", m)
	}
}

func TestLoadLifecycleHooks(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "surfy.yaml")
	data := "name: demo-app\nbefore_start:\n  - docker compose up -d --wait database\n  - npm run db:migrate\nafter_stop:\n  - docker compose stop database\nweb: .\n"
	if err := os.WriteFile(f, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.BeforeStart, "|"); got != "docker compose up -d --wait database|npm run db:migrate" {
		t.Fatalf("before_start = %q", got)
	}
	if got := strings.Join(m.AfterStop, "|"); got != "docker compose stop database" {
		t.Fatalf("after_stop = %q", got)
	}
}

func TestRejectsMalformedLifecycleHooks(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "surfy.yaml")
	for _, data := range []string{
		"before_start:\nweb: .\n",
		"before_start:\n  command: docker compose up\nweb: .\n",
		"before_start:\n  - \nweb: .\n",
	} {
		if err := os.WriteFile(f, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(f); err == nil || !strings.Contains(err.Error(), "before_start") {
			t.Fatalf("expected before_start validation error for %q, got %v", data, err)
		}
	}
}
func TestRejectsInvalidNameAndUnknownField(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "surfy.yaml")
	if err := os.WriteFile(f, []byte("name: Bad Name\nweb: .\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, e := Load(f); e == nil || !strings.Contains(e.Error(), "invalid project name") {
		t.Fatalf("expected invalid name, got %v", e)
	}
	if err := os.WriteFile(f, []byte("web: .\nroute: /bad\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, e := Load(f); e == nil || !strings.Contains(e.Error(), "unsupported field") {
		t.Fatalf("expected unknown field, got %v", e)
	}
}
func TestFindWalksParents(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "surfy.yaml")
	if err := os.WriteFile(f, []byte("web: .\n"), 0600); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(d, "a", "b")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	got, e := Find(child)
	if e != nil {
		t.Fatal(e)
	}
	want, e := filepath.EvalSymlinks(f)
	if e != nil {
		t.Fatal(e)
	}
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

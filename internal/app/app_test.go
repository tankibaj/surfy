package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureUserStateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom", "serve-them")
	if e := ensureUserStateDir(dir); e != nil {
		t.Fatal(e)
	}
	st, e := os.Stat(dir)
	if e != nil {
		t.Fatal(e)
	}
	if st.Mode().Perm() != 0700 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
}
func TestEnsureUserStateDirRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if e := os.Mkdir(target, 0755); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "serve-them")
	if e := os.Symlink(target, dir); e != nil {
		t.Fatal(e)
	}
	if e := ensureUserStateDir(dir); e == nil {
		t.Fatal("expected symlink rejection")
	}
}
func TestEnsureUserStateDirRejectsSymlinkedRegistry(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "serve-them")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("/tmp/target", filepath.Join(dir, "registry.json")); e != nil {
		t.Fatal(e)
	}
	if e := ensureUserStateDir(dir); e == nil {
		t.Fatal("expected registry symlink rejection")
	}
}

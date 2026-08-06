package app

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHelpDescribesCommands(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"serve-them [path]", "start [path]", "Start a project in the background.", "logs [web|api] [--follow]", "Stream or print project logs.", "certificate status", "Show local certificate authority status.", "uninstall [--remove-ca]", "Remove Serve Them and optionally its trusted CA."} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("help does not contain %q:\n%s", expected, out.String())
		}
	}
}

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

func TestValidatedInvokingUser(t *testing.T) {
	env := map[string]string{"SUDO_UID": "501", "SUDO_GID": "20"}
	lookup := func(id string) (*user.User, error) {
		if id != "501" {
			t.Fatalf("lookup id %q", id)
		}
		return &user.User{Uid: "501", Gid: "20", HomeDir: "/Users/naim"}, nil
	}
	got, err := validatedInvokingUser(func(k string) string { return env[k] }, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if got.uid != 501 || got.gid != 20 || got.home != "/Users/naim" {
		t.Fatalf("identity %+v", got)
	}
}

func TestParseDarwinUserRecord(t *testing.T) {
	got, err := parseDarwinUserRecord("naim:*:501:20:staff:0:0:Naim Ahmmed:/Users/naim:/bin/zsh\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "naim" || got.Uid != "501" || got.Gid != "20" || got.HomeDir != "/Users/naim" {
		t.Fatalf("account %+v", got)
	}
	if _, err = parseDarwinUserRecord("not-a-passwd-record"); err == nil {
		t.Fatal("expected malformed record rejection")
	}
}

func TestValidatedInvokingUserRejectsUnsafeIdentity(t *testing.T) {
	tests := []struct {
		name string
		uid  string
		gid  string
		user *user.User
	}{
		{name: "missing uid", gid: "20"},
		{name: "invalid uid", uid: "five", gid: "20"},
		{name: "root uid", uid: "0", gid: "20"},
		{name: "missing gid", uid: "501"},
		{name: "invalid gid", uid: "501", gid: "staff"},
		{name: "root gid", uid: "501", gid: "0"},
		{name: "uid mismatch", uid: "501", gid: "20", user: &user.User{Uid: "502", Gid: "20", HomeDir: "/Users/other"}},
		{name: "gid mismatch", uid: "501", gid: "20", user: &user.User{Uid: "501", Gid: "21", HomeDir: "/Users/naim"}},
		{name: "relative home", uid: "501", gid: "20", user: &user.User{Uid: "501", Gid: "20", HomeDir: "Users/naim"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(string) (*user.User, error) {
				if tt.user == nil {
					return &user.User{Uid: "501", Gid: "20", HomeDir: "/Users/naim"}, nil
				}
				return tt.user, nil
			}
			_, err := validatedInvokingUser(func(k string) string {
				if k == "SUDO_UID" {
					return tt.uid
				}
				return tt.gid
			}, lookup)
			if err == nil {
				t.Fatal("expected identity rejection")
			}
		})
	}
}

func TestMigrateLegacyUserStateChangesOnlyKnownDirectEntries(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "Library", "Application Support", "serve-them")
	if err = os.MkdirAll(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"registry.json", "supervisor.log", "edge.sock", filepath.Join("nested", "untouched")} {
		if err = os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var changed []string
	err = migrateLegacyUserState(home, 501, 20, nil, func(file *os.File, uid, gid int) error {
		if uid != 501 || gid != 20 {
			t.Fatalf("ownership %d:%d", uid, gid)
		}
		changed = append(changed, filepath.Clean(file.Name()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "registry.json"), filepath.Join(dir, "supervisor.log"), dir}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed\n%q\nwant\n%q", changed, want)
	}
}

func TestMigrateLegacyUserStateRejectsSymlinks(t *testing.T) {
	tests := []string{"state", "registry"}
	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			home, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, "Library", "Application Support", "serve-them")
			target := filepath.Join(home, "target")
			if err = os.MkdirAll(target, 0700); err != nil {
				t.Fatal(err)
			}
			if test == "state" {
				if err = os.MkdirAll(filepath.Dir(dir), 0700); err == nil {
					err = os.Symlink(target, dir)
				}
			} else {
				if err = os.MkdirAll(dir, 0700); err == nil {
					err = os.Symlink(target, filepath.Join(dir, "registry.json"))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = migrateLegacyUserState(home, 501, 20, nil, func(*os.File, int, int) error { return nil }); err == nil {
				t.Fatal("expected symlink rejection")
			}
		})
	}
}

func TestMigrateLegacyUserStateRejectsSymlinkedHomeParent(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realParent := filepath.Join(base, "real")
	home := filepath.Join(base, "alias", "user")
	if err = os.MkdirAll(canonicalMacOSStateDir(filepath.Join(realParent, "user")), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(realParent, filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	if err = migrateLegacyUserState(home, 501, 20, nil, func(*os.File, int, int) error { return nil }); err == nil {
		t.Fatal("expected symlinked home parent rejection")
	}
}

func TestMigrateLegacyUserStateRejectsHardLinkedFile(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := canonicalMacOSStateDir(home)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(dir, "registry.json")
	if err = os.WriteFile(registry, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(registry, filepath.Join(dir, "second-link")); err != nil {
		t.Fatal(err)
	}
	chowns := 0
	err = migrateLegacyUserState(home, 501, 20, nil, func(*os.File, int, int) error {
		chowns++
		return nil
	})
	if err == nil {
		t.Fatal("expected hard-link rejection")
	}
	if chowns != 0 {
		t.Fatalf("unexpected ownership changes: %d", chowns)
	}
}

func TestMigrateLegacyUserStateRejectsReplacedInode(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := canonicalMacOSStateDir(home)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(dir, "registry.json")
	replacement := filepath.Join(dir, "replacement")
	if err = os.WriteFile(registry, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(replacement, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	chowns := 0
	err = migrateLegacyUserState(home, 501, 20, func(root *os.Root, name string) error {
		if name != "registry.json" {
			return nil
		}
		if err := root.Remove(name); err != nil {
			return err
		}
		return root.Rename("replacement", name)
	}, func(*os.File, int, int) error {
		chowns++
		return nil
	})
	if err == nil {
		t.Fatal("expected replaced inode rejection")
	}
	if chowns != 0 {
		t.Fatalf("unexpected ownership changes: %d", chowns)
	}
}

func TestPrepareUserStateMigratesOnlyPermissionFailureAtDefaultPath(t *testing.T) {
	home := "/Users/naim"
	permission := &os.PathError{Op: "chmod", Path: canonicalMacOSStateDir(home), Err: os.ErrPermission}
	tests := []struct {
		name       string
		dir        string
		prepareErr error
		wantRun    bool
	}{
		{name: "default permission", dir: canonicalMacOSStateDir(home), prepareErr: permission, wantRun: true},
		{name: "custom permission", dir: "/Volumes/state/serve-them", prepareErr: permission},
		{name: "default symlink", dir: canonicalMacOSStateDir(home), prepareErr: errors.New("state directory must not be a symlink")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			migrations := 0
			err := prepareUserStateDirWith(tt.dir, home, func(string) error {
				calls++
				if calls == 1 {
					return tt.prepareErr
				}
				return nil
			}, func() error {
				migrations++
				return nil
			})
			if tt.wantRun && err != nil {
				t.Fatal(err)
			}
			if !tt.wantRun && err == nil {
				t.Fatal("expected preparation failure")
			}
			wantCalls := 1
			if tt.wantRun {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("prepare calls %d, want %d", calls, wantCalls)
			}
			wantMigrations := 0
			if tt.wantRun {
				wantMigrations = 1
			}
			if migrations != wantMigrations {
				t.Fatalf("migrations %d, want %d", migrations, wantMigrations)
			}
		})
	}
}

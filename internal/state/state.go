package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
)

type Service struct {
	Port   int    `json:"port"`
	PID    int    `json:"pid"`
	Status string `json:"status"`
	Log    string `json:"log,omitempty"`
}
type Project struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Manifest       string    `json:"manifest"`
	Status         string    `json:"status"`
	Enabled        bool      `json:"enabled"`
	SupervisorPID  int       `json:"supervisor_pid,omitempty"`
	SupervisorMode string    `json:"supervisor_mode,omitempty"`
	Web            *Service  `json:"web,omitempty"`
	API            *Service  `json:"api,omitempty"`
	Updated        time.Time `json:"updated"`
}
type Registry struct {
	Projects map[string]*Project `json:"projects"`
}

func Dir() (string, error) {
	if v := os.Getenv("SERVE_THEM_STATE_DIR"); v != "" {
		return filepath.Abs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "serve-them"), nil
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "serve-them"), nil
	}
	return filepath.Join(home, ".local", "state", "serve-them"), nil
}

func LogDir() (string, error) {
	if v := os.Getenv("SERVE_THEM_LOG_DIR"); v != "" {
		return filepath.Abs(v)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "serve-them"), nil
	}
	d, e := Dir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "logs"), nil
}

func ProjectID(manifest string) string {
	b := sha256.Sum256([]byte(manifest))
	return hex.EncodeToString(b[:12])
}

func Read() (*Registry, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(d, "registry.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &Registry{Projects: map[string]*Project{}}, nil
	}
	if err != nil {
		return nil, err
	}
	r := &Registry{}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, err
	}
	if r.Projects == nil {
		r.Projects = map[string]*Project{}
	}
	return r, nil
}
func RegistryFile() (string, error) {
	d, e := Dir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "registry.json"), nil
}

func Update(fn func(*Registry) error) error {
	d, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(d, "registry.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	r, err := Read()
	if err != nil {
		return err
	}
	if err := fn(r); err != nil {
		return err
	}
	if len(r.Projects) > 256 {
		return errors.New("project registry limit of 256 reached")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(d, "registry.json.tmp")
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(d, "registry.json")); err != nil {
		return err
	}
	notifyEdge(filepath.Join(d, "edge.sock"))
	return nil
}
func notifyEdge(path string) {
	c, e := net.DialTimeout("unix", path, 25*time.Millisecond)
	if e != nil {
		return
	}
	defer c.Close()
	_ = c.SetWriteDeadline(time.Now().Add(25 * time.Millisecond))
	_, _ = c.Write([]byte("RELOAD\n"))
}

func FindByManifest(r *Registry, manifest string) *Project {
	id := ProjectID(manifest)
	return r.Projects[id]
}
func Sorted(r *Registry) []*Project {
	out := make([]*Project, 0, len(r.Projects))
	for _, p := range r.Projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

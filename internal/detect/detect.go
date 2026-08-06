package detect

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tankibaj/server-them/internal/manifest"
)

type Result struct{ Command, Runtime, Confidence string }

func Service(svc *manifest.Service, port int) (Result, error) {
	if svc.Run != "" {
		return Result{expandPort(svc.Run, port), "custom", "confident"}, nil
	}
	if exists(svc.Path, "package.json") {
		return node(svc.Path, port)
	}
	if exists(svc.Path, "go.mod") {
		return Result{fmt.Sprintf("go run ."), "Go", "confident"}, nil
	}
	if exists(svc.Path, "manage.py") {
		return Result{fmt.Sprintf("python3 manage.py runserver 127.0.0.1:%d", port), "Django", "confident"}, nil
	}
	if exists(svc.Path, "pyproject.toml") || exists(svc.Path, "requirements.txt") || exists(svc.Path, "Pipfile") {
		return python(svc.Path, port)
	}
	return Result{}, fmt.Errorf("unable to determine how to start %s\n\nAdd a command:\n\napi:\n  path: %s\n  run: <command>", svc.Path, svc.Path)
}

func node(dir string, port int) (Result, error) {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return Result{}, err
	}
	var p struct {
		PackageManager  string            `json:"packageManager"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return Result{}, fmt.Errorf("invalid package.json: %w", err)
	}
	pm := strings.Split(p.PackageManager, "@")[0]
	if pm != "" && !allowedPM[pm] {
		return Result{}, fmt.Errorf("unsupported packageManager %q", pm)
	}
	if pm == "" {
		for _, x := range []struct{ f, p string }{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}, {"bun.lockb", "bun"}, {"package-lock.json", "npm"}} {
			if exists(dir, x.f) {
				pm = x.p
				break
			}
		}
	}
	if pm == "" {
		pm = "npm"
	}
	script := ""
	if _, ok := p.Scripts["dev"]; ok {
		script = "dev"
	} else if _, ok := p.Scripts["start"]; ok {
		script = "start"
	}
	if script == "" {
		return Result{}, fmt.Errorf("Node.js project has no dev or start script; add a run override")
	}
	cmd := pm + " run " + script
	if pm == "yarn" {
		cmd = "yarn " + script
	}
	all := map[string]string{}
	for k, v := range p.Dependencies {
		all[k] = v
	}
	for k, v := range p.DevDependencies {
		all[k] = v
	}
	runtime := "Node.js"
	if _, ok := all["vite"]; ok {
		runtime = "React/Vite"
		cmd += fmt.Sprintf(" -- --host 127.0.0.1 --port %d", port)
	}
	return Result{cmd, runtime, "confident"}, nil
}

func python(dir string, port int) (Result, error) {
	entries, _ := filepath.Glob(filepath.Join(dir, "*.py"))
	reFast := regexp.MustCompile(`(?m)^\s*app\s*=\s*FastAPI\s*\(`)
	reFlask := regexp.MustCompile(`(?m)^\s*app\s*=\s*Flask\s*\(`)
	var matches []Result
	for _, f := range entries {
		b, _ := os.ReadFile(f)
		mod := strings.TrimSuffix(filepath.Base(f), ".py")
		if reFast.Match(b) {
			matches = append(matches, Result{fmt.Sprintf("python3 -m uvicorn %s:app --host 127.0.0.1 --port %d", mod, port), "FastAPI", "confident"})
		}
		if reFlask.Match(b) {
			matches = append(matches, Result{fmt.Sprintf("python3 -m flask --app %s:app run --host 127.0.0.1 --port %d", mod, port), "Flask", "confident"})
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return Result{}, fmt.Errorf("Python application entry point is ambiguous; add a run override")
}

func expandPort(s string, port int) string { return strings.ReplaceAll(s, "${PORT}", fmt.Sprint(port)) }
func exists(dir, name string) bool         { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }

var allowedPM = map[string]bool{"npm": true, "pnpm": true, "yarn": true, "bun": true}

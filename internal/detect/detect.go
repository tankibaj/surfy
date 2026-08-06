package detect

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	type match struct{ framework, module string }
	var matches []match
	reFast := regexp.MustCompile(`(?m)^\s*app(?:\s*:[^=]+)?\s*=\s*FastAPI\s*\(`)
	reFlask := regexp.MustCompile(`(?m)^\s*app(?:\s*:[^=]+)?\s*=\s*Flask\s*\(`)
	excluded := map[string]bool{".venv": true, "venv": true, "env": true, ".git": true, "__pycache__": true, "site-packages": true, "tests": true, "test": true, "migrations": true}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if entry.IsDir() {
			if rel != "." && (excluded[entry.Name()] || len(parts) > 4) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".py" || len(parts) > 4 {
			return nil
		}
		module, ok := pythonModule(parts)
		if !ok {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if reFast.Match(b) {
			matches = append(matches, match{"FastAPI", module})
		}
		if reFlask.Match(b) {
			matches = append(matches, match{"Flask", module})
		}
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("inspect Python project: %w", err)
	}
	if len(matches) == 0 {
		return Result{}, fmt.Errorf("unable to identify a FastAPI or Flask application entry point; add a run override")
	}
	if len(matches) > 1 {
		modules := make([]string, 0, len(matches))
		for _, m := range matches {
			modules = append(modules, m.module+":app")
		}
		sort.Strings(modules)
		return Result{}, fmt.Errorf("Python application entry point is ambiguous (%s); add a run override", strings.Join(modules, ", "))
	}
	m := matches[0]
	if m.framework == "FastAPI" {
		runner := pythonRunner(dir, "uvicorn")
		return Result{fmt.Sprintf("%s %s:app --host 127.0.0.1 --port %d --reload", runner, m.module, port), "FastAPI", "confident"}, nil
	}
	runner := pythonRunner(dir, "flask")
	return Result{fmt.Sprintf("%s --app %s:app run --host 127.0.0.1 --port %d", runner, m.module, port), "Flask", "confident"}, nil
}

func pythonModule(parts []string) (string, bool) {
	last := parts[len(parts)-1]
	parts[len(parts)-1] = strings.TrimSuffix(last, ".py")
	if parts[len(parts)-1] == "__init__" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return "", false
	}
	identifier := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	for _, part := range parts {
		if !identifier.MatchString(part) {
			return "", false
		}
	}
	return strings.Join(parts, "."), true
}
func pythonRunner(dir, tool string) string {
	for _, venv := range []string{".venv", "venv"} {
		toolPath := filepath.Join(dir, venv, "bin", tool)
		if executable(toolPath) {
			return "./" + filepath.ToSlash(filepath.Join(venv, "bin", tool))
		}
		pythonPath := filepath.Join(dir, venv, "bin", "python")
		if executable(pythonPath) {
			return "./" + filepath.ToSlash(filepath.Join(venv, "bin", "python")) + " -m " + tool
		}
	}
	return "python3 -m " + tool
}
func executable(path string) bool {
	info, e := os.Stat(path)
	return e == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func expandPort(s string, port int) string { return strings.ReplaceAll(s, "${PORT}", fmt.Sprint(port)) }
func exists(dir, name string) bool         { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }

var allowedPM = map[string]bool{"npm": true, "pnpm": true, "yarn": true, "bun": true}

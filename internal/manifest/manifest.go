package manifest

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Service struct {
	Path string
	Run  string
	Env  map[string]string
}
type Manifest struct {
	File        string
	Name        string
	BeforeStart []string
	AfterStop   []string
	Web         *Service
	API         *Service
}

var hostnameLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Find(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(abs, "surfy.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.EvalSymlinks(candidate)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return "", errors.New("no surfy.yaml was found\n\nCreate one:\n\nweb: ./frontend\napi: ./backend")
}

// Load parses the deliberately small MVP schema without accepting arbitrary YAML tags, aliases, or object types.
func Load(file string) (*Manifest, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := &Manifest{File: file}
	var current *Service
	section := ""
	hookSections := map[string]*[]string{
		"before_start": &m.BeforeStart,
		"after_stop":   &m.AfterStop,
	}
	hookSeen := map[string]bool{}
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimRight(s.Text(), " \t\r")
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if strings.Contains(raw[:indent], "\t") {
			return nil, fmt.Errorf("%s:%d: tabs are not allowed", file, lineNo)
		}
		if indent == 2 {
			if commands := hookSections[section]; commands != nil {
				trimmed := strings.TrimSpace(raw)
				if trimmed != "-" && !strings.HasPrefix(trimmed, "- ") {
					return nil, fmt.Errorf("%s:%d: %s must be a command list", file, lineNo, section)
				}
				item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				item, err = scalar(item)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", file, lineNo, err)
				}
				if item == "" {
					return nil, fmt.Errorf("%s:%d: %s commands must not be empty", file, lineNo, section)
				}
				*commands = append(*commands, item)
				continue
			}
		}
		key, value, ok := splitKV(strings.TrimSpace(raw))
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected key: value", file, lineNo)
		}
		value, err = scalar(value)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", file, lineNo, err)
		}
		switch indent {
		case 0:
			current, section = nil, ""
			switch key {
			case "name":
				m.Name = value
			case "before_start", "after_stop":
				if value != "" {
					return nil, fmt.Errorf("%s:%d: %s must be a command list", file, lineNo, key)
				}
				section = key
				hookSeen[key] = true
			case "web", "api":
				svc := &Service{Env: map[string]string{}}
				if value != "" {
					svc.Path = value
				}
				if key == "web" {
					m.Web = svc
				} else {
					m.API = svc
				}
				current = svc
				section = key
			default:
				return nil, fmt.Errorf("%s:%d: unsupported field %q", file, lineNo, key)
			}
		case 2:
			if current == nil {
				return nil, fmt.Errorf("%s:%d: field outside service", file, lineNo)
			}
			switch key {
			case "path":
				current.Path = value
			case "run":
				current.Run = value
			case "env":
				if value != "" {
					return nil, errors.New("env must be a mapping")
				}
				section += ".env"
			default:
				return nil, fmt.Errorf("%s:%d: unsupported service field %q", file, lineNo, key)
			}
		case 4:
			if current == nil || !strings.HasSuffix(section, ".env") {
				return nil, fmt.Errorf("%s:%d: invalid indentation", file, lineNo)
			}
			if !environmentName.MatchString(key) {
				return nil, fmt.Errorf("invalid environment variable name %q", key)
			}
			current.Env[key] = value
		default:
			return nil, fmt.Errorf("%s:%d: invalid indentation", file, lineNo)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	for name, commands := range hookSections {
		if hookSeen[name] && len(*commands) == 0 {
			return nil, fmt.Errorf("%s must contain at least one command", name)
		}
	}
	if m.Web == nil && m.API == nil {
		return nil, errors.New("surfy.yaml must define web, api, or both")
	}
	root := filepath.Dir(file)
	for label, svc := range map[string]*Service{"web": m.Web, "api": m.API} {
		if svc == nil {
			continue
		}
		if svc.Path == "" {
			return nil, fmt.Errorf("%s.path is required", label)
		}
		if !filepath.IsAbs(svc.Path) {
			svc.Path = filepath.Join(root, svc.Path)
		}
		svc.Path, err = filepath.Abs(filepath.Clean(svc.Path))
		if err != nil {
			return nil, err
		}
		info, statErr := os.Stat(svc.Path)
		if statErr != nil || !info.IsDir() {
			return nil, fmt.Errorf("%s path is not a directory: %s", label, svc.Path)
		}
	}
	if m.Name == "" {
		m.Name = filepath.Base(root)
	}
	m.Name = strings.ToLower(m.Name)
	if !hostnameLabel.MatchString(m.Name) {
		return nil, fmt.Errorf("invalid project name %q: use lowercase letters, digits, and hyphens", m.Name)
	}
	return m, nil
}

func splitKV(s string) (string, string, bool) {
	i := strings.IndexByte(s, ':')
	if i < 1 {
		return "", "", false
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), true
}
func scalar(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") || strings.HasPrefix(v, "&") || strings.HasPrefix(v, "*") || strings.HasPrefix(v, "!") {
		return "", errors.New("only scalar values are supported")
	}
	if v[0] == '\'' || v[0] == '"' {
		if len(v) < 2 || v[len(v)-1] != v[0] {
			return "", errors.New("unterminated quoted value")
		}
		return v[1 : len(v)-1], nil
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), nil
}

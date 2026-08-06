package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tankibaj/server-them/internal/buildinfo"
	"github.com/tankibaj/server-them/internal/cert"
	"github.com/tankibaj/server-them/internal/edge"
	"github.com/tankibaj/server-them/internal/manifest"
	"github.com/tankibaj/server-them/internal/state"
	"github.com/tankibaj/server-them/internal/supervisor"
)

const version = buildinfo.Version

func Run(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return foreground(".", out)
	}
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "start":
		path := argPath(rest)
		mf, e := manifest.Find(path)
		if e != nil {
			return e
		}
		m, e := manifest.Load(mf)
		if e != nil {
			return e
		}
		r, e := state.Read()
		if e != nil {
			return e
		}
		if p := state.FindByManifest(r, mf); p != nil && supervisorAlive(p) {
			return fmt.Errorf("%s is already running", m.Name)
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		c := exec.Command(exe, "_supervise", mf)
		d, _ := state.Dir()
		_ = os.MkdirAll(d, 0700)
		f, e := os.OpenFile(filepath.Join(d, "supervisor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		c.Stdout = f
		c.Stderr = f
		c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if e = c.Start(); e != nil {
			return e
		}
		pid := c.Process.Pid
		_ = c.Process.Release()
		for i := 0; i < 2600; i++ {
			time.Sleep(50 * time.Millisecond)
			if !alive(pid) {
				return fmt.Errorf("%s failed to start; see %s", m.Name, filepath.Join(d, "supervisor.log"))
			}
			r, _ = state.Read()
			if p := state.FindByManifest(r, mf); p != nil && p.Status == "running" {
				fmt.Fprintf(out, "✓ %s started\n  https://%s.localhost\n", m.Name, m.Name)
				return nil
			}
		}
		return fmt.Errorf("%s did not start; see %s", m.Name, filepath.Join(d, "supervisor.log"))
	case "_supervise":
		if len(rest) != 1 {
			return errors.New("internal supervisor requires manifest")
		}
		return supervisor.Run(rest[0], true, out)
	case "_init-edge":
		if os.Geteuid() != 0 {
			return errors.New("edge initialization must run as root")
		}
		_ = os.Setenv("SERVE_THEM_CERT_DIR", "/Library/Application Support/serve-them/certificates")
		_, _, e := cert.EnsureCA()
		return e
	case "_repair-edge":
		if os.Geteuid() != 0 {
			return errors.New("edge repair must run as root")
		}
		certDir := "/Library/Application Support/serve-them/certificates"
		if e := os.RemoveAll(certDir); e != nil {
			return e
		}
		_ = os.Setenv("SERVE_THEM_CERT_DIR", certDir)
		_, _, e := cert.EnsureCA()
		return e
	case "edge":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return edge.Run(ctx)
	case "stop":
		return stop(argPath(rest), out)
	case "restart":
		if e := stop(argPath(rest), io.Discard); e != nil && !strings.Contains(e.Error(), "not running") {
			return e
		}
		return Run(append([]string{"start"}, rest...), out, errOut)
	case "status":
		return status(argPath(rest), out)
	case "list":
		return list(out, len(rest) > 0 && rest[0] == "--enabled")
	case "logs":
		return logs(rest, out)
	case "open":
		mf, e := manifest.Find(argPath(rest))
		if e != nil {
			return e
		}
		m, e := manifest.Load(mf)
		if e != nil {
			return e
		}
		return exec.Command("open", "https://"+m.Name+".localhost").Run()
	case "enable":
		return enable(argPath(rest), true, out)
	case "disable":
		return enable(argPath(rest), false, out)
	case "doctor":
		return doctor(out)
	case "install":
		return install(out)
	case "uninstall":
		return uninstall(rest, out)
	case "certificate":
		return certificate(rest, out)
	case "version", "--version", "-v":
		fmt.Fprintln(out, version)
		return nil
	case "help", "--help", "-h":
		usage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q (run serve-them help)", cmd)
	}
}

func foreground(path string, out io.Writer) error {
	mf, e := manifest.Find(path)
	if e != nil {
		return e
	}
	return supervisor.Run(mf, false, out)
}
func argPath(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return "."
}
func stop(path string, out io.Writer) error {
	mf, e := manifest.Find(path)
	if e != nil {
		return e
	}
	r, e := state.Read()
	if e != nil {
		return e
	}
	p := state.FindByManifest(r, mf)
	if p == nil || !supervisorAlive(p) {
		return errors.New("project is not running")
	}
	if e = syscall.Kill(p.SupervisorPID, syscall.SIGTERM); e != nil {
		return e
	}
	for i := 0; i < 50 && alive(p.SupervisorPID); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(out, "✓ %s stopped\n", p.Name)
	return nil
}
func status(path string, out io.Writer) error {
	mf, e := manifest.Find(path)
	if e != nil {
		return e
	}
	r, e := state.Read()
	if e != nil {
		return e
	}
	p := state.FindByManifest(r, mf)
	if p == nil {
		return errors.New("project has not been registered")
	}
	s := "stopped"
	if supervisorAlive(p) {
		s = p.Status
		if s == "starting" {
			s = "running"
		}
	}
	fmt.Fprintf(out, "Project: %s\nStatus: %s\nURL: https://%s.localhost\n\n", p.Name, s, p.Name)
	for _, x := range []struct {
		n string
		s *state.Service
	}{{"web", p.Web}, {"api", p.API}} {
		if x.s != nil {
			st := "stopped"
			if alive(x.s.PID) {
				st = "running"
			}
			fmt.Fprintf(out, "%-6s %-10s pid %d\n", x.n, st, x.s.PID)
		}
	}
	return nil
}
func list(out io.Writer, enabled bool) error {
	r, e := state.Read()
	if e != nil {
		return e
	}
	fmt.Fprintln(out, "NAME                 URL                                      STATUS")
	for _, p := range state.Sorted(r) {
		if enabled && !p.Enabled {
			continue
		}
		st := "stopped"
		if supervisorAlive(p) {
			st = p.Status
			if st == "starting" {
				st = "running"
			}
		}
		fmt.Fprintf(out, "%-20s %-40s %s\n", p.Name, "https://"+p.Name+".localhost", st)
	}
	return nil
}
func logs(args []string, out io.Writer) error {
	follow := false
	service := ""
	for _, a := range args {
		if a == "--follow" || a == "-f" {
			follow = true
		} else if a == "web" || a == "api" {
			service = a
		}
	}
	mf, e := manifest.Find(".")
	if e != nil {
		return e
	}
	r, e := state.Read()
	if e != nil {
		return e
	}
	p := state.FindByManifest(r, mf)
	if p == nil {
		return errors.New("project has no logs")
	}
	var files []string
	for _, x := range []struct {
		n string
		s *state.Service
	}{{"web", p.Web}, {"api", p.API}} {
		if x.s != nil && x.s.Log != "" && (service == "" || service == x.n) {
			files = append(files, x.s.Log)
		}
	}
	if len(files) == 0 {
		return errors.New("no background logs found")
	}
	offsets := map[string]int64{}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		_, _ = out.Write(b)
		offsets[f] = int64(len(b))
	}
	if follow {
		for {
			time.Sleep(250 * time.Millisecond)
			for _, f := range files {
				fh, e := os.Open(f)
				if e != nil {
					continue
				}
				st, e := fh.Stat()
				if e == nil && st.Size() < offsets[f] {
					offsets[f] = 0
				}
				b, e := io.ReadAll(io.NewSectionReader(fh, offsets[f], 1<<20))
				_ = fh.Close()
				if e == nil && len(b) > 0 {
					_, _ = out.Write(b)
					offsets[f] += int64(len(b))
				}
			}
			if !supervisorAlive(p) {
				return nil
			}
		}
	}
	return nil
}
func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }
func supervisorAlive(p *state.Project) bool {
	if p == nil || !alive(p.SupervisorPID) {
		return false
	}
	b, e := exec.Command("ps", "-p", strconv.Itoa(p.SupervisorPID), "-o", "command=").Output()
	if e != nil {
		return false
	}
	cmd := string(b)
	if p.SupervisorMode == "background" {
		return strings.Contains(cmd, "_supervise "+p.Manifest)
	}
	fields := strings.Fields(cmd)
	return len(fields) > 0 && strings.Contains(filepath.Base(fields[0]), "serve-them")
}
func enable(path string, on bool, out io.Writer) error {
	mf, e := manifest.Find(path)
	if e != nil {
		return e
	}
	m, e := manifest.Load(mf)
	if e != nil {
		return e
	}
	id := state.ProjectID(mf)
	e = state.Update(func(r *state.Registry) error {
		p := r.Projects[id]
		if p == nil {
			p = &state.Project{ID: id, Name: m.Name, Manifest: mf, Status: "stopped"}
			r.Projects[id] = p
		}
		p.Enabled = on
		p.Updated = time.Now()
		return nil
	})
	if e == nil && runtime.GOOS == "darwin" {
		e = updateLaunchAgent(m, mf, id, on)
	}
	if e == nil {
		fmt.Fprintf(out, "✓ %s login startup %s\n", m.Name, map[bool]string{true: "enabled", false: "disabled"}[on])
	}
	return e
}

func updateLaunchAgent(m *manifest.Manifest, mf, id string, on bool) error {
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	file := filepath.Join(dir, "dev.serve-them.project."+id+".plist")
	label := "dev.serve-them.project." + id
	domain := "gui/" + strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+label).Run()
	if !on {
		if e := os.Remove(file); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		return nil
	}
	if e = os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	exe, e := stableExecutable()
	if e != nil {
		return e
	}
	pathValue := "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	if current := os.Getenv("PATH"); current != "" {
		pathValue = current + ":" + pathValue
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>start</string><string>%s</string></array><key>EnvironmentVariables</key><dict><key>PATH</key><string>%s</string></dict><key>RunAtLoad</key><true/><key>ProcessType</key><string>Background</string></dict></plist>`, label, xmlEscape(exe), xmlEscape(mf), xmlEscape(pathValue))
	if e = os.WriteFile(file, []byte(plist), 0644); e != nil {
		return e
	}
	return exec.Command("launchctl", "bootstrap", domain, file).Run()
}

func install(out io.Writer) error {
	return installMode(out, false)
}
func installMode(out io.Writer, repair bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("the MVP installer currently supports macOS only")
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	d, e := state.Dir()
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp("", "serve-them-edge-*.plist")
	if e != nil {
		return e
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	helper := "/Library/PrivilegedHelperTools/serve-them-edge"
	certDir := "/Library/Application Support/serve-them/certificates"
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>dev.serve-them.edge</string><key>ProgramArguments</key><array><string>%s</string><string>edge</string></array><key>EnvironmentVariables</key><dict><key>SERVE_THEM_STATE_DIR</key><string>%s</string><key>SERVE_THEM_CERT_DIR</key><string>%s</string><key>SERVE_THEM_USER_UID</key><string>%d</string></dict><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>/var/log/serve-them-edge.log</string><key>StandardErrorPath</key><string>/var/log/serve-them-edge.log</string></dict></plist>`, xmlEscape(helper), xmlEscape(d), xmlEscape(certDir), os.Getuid())
	if _, e = tmp.WriteString(plist); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if exec.Command("launchctl", "print", "system/dev.serve-them.edge").Run() == nil {
		c := exec.Command("sudo", "launchctl", "bootout", "system/dev.serve-them.edge")
		c.Stdin = os.Stdin
		c.Stdout = out
		c.Stderr = os.Stderr
		if e = c.Run(); e != nil {
			return e
		}
	}
	caFile := filepath.Join(certDir, "rootCA.pem")
	if e = removeTrustedCAs(); e != nil {
		return e
	}
	initCommand := "_init-edge"
	if repair {
		initCommand = "_repair-edge"
	}
	commands := [][]string{{"mkdir", "-p", "/Library/PrivilegedHelperTools"}, {"cp", exe, helper}, {"chown", "root:wheel", helper}, {"chmod", "755", helper}, {helper, initCommand}, {"security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", caFile}, {"cp", tmpName, "/Library/LaunchDaemons/dev.serve-them.edge.plist"}, {"chown", "root:wheel", "/Library/LaunchDaemons/dev.serve-them.edge.plist"}, {"chmod", "644", "/Library/LaunchDaemons/dev.serve-them.edge.plist"}, {"launchctl", "bootstrap", "system", "/Library/LaunchDaemons/dev.serve-them.edge.plist"}}
	for _, a := range commands {
		c := exec.Command("sudo", a...)
		c.Stdin = os.Stdin
		c.Stdout = out
		c.Stderr = os.Stderr
		if e = c.Run(); e != nil {
			return fmt.Errorf("sudo %s: %w", a[0], e)
		}
	}
	fmt.Fprintln(out, "✓ Serve Them local CA trusted\n✓ HTTPS edge service installed and started")
	return nil
}

func uninstall(args []string, out io.Writer) error {
	removeCA := false
	for _, a := range args {
		if a == "--remove-ca" {
			removeCA = true
		}
	}
	if r, e := state.Read(); e == nil {
		for _, p := range r.Projects {
			if supervisorAlive(p) {
				_ = syscall.Kill(p.SupervisorPID, syscall.SIGTERM)
			}
		}
		for i := 0; i < 50; i++ {
			running := false
			for _, p := range r.Projects {
				if supervisorAlive(p) {
					running = true
				}
			}
			if !running {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if runtime.GOOS == "darwin" {
		if exec.Command("launchctl", "print", "system/dev.serve-them.edge").Run() == nil {
			c := exec.Command("sudo", "launchctl", "bootout", "system/dev.serve-them.edge")
			c.Stdin = os.Stdin
			c.Stdout = out
			c.Stderr = os.Stderr
			if e := c.Run(); e != nil {
				return e
			}
		}
		for _, target := range []string{"/Library/LaunchDaemons/dev.serve-them.edge.plist", "/Library/PrivilegedHelperTools/serve-them-edge"} {
			c := exec.Command("sudo", "rm", "-f", target)
			c.Stdin = os.Stdin
			c.Stdout = out
			c.Stderr = os.Stderr
			if e := c.Run(); e != nil {
				return e
			}
		}
	}
	if removeCA {
		if runtime.GOOS == "darwin" {
			if e := removeTrustedCAs(); e != nil {
				return e
			}
			c := exec.Command("sudo", "rm", "-rf", "/Library/Application Support/serve-them")
			c.Stdin = os.Stdin
			c.Stdout = out
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return err
			}
		}
	}
	if home, e := os.UserHomeDir(); e == nil {
		agents, _ := filepath.Glob(filepath.Join(home, "Library", "LaunchAgents", "dev.serve-them.project.*.plist"))
		for _, f := range agents {
			label := strings.TrimSuffix(filepath.Base(f), ".plist")
			_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+label).Run()
			if e = os.Remove(f); e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
	}
	if d, e := state.Dir(); e == nil && filepath.Base(d) == "serve-them" {
		_ = os.RemoveAll(d)
	}
	fmt.Fprintln(out, "✓ edge service removed")
	if removeCA {
		fmt.Fprintln(out, "✓ local CA trust removed")
	}
	return nil
}

func removeTrustedCAs() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	for i := 0; i < 64; i++ {
		if exec.Command("security", "find-certificate", "-c", "Serve Them Local CA", "/Library/Keychains/System.keychain").Run() != nil {
			return nil
		}
		c := exec.Command("sudo", "security", "delete-certificate", "-c", "Serve Them Local CA", "/Library/Keychains/System.keychain")
		c.Stdin = os.Stdin
		if c.Run() != nil {
			return errors.New("failed to remove Serve Them certificate from the System Keychain")
		}
	}
	return errors.New("too many Serve Them certificates remain in the System Keychain")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(s)
}
func stableExecutable() (string, error) {
	if p, e := exec.LookPath("serve-them"); e == nil {
		if abs, e := filepath.Abs(p); e == nil {
			return abs, nil
		}
	}
	return os.Executable()
}
func caTrusted(file string) bool {
	b, e := os.ReadFile(file)
	if e != nil {
		return false
	}
	out, e := exec.Command("security", "find-certificate", "-a", "-c", "Serve Them Local CA", "-p", "/Library/Keychains/System.keychain").Output()
	return e == nil && bytes.Contains(out, bytes.TrimSpace(b))
}
func doctor(out io.Writer) error {
	d, e := state.Dir()
	if e != nil {
		return e
	}
	fmt.Fprintln(out, "Serve Them doctor")
	fmt.Fprintf(out, "✓ state directory: %s\n", d)
	ca, e := cert.CAFile()
	if e != nil {
		return e
	}
	if _, e = os.Stat(ca); e != nil {
		return fmt.Errorf("local CA is not installed: run serve-them install")
	}
	fmt.Fprintf(out, "✓ local CA: %s\n", ca)
	if runtime.GOOS == "darwin" {
		if !caTrusted(ca) {
			return errors.New("Serve Them local CA is not trusted in the System Keychain")
		}
		fmt.Fprintln(out, "✓ local CA trusted in macOS System Keychain")
	}
	r, e := state.Read()
	if e != nil {
		return e
	}
	fmt.Fprintf(out, "✓ registry: %d project(s)\n", len(r.Projects))
	edgeVersion, e := cert.EdgeVersion()
	if e != nil {
		return e
	}
	if edgeVersion != version {
		return fmt.Errorf("HTTPS edge version %s does not match CLI %s; run serve-them install", edgeVersion, version)
	}
	fmt.Fprintf(out, "✓ HTTPS edge version: %s\n", edgeVersion)
	return nil
}
func certificate(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "status" {
		f, e := cert.CAFile()
		if e != nil {
			return e
		}
		if _, e = os.Stat(f); e != nil {
			return errors.New("local CA is not installed")
		}
		if runtime.GOOS == "darwin" {
			if !caTrusted(f) {
				return errors.New("local CA is not trusted in the macOS System Keychain")
			}
		}
		fmt.Fprintf(out, "trusted: %s\n", f)
		return nil
	}
	if args[0] == "repair" {
		return installMode(out, true)
	}
	return errors.New("certificate command must be status or repair")
}
func usage(w io.Writer) {
	fmt.Fprintln(w, `Serve Them — local multi-application HTTPS runtime

Usage: serve-them [command] [path]

Commands: start, stop, restart, status, list, logs, open, enable, disable,
          doctor, certificate status|repair, install, uninstall, edge, version`)
}

var _ = strconv.Itoa

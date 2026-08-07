package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tankibaj/surfy/internal/cert"
	"github.com/tankibaj/surfy/internal/detect"
	"github.com/tankibaj/surfy/internal/logview"
	"github.com/tankibaj/surfy/internal/manifest"
	"github.com/tankibaj/surfy/internal/state"
)

type running struct {
	name    string
	port    int
	cmd     *exec.Cmd
	log     *rotatingLog
	done    chan error
	svc     *manifest.Service
	det     detect.Result
	started time.Time
}
type exitEvent struct {
	s   *running
	err error
}
type restartResult struct{ old, next *running }

const hookPhaseTimeout = 60 * time.Second

func Run(manifestFile string, background bool, out io.Writer) error {
	m, err := manifest.Load(manifestFile)
	if err != nil {
		return err
	}
	id := state.ProjectID(m.File)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	events := make(chan exitEvent, 2)
	var services []*running
	lifecycleStarted := false
	cleanup := func() {
		for _, s := range services {
			if s.cmd.Process != nil {
				_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for _, s := range services {
			select {
			case <-s.done:
			case <-time.After(time.Until(deadline)):
				if s.cmd.Process != nil {
					_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
				}
			}
			if s.log != nil {
				_ = s.log.Close()
			}
		}
		if lifecycleStarted && len(m.AfterStop) > 0 {
			if hookErr := runHooks(context.Background(), "after_stop", m.AfterStop, filepath.Dir(m.File), out, true); hookErr != nil {
				fmt.Fprintf(out, "! %v\n", hookErr)
			}
		}
		_ = state.Update(func(r *state.Registry) error {
			if p := r.Projects[id]; p != nil && p.SupervisorPID == os.Getpid() {
				p.Status = "stopped"
				p.SupervisorPID = 0
				if p.Web != nil {
					p.Web.Status = "stopped"
					p.Web.PID = 0
				}
				if p.API != nil {
					p.API.Status = "stopped"
					p.API.PID = 0
				}
				p.Updated = time.Now()
			}
			return nil
		})
	}
	defer cleanup()
	if err = state.Update(func(r *state.Registry) error {
		if current := r.Projects[id]; current != nil && current.SupervisorPID != 0 && current.SupervisorPID != os.Getpid() && supervisorProcess(current) {
			return fmt.Errorf("%s is already starting or running", m.Name)
		}
		for oid, p := range r.Projects {
			if oid != id && p.Name == m.Name {
				return fmt.Errorf("the hostname %s.localhost is already registered by:\n\n%s\n\nSet a unique name in surfy.yaml", m.Name, filepath.Dir(p.Manifest))
			}
		}
		enabled := false
		if old := r.Projects[id]; old != nil {
			enabled = old.Enabled
		}
		mode := "foreground"
		if background {
			mode = "background"
		}
		r.Projects[id] = &state.Project{ID: id, Name: m.Name, Manifest: m.File, Status: "starting", SupervisorPID: os.Getpid(), SupervisorMode: mode, Enabled: enabled, Updated: time.Now()}
		return nil
	}); err != nil {
		return err
	}
	lifecycleStarted = true
	if err = runHooks(ctx, "before_start", m.BeforeStart, filepath.Dir(m.File), out, false); err != nil {
		return err
	}
	if err = cert.RequestHost(m.Name); err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	for _, item := range []struct {
		name string
		svc  *manifest.Service
	}{{"web", m.Web}, {"api", m.API}} {
		if item.svc == nil {
			continue
		}
		reservation, port, e := reservePort()
		if e != nil {
			return e
		}
		det, e := detect.Service(item.svc, port)
		if e != nil {
			_ = reservation.Close()
			return fmt.Errorf("%s: %w", item.name, e)
		}
		s, e := start(item.name, item.svc, det, port, background, out, id, reservation)
		if e != nil {
			return e
		}
		services = append(services, s)
		if e = waitReady(ctx, s, 60*time.Second); e != nil {
			return e
		}
		go forwardExit(s, events)
		fmt.Fprintf(out, "✓ Detected %s %s\n✓ %s listening internally on 127.0.0.1:%d\n", det.Runtime, item.name, item.name, port)
	}
	mode := "foreground"
	if background {
		mode = "background"
	}
	p := &state.Project{ID: id, Name: m.Name, Manifest: m.File, Status: "running", SupervisorPID: os.Getpid(), SupervisorMode: mode, Updated: time.Now()}
	for _, s := range services {
		x := &state.Service{Port: s.port, PID: s.cmd.Process.Pid, Status: "running"}
		if s.log != nil {
			x.Log = s.log.Name()
		}
		if s.name == "web" {
			p.Web = x
		} else {
			p.API = x
		}
	}
	if err = state.Update(func(r *state.Registry) error {
		if old := r.Projects[id]; old != nil && old.SupervisorPID != os.Getpid() {
			return errors.New("project startup ownership changed")
		}
		if old := r.Projects[id]; old != nil {
			p.Enabled = old.Enabled
		}
		r.Projects[id] = p
		return nil
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ HTTPS certificate ready\n\nhttps://%s.localhost\n\nRoutes:\n", m.Name)
	if m.Web != nil {
		fmt.Fprintln(out, "  /       → web")
	}
	if m.API != nil {
		fmt.Fprintln(out, "  /api    → api")
	}
	if !background {
		fmt.Fprintln(out, "\nPress Ctrl+C to stop.")
	}
	retries := map[string]int{}
	restarted := make(chan restartResult)
	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-restarted:
			for i, current := range services {
				if current == result.old {
					services[i] = result.next
					_ = updateService(id, result.next.name, result.next.cmd.Process.Pid, result.next.port, "running")
					go forwardExit(result.next, events)
					break
				}
			}
		case event := <-events:
			s, exit := event.s, event.err
			i := -1
			for n, current := range services {
				if current == s {
					i = n
					break
				}
			}
			if i < 0 {
				continue
			}
			if !background {
				if exit == nil {
					return errors.New("application exited unexpectedly")
				}
				return exit
			}
			if s.log != nil {
				_ = s.log.Close()
			}
			if time.Since(s.started) > time.Minute {
				retries[s.name] = 0
			}
			retries[s.name]++
			n := retries[s.name]
			delay := restartDelay(n)
			status := "restarting"
			if n > 3 {
				status = "unhealthy"
			}
			_ = updateService(id, s.name, 0, 0, status)
			fmt.Fprintf(out, "%s exited; restarting in %s\n", s.name, delay)
			go recoverService(ctx, s, n, out, id, restarted)
		}
	}
}

func runHooks(ctx context.Context, phase string, commands []string, dir string, out io.Writer, continueOnError bool) error {
	if len(commands) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, hookPhaseTimeout)
	defer cancel()
	var failures []error
	for i, command := range commands {
		position := fmt.Sprintf("%d/%d", i+1, len(commands))
		fmt.Fprintf(out, "→ %s command %s\n", phase, position)
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
		cmd.Dir = dir
		cmd.Stdout = out
		cmd.Stderr = out
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		cmd.WaitDelay = 3 * time.Second
		if err := cmd.Run(); err != nil {
			failure := fmt.Errorf("%s command %s failed: %w", phase, position, err)
			failures = append(failures, failure)
			if !continueOnError {
				return failure
			}
			continue
		}
		fmt.Fprintf(out, "✓ %s command %s completed\n", phase, position)
	}
	return errors.Join(failures...)
}

func recoverService(ctx context.Context, s *running, attempt int, out io.Writer, id string, result chan<- restartResult) {
	for {
		delay := restartDelay(attempt)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		reservation, port, e := reservePort()
		if e != nil {
			attempt++
			continue
		}
		det, e := detect.Service(s.svc, port)
		if e != nil {
			_ = reservation.Close()
			attempt++
			continue
		}
		next, e := start(s.name, s.svc, det, port, true, out, id, reservation)
		if e != nil {
			attempt++
			_ = updateService(id, s.name, 0, 0, restartStatus(attempt))
			continue
		}
		if e = waitReady(ctx, next, 60*time.Second); e != nil {
			terminate(next)
			attempt++
			_ = updateService(id, s.name, 0, 0, restartStatus(attempt))
			continue
		}
		select {
		case <-ctx.Done():
			terminate(next)
			return
		case result <- restartResult{old: s, next: next}:
			return
		}
	}
}
func restartDelay(n int) time.Duration {
	if n <= 3 {
		return []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}[n-1]
	}
	return 30 * time.Second
}
func restartStatus(n int) string {
	if n > 3 {
		return "unhealthy"
	}
	return "restarting"
}
func terminate(s *running) {
	if s == nil {
		return
	}
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		if s.cmd.Process != nil {
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		select {
		case <-s.done:
		case <-time.After(time.Second):
		}
	}
	if s.log != nil {
		_ = s.log.Close()
	}
}

func start(name string, svc *manifest.Service, d detect.Result, port int, bg bool, out io.Writer, id string, reservation net.Listener) (*running, error) {
	if reservation != nil {
		defer reservation.Close()
	}
	cmd := exec.Command("/bin/sh", "-c", d.Command)
	cmd.Dir = svc.Path
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	env := append([]string{}, os.Environ()...)
	env = append(env, "PORT="+strconv.Itoa(port), "HOST=127.0.0.1")
	for k, v := range svc.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	var dest io.Writer = out
	var lf *rotatingLog
	if bg {
		dir, err := state.LogDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(dir, id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		f, err := newRotatingLog(filepath.Join(dir, name+".log"), 10<<20, 5)
		if err != nil {
			return nil, err
		}
		lf = f
		dest = f
	}
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if reservation != nil {
		if err := reservation.Close(); err != nil {
			return nil, err
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &running{name: name, port: port, cmd: cmd, log: lf, done: make(chan error, 1), svc: svc, det: d, started: time.Now()}
	go prefix(pr, dest, name, !bg)
	go func() {
		e := cmd.Wait()
		_ = pw.Close()
		s.done <- func() error {
			if e != nil {
				return fmt.Errorf("%s exited: %w", name, e)
			}
			return nil
		}()
	}()
	return s, nil
}

func prefix(r io.Reader, w io.Writer, name string, styled bool) {
	br := bufio.NewReaderSize(r, 64<<10)
	lineStart := true
	for {
		part, continued, e := br.ReadLine()
		if len(part) == 0 && e != nil {
			return
		}
		if len(part) > 0 {
			if lineStart {
				header := fmt.Sprintf("%s [%s] ", time.Now().Format(time.RFC3339), name)
				if styled {
					_, _ = io.WriteString(w, logview.Format(header+string(part), logview.Enabled(w)))
					lineStart = false
					if !continued {
						_, _ = io.WriteString(w, "\n")
						lineStart = true
					}
					if e != nil {
						return
					}
					continue
				}
				_, _ = io.WriteString(w, header)
			}
			_, _ = w.Write(part)
			lineStart = false
		}
		if !continued && (!lineStart || len(part) == 0) {
			_, _ = io.WriteString(w, "\n")
			lineStart = true
		}
		if e != nil {
			return
		}
	}
}
func reservePort() (net.Listener, int, error) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, 0, e
	}
	return l, l.Addr().(*net.TCPAddr).Port, nil
}
func waitReady(ctx context.Context, s *running, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-s.done:
			s.done <- e
			if e == nil {
				return fmt.Errorf("%s exited before becoming ready", s.name)
			}
			return e
		default:
		}
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.port), 150*time.Millisecond)
		if e == nil {
			c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("%s failed to become ready within %s", s.name, timeout)
}
func forwardExit(s *running, events chan<- exitEvent) {
	e := <-s.done
	s.done <- e
	events <- exitEvent{s: s, err: e}
}
func updateService(id, name string, pid, port int, status string) error {
	return state.Update(func(r *state.Registry) error {
		p := r.Projects[id]
		if p == nil {
			return nil
		}
		s := p.Web
		if name == "api" {
			s = p.API
		}
		if s != nil {
			s.PID = pid
			if port > 0 {
				s.Port = port
			}
			s.Status = status
		}
		if status == "unhealthy" {
			p.Status = "unhealthy"
		} else if status == "running" {
			p.Status = "running"
			for _, other := range []*state.Service{p.Web, p.API} {
				if other != nil && other.Status == "unhealthy" {
					p.Status = "unhealthy"
				}
			}
		}
		p.Updated = time.Now()
		return nil
	})
}
func supervisorProcess(p *state.Project) bool {
	if p == nil || p.SupervisorPID <= 0 || syscall.Kill(p.SupervisorPID, 0) != nil {
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
	f := strings.Fields(cmd)
	return len(f) > 0 && strings.Contains(filepath.Base(f[0]), "surfy")
}
func rotate(file string, max int64, keep int) {
	st, e := os.Stat(file)
	if e != nil || st.Size() < max {
		return
	}
	for i := keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", file, i), fmt.Sprintf("%s.%d", file, i+1))
	}
	_ = os.Rename(file, file+".1")
}

type rotatingLog struct {
	mu        sync.Mutex
	path      string
	file      *os.File
	size, max int64
	keep      int
}

func newRotatingLog(path string, max int64, keep int) (*rotatingLog, error) {
	rotate(path, max, keep)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	st, _ := f.Stat()
	return &rotatingLog{path: path, file: f, size: st.Size(), max: max, keep: keep}, nil
}
func (l *rotatingLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size+int64(len(p)) > l.max {
		if e := l.file.Close(); e != nil {
			return 0, e
		}
		rotate(l.path, 0, l.keep)
		f, e := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		l.file = f
		l.size = 0
	}
	n, e := l.file.Write(p)
	l.size += int64(n)
	return n, e
}
func (l *rotatingLog) Close() error { l.mu.Lock(); defer l.mu.Unlock(); return l.file.Close() }
func (l *rotatingLog) Name() string { return l.path }

package edge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tankibaj/server-them/internal/buildinfo"
	"github.com/tankibaj/server-them/internal/cert"
	"github.com/tankibaj/server-them/internal/state"
)

func Run(ctx context.Context) error {
	httpAddr := env("SERVE_THEM_HTTP_ADDR", "127.0.0.1:80")
	httpsAddr := env("SERVE_THEM_HTTPS_ADDR", "127.0.0.1:443")
	if err := startControl(ctx); err != nil {
		return err
	}
	handler := http.HandlerFunc(proxy)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		host := strings.ToLower(hello.ServerName)
		if !strings.HasSuffix(host, ".localhost") {
			return nil, fmt.Errorf("SNI hostname must end in .localhost")
		}
		name := strings.TrimSuffix(host, ".localhost")
		if !registeredName(name) {
			return nil, fmt.Errorf("no project is registered for %s", host)
		}
		cf, kf, e := cert.HostFiles(name)
		if e != nil {
			return nil, e
		}
		return loadCertificate(name, cf, kf)
	}}
	https := &http.Server{Addr: httpsAddr, Handler: handler, TLSConfig: tlsConfig, ReadHeaderTimeout: 10 * time.Second}
	httpSrv := &http.Server{Addr: httpAddr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		if !strings.HasSuffix(strings.ToLower(host), ".localhost") {
			http.Error(w, "Serve Them only accepts .localhost hosts.", http.StatusMisdirectedRequest)
			return
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}), ReadHeaderTimeout: 10 * time.Second}
	errch := make(chan error, 2)
	go func() { errch <- httpSrv.ListenAndServe() }()
	go func() {
		ln, e := net.Listen("tcp", httpsAddr)
		if e != nil {
			errch <- e
			return
		}
		errch <- https.Serve(tls.NewListener(ln, tlsConfig))
	}()
	select {
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(c)
		return https.Shutdown(c)
	case e := <-errch:
		return e
	}
}

func proxy(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	host = strings.ToLower(host)
	if !strings.HasSuffix(host, ".localhost") {
		http.Error(w, "Serve Them only accepts .localhost hosts.", http.StatusMisdirectedRequest)
		return
	}
	name := strings.TrimSuffix(host, ".localhost")
	reg, e := cachedRegistry()
	if e != nil {
		http.Error(w, "Serve Them could not read its route registry.", 500)
		return
	}
	var p *state.Project
	for _, x := range reg.Projects {
		if x.Name == name && x.Status == "running" {
			p = x
			break
		}
	}
	if p == nil {
		unavailable(w, "No running project is registered for "+host)
		return
	}
	var svc *state.Service
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		svc = p.API
	} else {
		svc = p.Web
	}
	if svc == nil || svc.Status != "running" {
		unavailable(w, "The requested service is unavailable.")
		return
	}
	rp := proxyFor(svc.Port)
	rp.ServeHTTP(w, r)
}

var proxyTransport http.RoundTripper = http.DefaultTransport
var certCache sync.Map
var proxyCache sync.Map
var routeCache struct {
	sync.Mutex
	dir      string
	checked  time.Time
	modified time.Time
	registry *state.Registry
}

func loadCertificate(name, certFile, keyFile string) (*tls.Certificate, error) {
	if v, ok := certCache.Load(name); ok {
		cached := v.(*tls.Certificate)
		leaf := cached.Leaf
		if leaf == nil && len(cached.Certificate) > 0 {
			leaf, _ = x509.ParseCertificate(cached.Certificate[0])
			cached.Leaf = leaf
		}
		if leaf != nil && time.Until(leaf.NotAfter) > 7*24*time.Hour {
			return cached, nil
		}
		if _, _, e := cert.EnsureHost(name); e != nil {
			return nil, e
		}
		certCache.Delete(name)
	}
	if _, _, e := cert.EnsureHost(name); e != nil {
		return nil, e
	}
	c, e := tls.LoadX509KeyPair(certFile, keyFile)
	if e != nil {
		return nil, e
	}
	certCache.Store(name, &c)
	return &c, nil
}
func proxyFor(port int) *httputil.ReverseProxy {
	if v, ok := proxyCache.Load(port); ok {
		return v.(*httputil.ReverseProxy)
	}
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = proxyTransport
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("proxy port %d: %v", port, e)
		unavailable(w, "The application is not accepting connections.")
	}
	actual, _ := proxyCache.LoadOrStore(port, rp)
	return actual.(*httputil.ReverseProxy)
}
func cachedRegistry() (*state.Registry, error) {
	d, e := state.Dir()
	if e != nil {
		return nil, e
	}
	routeCache.Lock()
	defer routeCache.Unlock()
	now := time.Now()
	if routeCache.registry != nil && routeCache.dir == d {
		if now.Sub(routeCache.checked) < time.Second {
			return routeCache.registry, nil
		}
		file, _ := state.RegistryFile()
		if st, statErr := os.Stat(file); statErr == nil && st.ModTime().Equal(routeCache.modified) {
			routeCache.checked = now
			return routeCache.registry, nil
		}
	}
	r, e := state.Read()
	if e != nil {
		return nil, e
	}
	file, _ := state.RegistryFile()
	if st, statErr := os.Stat(file); statErr == nil {
		routeCache.modified = st.ModTime()
	}
	routeCache.checked = now
	routeCache.dir = d
	routeCache.registry = r
	return r, nil
}

func startControl(ctx context.Context) error {
	d, e := state.Dir()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(d, 0700); e != nil {
		return e
	}
	path := filepath.Join(d, "edge.sock")
	_ = os.Remove(path)
	ln, e := net.Listen("unix", path)
	if e != nil {
		return e
	}
	if e = os.Chmod(path, 0600); e != nil {
		ln.Close()
		return e
	}
	if uid := os.Getenv("SERVE_THEM_USER_UID"); uid != "" {
		n, parseErr := strconv.Atoi(uid)
		if parseErr != nil {
			ln.Close()
			return parseErr
		}
		if e = os.Chown(path, n, -1); e != nil {
			ln.Close()
			return e
		}
	}
	go func() { <-ctx.Done(); _ = ln.Close(); _ = os.Remove(path) }()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			go handleControl(c)
		}
	}()
	return nil
}

func handleControl(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	b := make([]byte, 128)
	n, e := c.Read(b)
	if e != nil {
		return
	}
	name := strings.TrimSpace(string(b[:n]))
	if name == "VERSION" {
		_, _ = io.WriteString(c, buildinfo.Version+"\n")
		return
	}
	if name == "RELOAD" {
		routeCache.Lock()
		routeCache.registry = nil
		routeCache.Unlock()
		proxyCache.Range(func(k, v any) bool { proxyCache.Delete(k); return true })
		_, _ = io.WriteString(c, "OK\n")
		return
	}
	if strings.ContainsAny(name, "/\\\x00\r\n") {
		_, _ = io.WriteString(c, "invalid name\n")
		return
	}
	if !registeredNameFresh(name) {
		_, _ = io.WriteString(c, "project is not registered\n")
		return
	}
	if _, _, e = cert.EnsureHost(name); e != nil {
		_, _ = io.WriteString(c, e.Error()+"\n")
		return
	}
	certCache.Delete(name)
	_, _ = io.WriteString(c, "OK\n")
}
func registeredName(name string) bool {
	r, e := cachedRegistry()
	if e != nil {
		return false
	}
	for _, p := range r.Projects {
		if p.Name == name {
			return true
		}
	}
	return false
}
func registeredNameFresh(name string) bool {
	r, e := state.Read()
	if e != nil {
		return false
	}
	for _, p := range r.Projects {
		if p.Name == name {
			return true
		}
	}
	return false
}
func unavailable(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = io.WriteString(w, "<!doctype html><title>Serve Them</title><h1>Application unavailable</h1><p>"+msg+"</p>")
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

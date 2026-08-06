package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tankibaj/server-them/internal/state"
)

func EnsureCA() (certFile, keyFile string, err error) {
	d, e := directory()
	if e != nil {
		return "", "", e
	}
	if e = os.MkdirAll(d, 0700); e != nil {
		return "", "", e
	}
	if e = os.Chmod(d, 0711); e != nil {
		return "", "", e
	}
	if e = os.Chmod(filepath.Dir(d), 0711); e != nil {
		return "", "", e
	}
	certFile, keyFile = filepath.Join(d, "rootCA.pem"), filepath.Join(d, "rootCA-key.pem")
	unlock, e := lock(d)
	if e != nil {
		return "", "", e
	}
	defer unlock()
	certExists := exists(certFile)
	keyExists := exists(keyFile)
	if certExists && keyExists && validPair(certFile, keyFile, 30*24*time.Hour) {
		return certFile, keyFile, nil
	}
	if certExists || keyExists {
		return "", "", errors.New("local CA keypair is invalid; run serve-them uninstall --remove-ca, then serve-them install")
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return "", "", e
	}
	now := time.Now()
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	t := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Serve Them Local CA", Organization: []string{"Serve Them"}}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, t, t, &key.PublicKey, key)
	if e != nil {
		return "", "", e
	}
	kb, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return "", "", e
	}
	if e = writePEM(keyFile, 0600, "PRIVATE KEY", kb); e != nil {
		return "", "", e
	}
	if e = writePEM(certFile, 0644, "CERTIFICATE", der); e != nil {
		return "", "", e
	}
	return certFile, keyFile, nil
}

func EnsureHost(name string) (string, string, error) {
	if !validName.MatchString(name) {
		return "", "", fmt.Errorf("invalid certificate hostname %q", name)
	}
	caFile, caKeyFile, err := EnsureCA()
	if err != nil {
		return "", "", err
	}
	d, err := directory()
	if err != nil {
		return "", "", err
	}
	unlock, e := lock(d)
	if e != nil {
		return "", "", e
	}
	defer unlock()
	certFile, keyFile := filepath.Join(d, name+".pem"), filepath.Join(d, name+"-key.pem")
	if !exists(certFile) {
		matches, _ := filepath.Glob(filepath.Join(d, "*-key.pem"))
		if len(matches) >= 256 {
			return "", "", errors.New("local certificate limit reached")
		}
	}
	if validPair(certFile, keyFile, 7*24*time.Hour) {
		return certFile, keyFile, nil
	}
	ca, err := readCert(caFile)
	if err != nil {
		return "", "", err
	}
	caKey, err := readKey(caKeyFile)
	if err != nil {
		return "", "", err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	host := name + ".localhost"
	t := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(0, 3, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, t, ca, &key.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	if err = writePEM(keyFile, 0600, "PRIVATE KEY", kb); err != nil {
		return "", "", err
	}
	if err = writePEM(certFile, 0600, "CERTIFICATE", der); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

func RequestHost(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid certificate hostname %q", name)
	}
	if os.Getenv("SERVE_THEM_UNPRIVILEGED_CERTS") == "1" {
		_, _, e := EnsureHost(name)
		return e
	}
	reply, e := control(name)
	if e != nil {
		return e
	}
	if reply != "OK" {
		return fmt.Errorf("certificate request failed: %s", reply)
	}
	return nil
}

func EdgeVersion() (string, error) { return control("VERSION") }

func control(command string) (string, error) {
	d, e := state.Dir()
	if e != nil {
		return "", e
	}
	c, e := net.DialTimeout("unix", filepath.Join(d, "edge.sock"), 2*time.Second)
	if e != nil {
		return "", fmt.Errorf("local HTTPS service is not running: %w", e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, e = fmt.Fprintln(c, command); e != nil {
		return "", e
	}
	b := make([]byte, 256)
	n, e := c.Read(b)
	if e != nil {
		return "", e
	}
	reply := strings.TrimSpace(string(b[:n]))
	return reply, nil
}

func HostFiles(name string) (string, string, error) {
	if !validName.MatchString(name) {
		return "", "", fmt.Errorf("invalid certificate hostname %q", name)
	}
	d, e := directory()
	if e != nil {
		return "", "", e
	}
	return filepath.Join(d, name+".pem"), filepath.Join(d, name+"-key.pem"), nil
}

func CAFile() (string, error) {
	d, e := directory()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "rootCA.pem"), nil
}

func valid(file string, min time.Duration) bool {
	c, e := readCert(file)
	return e == nil && time.Until(c.NotAfter) > min && time.Now().After(c.NotBefore)
}
func validPair(certFile, keyFile string, min time.Duration) bool {
	if !valid(certFile, min) {
		return false
	}
	c, e := readCert(certFile)
	if e != nil {
		return false
	}
	k, e := readKey(keyFile)
	if e != nil {
		return false
	}
	pub, ok := c.PublicKey.(*ecdsa.PublicKey)
	return ok && pub.Equal(&k.PublicKey)
}
func exists(path string) bool { _, e := os.Stat(path); return e == nil }
func readCert(f string) (*x509.Certificate, error) {
	b, e := os.ReadFile(f)
	if e != nil {
		return nil, e
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return nil, errors.New("invalid certificate PEM")
	}
	return x509.ParseCertificate(p.Bytes)
}
func readKey(f string) (*ecdsa.PrivateKey, error) {
	b, e := os.ReadFile(f)
	if e != nil {
		return nil, e
	}
	p, _ := pem.Decode(b)
	if p == nil {
		return nil, errors.New("invalid key PEM")
	}
	k, e := x509.ParsePKCS8PrivateKey(p.Bytes)
	if e != nil {
		return nil, e
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected CA key type")
	}
	return ec, nil
}
func writePEM(f string, mode os.FileMode, typ string, b []byte) error {
	tmp, e := os.CreateTemp(filepath.Dir(f), ".pem-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(mode); e == nil {
		_, e = tmp.Write(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}))
	}
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(name, f)
}

var validName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func directory() (string, error) {
	if d := os.Getenv("SERVE_THEM_CERT_DIR"); d != "" {
		return filepath.Abs(d)
	}
	if runtime.GOOS == "darwin" {
		return "/Library/Application Support/serve-them/certificates", nil
	}
	d, e := state.Dir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "certificates"), nil
}
func lock(d string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(d, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

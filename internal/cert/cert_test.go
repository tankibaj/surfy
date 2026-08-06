package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHostCertificateIsSignedByCA(t *testing.T) {
	d := t.TempDir()
	t.Setenv("SERVE_THEM_STATE_DIR", d)
	t.Setenv("SERVE_THEM_CERT_DIR", d+"/certificates")
	caFile, _, e := EnsureCA()
	if e != nil {
		t.Fatal(e)
	}
	hostFile, _, e := EnsureHost("shop")
	if e != nil {
		t.Fatal(e)
	}
	read := func(f string) *x509.Certificate {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		p, _ := pem.Decode(b)
		c, e := x509.ParseCertificate(p.Bytes)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	ca, host := read(caFile), read(hostFile)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, e = host.Verify(x509.VerifyOptions{Roots: pool, DNSName: "shop.localhost"}); e != nil {
		t.Fatal(e)
	}
}

func TestMismatchedKeyPairIsRepaired(t *testing.T) {
	d := t.TempDir()
	t.Setenv("SERVE_THEM_CERT_DIR", d)
	certFile, keyFile, e := EnsureHost("demo")
	if e != nil {
		t.Fatal(e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	kb, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	if e = writePEM(keyFile, 0600, "PRIVATE KEY", kb); e != nil {
		t.Fatal(e)
	}
	if validPair(certFile, keyFile, time.Hour) {
		t.Fatal("mismatched pair considered valid")
	}
	if _, _, e = EnsureHost("demo"); e != nil {
		t.Fatal(e)
	}
	if !validPair(certFile, keyFile, time.Hour) {
		t.Fatal("pair was not repaired")
	}
}

func TestRejectsUnsafeHostname(t *testing.T) {
	t.Setenv("SERVE_THEM_CERT_DIR", t.TempDir())
	if _, _, e := EnsureHost("../../tmp/owned"); e == nil || !strings.Contains(e.Error(), "invalid") {
		t.Fatalf("expected validation error, got %v", e)
	}
}

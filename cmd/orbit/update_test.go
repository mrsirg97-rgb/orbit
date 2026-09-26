package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
)

type minisignTestKey struct {
	pubText string
	sign    func([]byte) []byte
}

// newMinisignKey builds the wire format minisign 0.11 actually produces:
// the public key is 2-byte "Ed" + 8-byte key id + 32-byte ed25519 key, and
// the signature is 2-byte "ED" + 8-byte key id + 64-byte ed25519 signature
// over the BLAKE2b-512 digest of the file (minisign's default hashed mode).
func newMinisignKey(t *testing.T) minisignTestKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyID := pub[:8]
	blob := append([]byte("Ed"), keyID...)
	blob = append(blob, pub...)
	pubText := "untrusted comment: minisign public key\n" +
		base64.StdEncoding.EncodeToString(blob)
	return minisignTestKey{pubText: pubText, sign: func(data []byte) []byte {
		h := blake2b.Sum512(data)
		sig := ed25519.Sign(priv, h[:])
		sigBlob := append([]byte("ED"), keyID...)
		sigBlob = append(sigBlob, sig...)
		return []byte("untrusted comment: signature from minisign secret key\n" +
			base64.StdEncoding.EncodeToString(sigBlob))
	}}
}

func newUpdateSrv(t *testing.T, latest string, asset []byte, checksums string, sig []byte) *httptest.Server {
	t.Helper()
	assetPath := "/" + updateRepo + "/releases/download/v" + latest + "/orbit_linux_amd64"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+updateRepo+"/releases/latest":
			http.Redirect(w, r, "/"+updateRepo+"/releases/tag/v"+latest, http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/"+updateRepo+"/releases/tag/"):
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/"+updateRepo+"/releases/download/v"+latest+"/checksums.txt":
			fmt.Fprint(w, checksums)
		case r.URL.Path == assetPath:
			_, _ = w.Write(asset)
		case r.URL.Path == assetPath+".minisig":
			if sig == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(sig)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func updateCfgFor(t *testing.T, srv *httptest.Server, version, bin string) updateCfg {
	t.Helper()
	return updateCfg{
		base:    srv.URL,
		repo:    updateRepo,
		version: version,
		bin:     bin,
		goos:    "linux",
		arch:    "amd64",
		out:     &bytes.Buffer{},
	}
}

func writeOld(t *testing.T, bin string) {
	t.Helper()
	if err := os.WriteFile(bin, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func checksumsFor(sum string) string {
	return fmt.Sprintf("%s  orbit_linux_amd64\n%s  orbit_darwin_amd64\n", sum, strings.Repeat("0", 64))
}

func TestUpdateReplacesInPlace(t *testing.T) {
	key := newMinisignKey(t)
	asset := []byte("new binary bytes")
	srv := newUpdateSrv(t, "0.2.1", asset, checksumsFor(sha256Hex(asset)), key.sign(asset))
	dir := t.TempDir()
	bin := filepath.Join(dir, "orbit")
	writeOld(t, bin)
	cfg := updateCfgFor(t, srv, "0.2.0", bin)
	cfg.key = key.pubText
	if err := update(context.Background(), cfg); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("read new binary: %v", err)
	}
	if string(got) != string(asset) {
		t.Fatalf("binary = %q, want %q", got, asset)
	}
	fi, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat new binary: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", fi.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "orbit" {
		t.Fatalf("dir has %d entries (want only orbit), old file gone", len(entries))
	}
	out := cfg.out.(*bytes.Buffer).String()
	if !strings.Contains(out, "orbit: 0.2.0 -> 0.2.1 ("+bin+")") {
		t.Fatalf("output %q missing the version->version line", out)
	}
	if !strings.Contains(out, "running sessions keep the old binary") {
		t.Fatalf("output %q missing the running-sessions note", out)
	}
}

func TestUpdateBadChecksumRefuses(t *testing.T) {
	key := newMinisignKey(t)
	asset := []byte("new binary bytes")
	srv := newUpdateSrv(t, "0.2.1", asset, checksumsFor(strings.Repeat("f", 64)), key.sign(asset))
	dir := t.TempDir()
	bin := filepath.Join(dir, "orbit")
	writeOld(t, bin)
	cfg := updateCfgFor(t, srv, "0.2.0", bin)
	cfg.key = key.pubText
	err := update(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("update err = %v, want a checksum mismatch", err)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(got) != "old binary" {
		t.Fatalf("binary = %q, want the old bytes (nothing written)", got)
	}
}

func TestUpdateAlreadyLatestIsNoOp(t *testing.T) {
	asset := []byte("new binary bytes")
	srv := newUpdateSrv(t, "0.2.0", asset, checksumsFor(sha256Hex(asset)), nil)
	dir := t.TempDir()
	bin := filepath.Join(dir, "orbit")
	writeOld(t, bin)
	cfg := updateCfgFor(t, srv, "0.2.0", bin)
	if err := update(context.Background(), cfg); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(got) != "old binary" {
		t.Fatalf("binary = %q, want unchanged", got)
	}
	out := cfg.out.(*bytes.Buffer).String()
	if !strings.Contains(out, "orbit: already at 0.2.0 (latest)") {
		t.Fatalf("output %q missing the already-latest line", out)
	}
}

func TestUpdateRefusesWithoutAVerificationKey(t *testing.T) {
	asset := []byte("new binary bytes")
	srv := newUpdateSrv(t, "0.2.1", asset, checksumsFor(sha256Hex(asset)), nil)
	dir := t.TempDir()
	bin := filepath.Join(dir, "orbit")
	writeOld(t, bin)
	cfg := updateCfgFor(t, srv, "0.2.0", bin)
	err := update(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "no verification key") {
		t.Fatalf("update err = %v, want the unpinned refusal", err)
	}
}

func TestUpdateKeyResolvesOperatorSettings(t *testing.T) {
	home := t.TempDir()
	key := newMinisignKey(t)
	// The operator's settings.json pins orbit's own key; the rig embedded
	// updateKey is not orbit's and must never be used.
	b, err := json.Marshal(map[string]string{"updateKey": key.pubText})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "settings.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(k string) string {
		if k == "RIG_HOME" {
			return home
		}
		return ""
	}
	got, err := updateKey(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if got != key.pubText {
		t.Errorf("update key %q, want the settings.json pin", got)
	}
	env := func(k string) string {
		if k == "RIG_HOME" {
			return home
		}
		if k == "ORBIT_UPDATE_KEY" {
			return "env-key"
		}
		return ""
	}
	if got, _ := updateKey(env); got != "env-key" {
		t.Errorf("update key %q, want ORBIT_UPDATE_KEY to override settings", got)
	}
	// A home without updateKey (the rig embedded key must not count) is unpinned.
	empty := t.TempDir()
	got, err = updateKey(func(k string) string {
		if k == "RIG_HOME" {
			return empty
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("update key %q, want none (the embedded rig key is not orbit's)", got)
	}
}

func TestVerifyMinisignRefusesTamperAndShortFormat(t *testing.T) {
	key := newMinisignKey(t)
	asset := []byte("new binary bytes")
	if err := verifyMinisign(key.pubText, asset, key.sign(asset)); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := verifyMinisign(key.pubText, []byte("tampered"), key.sign(asset)); err == nil {
		t.Fatal("a tampered asset must refuse")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := pub[:8]
	pubText := "untrusted comment: minisign public key\n" +
		base64.StdEncoding.EncodeToString(append(append([]byte{}, keyID...), pub...))
	sig := ed25519.Sign(priv, asset)
	sigText := "untrusted comment: signature from minisign secret key\n" +
		base64.StdEncoding.EncodeToString(append(append([]byte{}, sig...), keyID...))
	err = verifyMinisign(pubText, asset, []byte(sigText))
	if err == nil || !strings.Contains(err.Error(), "want 42") {
		t.Fatalf("the invented short key format must refuse by name, got %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.2.1", "0.2.0", 1},
		{"0.2.0", "0.2.1", -1},
		{"0.2.0", "0.2.0", 0},
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1},
	} {
		got, err := compareVersions(c.a, c.b)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", c.a, c.b, err)
		}
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

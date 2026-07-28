package audit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSignedArchiveVerifiesWithTrustedPublicKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	records := []Entry{{ID: 4, Actor: Actor{Name: "Admin"}, Action: "cluster.probe", Result: "success", RequestID: "request-4", Details: map[string]any{"route": "/api/v1/clusters/4/probe"}, CreatedAt: time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)}}
	payload, manifest, _, err := EncodeSignedArchive(records, privateKey, time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC), "test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := VerifySignedArchive(payload, manifest, publicKey)
	if err != nil || result.RecordCount != 1 || result.FirstAuditID != 4 || result.LastAuditID != 4 {
		t.Fatalf("VerifySignedArchive() result=%#v err=%v", result, err)
	}
}

func TestSignedArchiveRejectsPayloadTamperingAndUntrustedSigner(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, manifest, _, err := EncodeSignedArchive([]Entry{{ID: 1, Details: map[string]any{}, CreatedAt: time.Now().UTC()}}, privateKey, time.Now().UTC(), "test")
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-2] ^= 1
	if _, err := VerifySignedArchive(payload, manifest, publicKey); err == nil {
		t.Fatal("VerifySignedArchive() accepted modified payload")
	}
	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, manifest, _, err = EncodeSignedArchive([]Entry{{ID: 1, Details: map[string]any{}, CreatedAt: time.Now().UTC()}}, privateKey, time.Now().UTC(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySignedArchive(payload, manifest, otherPublicKey); !errors.Is(err, ErrUntrustedSigner) {
		t.Fatalf("VerifySignedArchive() error=%v, want ErrUntrustedSigner", err)
	}
}

func TestSignedArchiveRejectsManifestTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, manifest, _, err := EncodeSignedArchive([]Entry{{ID: 1, Details: map[string]any{}, CreatedAt: time.Now().UTC()}}, privateKey, time.Now().UTC(), "test")
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte(`"record_count":1`), []byte(`"record_count":2`), 1)
	if _, err := VerifySignedArchive(payload, manifest, publicKey); err == nil {
		t.Fatal("VerifySignedArchive() accepted modified manifest")
	}
}

func TestWriteSignedArchiveRefusesOverwrite(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "audit.json")
	records := []Entry{{ID: 1, Details: map[string]any{}, CreatedAt: time.Now().UTC()}}
	if _, err := WriteSignedArchive(path, records, privateKey, time.Now().UTC(), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSignedArchive(path, records, privateKey, time.Now().UTC(), "test"); err == nil {
		t.Fatal("WriteSignedArchive() accepted existing output")
	}
	if _, err := os.Stat(ManifestPath(path)); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

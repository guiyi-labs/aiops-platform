package audit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ArchiveFormat           = "aiops.audit.archive.v1"
	ArchiveSignature        = "Ed25519"
	archiveManifestFileTail = ".manifest.json"
)

var (
	ErrNoArchiveRecords = errors.New("audit archive contains no records")
	ErrUntrustedSigner  = errors.New("audit archive signer does not match trusted public key")
)

type ArchivePayload struct {
	Format  string  `json:"format"`
	Records []Entry `json:"records"`
}

type ArchiveManifest struct {
	Format          string    `json:"format"`
	PayloadSHA256   string    `json:"payload_sha256"`
	RecordCount     int       `json:"record_count"`
	FirstAuditID    int64     `json:"first_audit_id"`
	LastAuditID     int64     `json:"last_audit_id"`
	CreatedAt       time.Time `json:"created_at"`
	ToolVersion     string    `json:"tool_version"`
	SignatureFormat string    `json:"signature_format"`
	SignerPublicKey string    `json:"signer_public_key"`
	Signature       string    `json:"signature"`
}

type unsignedArchiveManifest struct {
	Format          string    `json:"format"`
	PayloadSHA256   string    `json:"payload_sha256"`
	RecordCount     int       `json:"record_count"`
	FirstAuditID    int64     `json:"first_audit_id"`
	LastAuditID     int64     `json:"last_audit_id"`
	CreatedAt       time.Time `json:"created_at"`
	ToolVersion     string    `json:"tool_version"`
	SignatureFormat string    `json:"signature_format"`
	SignerPublicKey string    `json:"signer_public_key"`
}

type ArchiveResult struct {
	ArchivePath  string `json:"archive_path"`
	ManifestPath string `json:"manifest_path"`
	PayloadSHA   string `json:"payload_sha256"`
	RecordCount  int    `json:"record_count"`
	FirstAuditID int64  `json:"first_audit_id"`
	LastAuditID  int64  `json:"last_audit_id"`
}

type VerificationResult struct {
	PayloadSHA   string `json:"payload_sha256"`
	RecordCount  int    `json:"record_count"`
	FirstAuditID int64  `json:"first_audit_id"`
	LastAuditID  int64  `json:"last_audit_id"`
}

func ManifestPath(archivePath string) string { return archivePath + archiveManifestFileTail }

func EncodeSignedArchive(records []Entry, privateKey ed25519.PrivateKey, createdAt time.Time, toolVersion string) ([]byte, []byte, ArchiveManifest, error) {
	if len(records) == 0 {
		return nil, nil, ArchiveManifest{}, ErrNoArchiveRecords
	}
	if len(records) > MaxArchiveRecords || len(privateKey) != ed25519.PrivateKeySize {
		return nil, nil, ArchiveManifest{}, fmt.Errorf("invalid audit archive signing input")
	}
	for index, record := range records {
		if record.ID < 1 || (index > 0 && record.ID <= records[index-1].ID) {
			return nil, nil, ArchiveManifest{}, fmt.Errorf("audit archive records must have strictly ascending positive IDs")
		}
	}
	payload, err := json.Marshal(ArchivePayload{Format: ArchiveFormat, Records: records})
	if err != nil {
		return nil, nil, ArchiveManifest{}, fmt.Errorf("encode audit archive payload: %w", err)
	}
	digest := sha256.Sum256(payload)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	unsigned := unsignedArchiveManifest{
		Format: ArchiveFormat, PayloadSHA256: hex.EncodeToString(digest[:]), RecordCount: len(records),
		FirstAuditID: records[0].ID, LastAuditID: records[len(records)-1].ID, CreatedAt: createdAt.UTC(),
		ToolVersion: toolVersion, SignatureFormat: ArchiveSignature, SignerPublicKey: base64.StdEncoding.EncodeToString(publicKey),
	}
	message, err := json.Marshal(unsigned)
	if err != nil {
		return nil, nil, ArchiveManifest{}, fmt.Errorf("encode audit archive manifest: %w", err)
	}
	manifest := ArchiveManifest{
		Format: unsigned.Format, PayloadSHA256: unsigned.PayloadSHA256, RecordCount: unsigned.RecordCount,
		FirstAuditID: unsigned.FirstAuditID, LastAuditID: unsigned.LastAuditID, CreatedAt: unsigned.CreatedAt,
		ToolVersion: unsigned.ToolVersion, SignatureFormat: unsigned.SignatureFormat, SignerPublicKey: unsigned.SignerPublicKey,
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, message)),
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, nil, ArchiveManifest{}, fmt.Errorf("encode signed audit manifest: %w", err)
	}
	return payload, manifestBytes, manifest, nil
}

func VerifySignedArchive(payload, manifestBytes []byte, trustedPublicKey ed25519.PublicKey) (VerificationResult, error) {
	if len(trustedPublicKey) != ed25519.PublicKeySize {
		return VerificationResult{}, fmt.Errorf("invalid trusted Ed25519 public key")
	}
	var manifest ArchiveManifest
	if err := decodeJSON(manifestBytes, &manifest); err != nil {
		return VerificationResult{}, fmt.Errorf("decode audit archive manifest: %w", err)
	}
	if manifest.Format != ArchiveFormat || manifest.SignatureFormat != ArchiveSignature || manifest.RecordCount < 1 || manifest.RecordCount > MaxArchiveRecords || manifest.FirstAuditID < 1 || manifest.LastAuditID < manifest.FirstAuditID || manifest.CreatedAt.IsZero() || strings.TrimSpace(manifest.ToolVersion) == "" {
		return VerificationResult{}, fmt.Errorf("invalid audit archive manifest")
	}
	embeddedPublicKey, err := base64.StdEncoding.DecodeString(manifest.SignerPublicKey)
	if err != nil || len(embeddedPublicKey) != ed25519.PublicKeySize {
		return VerificationResult{}, fmt.Errorf("invalid audit archive signer public key")
	}
	if !bytes.Equal(embeddedPublicKey, trustedPublicKey) {
		return VerificationResult{}, ErrUntrustedSigner
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return VerificationResult{}, fmt.Errorf("invalid audit archive signature")
	}
	unsigned := unsignedArchiveManifest{
		Format: manifest.Format, PayloadSHA256: manifest.PayloadSHA256, RecordCount: manifest.RecordCount,
		FirstAuditID: manifest.FirstAuditID, LastAuditID: manifest.LastAuditID, CreatedAt: manifest.CreatedAt,
		ToolVersion: manifest.ToolVersion, SignatureFormat: manifest.SignatureFormat, SignerPublicKey: manifest.SignerPublicKey,
	}
	message, err := json.Marshal(unsigned)
	if err != nil || !ed25519.Verify(trustedPublicKey, message, signature) {
		return VerificationResult{}, fmt.Errorf("audit archive signature verification failed")
	}
	digest := sha256.Sum256(payload)
	if !strings.EqualFold(manifest.PayloadSHA256, hex.EncodeToString(digest[:])) {
		return VerificationResult{}, fmt.Errorf("audit archive payload digest mismatch")
	}
	var archive ArchivePayload
	if err := decodeJSON(payload, &archive); err != nil {
		return VerificationResult{}, fmt.Errorf("decode audit archive payload: %w", err)
	}
	if archive.Format != ArchiveFormat || len(archive.Records) != manifest.RecordCount {
		return VerificationResult{}, fmt.Errorf("audit archive payload metadata mismatch")
	}
	for index, record := range archive.Records {
		if record.ID < 1 || (index > 0 && record.ID <= archive.Records[index-1].ID) {
			return VerificationResult{}, fmt.Errorf("invalid audit archive record ordering")
		}
	}
	if archive.Records[0].ID != manifest.FirstAuditID || archive.Records[len(archive.Records)-1].ID != manifest.LastAuditID {
		return VerificationResult{}, fmt.Errorf("audit archive record range mismatch")
	}
	return VerificationResult{PayloadSHA: manifest.PayloadSHA256, RecordCount: manifest.RecordCount, FirstAuditID: manifest.FirstAuditID, LastAuditID: manifest.LastAuditID}, nil
}

func WriteSignedArchive(archivePath string, records []Entry, privateKey ed25519.PrivateKey, createdAt time.Time, toolVersion string) (ArchiveResult, error) {
	if strings.TrimSpace(archivePath) == "" {
		return ArchiveResult{}, fmt.Errorf("audit archive output path is required")
	}
	manifestPath := ManifestPath(archivePath)
	if archivePath == manifestPath {
		return ArchiveResult{}, fmt.Errorf("audit archive and manifest paths must differ")
	}
	if err := ensureNewPath(archivePath); err != nil {
		return ArchiveResult{}, err
	}
	if err := ensureNewPath(manifestPath); err != nil {
		return ArchiveResult{}, err
	}
	payload, manifestBytes, manifest, err := EncodeSignedArchive(records, privateKey, createdAt, toolVersion)
	if err != nil {
		return ArchiveResult{}, err
	}
	if err := writeNewFile(archivePath, payload); err != nil {
		return ArchiveResult{}, err
	}
	if err := writeNewFile(manifestPath, manifestBytes); err != nil {
		_ = os.Remove(archivePath)
		return ArchiveResult{}, err
	}
	return ArchiveResult{ArchivePath: archivePath, ManifestPath: manifestPath, PayloadSHA: manifest.PayloadSHA256, RecordCount: manifest.RecordCount, FirstAuditID: manifest.FirstAuditID, LastAuditID: manifest.LastAuditID}, nil
}

func VerifyArchiveFiles(archivePath, manifestPath string, trustedPublicKey ed25519.PublicKey) (VerificationResult, error) {
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		return VerificationResult{}, fmt.Errorf("read audit archive: %w", err)
	}
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerificationResult{}, fmt.Errorf("read audit archive manifest: %w", err)
	}
	return VerifySignedArchive(payload, manifest, trustedPublicKey)
}

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	encoded, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(encoded) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(encoded), nil
	}
	if len(encoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("Ed25519 private key must decode to %d-byte seed or %d-byte private key", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(encoded), nil
}

func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	encoded, err := readKeyFile(path)
	if err != nil {
		return nil, err
	}
	if len(encoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("Ed25519 public key must decode to %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(encoded), nil
}

func readKeyFile(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("key file path is required")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(contents)))
	if err != nil {
		return nil, fmt.Errorf("decode base64 key file: %w", err)
	}
	return decoded, nil
}

func ensureNewPath(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing audit archive path %q", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect audit archive path: %w", err)
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("inspect audit archive output directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("audit archive output parent is not a directory")
	}
	return nil
}

func writeNewFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create audit archive file: %w", err)
	}
	succeeded := false
	defer func() {
		_ = file.Close()
		if !succeeded {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write audit archive file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync audit archive file: %w", err)
	}
	succeeded = true
	return nil
}

func decodeJSON(source []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected one JSON document")
	}
	return nil
}

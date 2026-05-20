// Package pack produces a flat single-file JSON audit pack (audit.pack.json)
// that contains the same content as the multi-file ZIP artifact but as one
// schema-versioned JSON document. The document is JCS-canonicalized, so it is
// byte-deterministic for a given bundle + sidecar combination.
//
// Callers that prefer streaming a single artifact (e.g. CI pipelines, HTTP
// uploads) can use this package instead of the ZIP. The ZIP path remains
// unchanged and is still the default for the reconcile command.
package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
)

const (
	// SchemaVersion is the schema version string embedded in every pack document.
	SchemaVersion = "provenant.audit.pack/v1"

	// manifestSchemaVersion is the schema version of the embedded manifest object.
	manifestSchemaVersion = "provenant.audit.pack.manifest/v1"
)

// ErrUnknownSchema is returned by Decode when the schema_version field is not
// a recognised value. Decoders must reject unknown schema versions.
var ErrUnknownSchema = errors.New("pack: unknown schema_version")

// Options controls which optional sidecar artifacts are embedded in the pack.
// All sidecar fields are optional; absent sidecars are omitted from the pack.
type Options struct {
	// BundleDir is the directory to search for sidecar files when the caller
	// does not supply them explicitly. If empty, the caller's working directory
	// is used. Build looks for report.md, report.json, and mismatches.csv in
	// this directory.
	BundleDir string

	// ReportMarkdown overrides the auto-discovered report.md content.
	ReportMarkdown []byte
	// ReportJSON overrides the auto-discovered report.json content.
	ReportJSON json.RawMessage
	// MismatchesCSV overrides the auto-discovered mismatches.csv content.
	MismatchesCSV []byte

	// SkipSidecarDiscovery disables file-system sidecar discovery entirely.
	// Only explicitly-supplied sidecar fields are used.
	SkipSidecarDiscovery bool
}

// Document is the on-disk JSON shape of an audit pack. It is exported so that
// callers of Decode can introspect the decoded pack contents.
type Document struct {
	SchemaVersion string                `json:"schema_version"`
	BundleSHA256  string                `json:"bundle_sha256"`
	Bundle        json.RawMessage       `json:"bundle"`
	CapturedReads []kernel.CapturedRead `json:"captured_reads"`
	Tokens        []kernel.Token        `json:"tokens"`
	AuditJSONL    string                `json:"audit_jsonl"`
	Report        *ReportSection        `json:"report,omitempty"`
	Manifest      ManifestSection       `json:"manifest"`
}

// ReportSection is the optional reconciliation report embedded in the pack.
// Present only when the run came from a reconcile flow; absent otherwise.
type ReportSection struct {
	Markdown      string          `json:"markdown"`
	JSON          json.RawMessage `json:"json,omitempty"`
	MismatchesCSV string          `json:"mismatches_csv,omitempty"`
}

// ManifestSection is a lightweight integrity manifest over the key artifacts.
type ManifestSection struct {
	SchemaVersion string           `json:"schema_version"`
	BundleHash    string           `json:"bundle_hash"`
	Artifacts     []ArtifactRecord `json:"artifacts"`
}

// ArtifactRecord is one entry in the manifest's artifact list.
type ArtifactRecord struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Build produces a JCS-canonical audit pack from finalized bundle bytes.
//
// The bundle is verified before the pack is assembled. If verification fails,
// Build returns an error wrapping bundle.ErrTamperDetected.
//
// Sidecar artifacts (report.md, report.json, mismatches.csv) are discovered
// from opts.BundleDir unless opts.SkipSidecarDiscovery is true.
func Build(bundleBytes []byte, opts Options) ([]byte, error) {
	// Verify the bundle before embedding it.
	state, err := bundle.Replay(bundleBytes)
	if err != nil {
		return nil, fmt.Errorf("pack: bundle verification failed: %w", err)
	}

	bundleSHA256 := sha256Hex(bundleBytes)

	// Discover sidecars unless the caller opted out.
	if !opts.SkipSidecarDiscovery {
		opts = discoverSidecars(opts)
	}

	// Build the audit_jsonl string from the replayed state entries. We re-use
	// the bundle's embedded audit_jsonl by extracting it from the raw bundle
	// document rather than re-serialising the entries.
	auditJSONL, err := extractAuditJSONL(bundleBytes)
	if err != nil {
		return nil, fmt.Errorf("pack: extract audit_jsonl: %w", err)
	}

	// Assemble the report section (omitted when no sidecars are present).
	var reportSec *ReportSection
	if len(opts.ReportMarkdown) > 0 || len(opts.ReportJSON) > 0 || len(opts.MismatchesCSV) > 0 {
		reportSec = &ReportSection{
			Markdown:      string(opts.ReportMarkdown),
			JSON:          opts.ReportJSON,
			MismatchesCSV: string(opts.MismatchesCSV),
		}
	}

	// Build the manifest over bundle + audit_jsonl.
	artifacts := []ArtifactRecord{
		{Name: "bundle", SHA256: bundleSHA256},
		{Name: "audit.jsonl", SHA256: sha256Hex([]byte(auditJSONL))},
	}

	doc := Document{
		SchemaVersion: SchemaVersion,
		BundleSHA256:  bundleSHA256,
		Bundle:        json.RawMessage(bundleBytes),
		CapturedReads: state.CapturedReads,
		Tokens:        state.Tokens,
		AuditJSONL:    auditJSONL,
		Report:        reportSec,
		Manifest: ManifestSection{
			SchemaVersion: manifestSchemaVersion,
			BundleHash:    bundleSHA256,
			Artifacts:     artifacts,
		},
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("pack: marshal document: %w", err)
	}

	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("pack: JCS canonicalize: %w", err)
	}

	return canonical, nil
}

// extractBundleDoc is the minimal shape needed to pull audit_jsonl from a
// bundle document without re-importing bundle internals.
type extractBundleDoc struct {
	AuditJSONL string `json:"audit_jsonl"`
}

// extractAuditJSONL pulls the audit_jsonl string out of a raw bundle document
// without re-verifying it (verification already happened in Build).
func extractAuditJSONL(bundleBytes []byte) (string, error) {
	var doc extractBundleDoc
	if err := json.Unmarshal(bundleBytes, &doc); err != nil {
		return "", err
	}
	return doc.AuditJSONL, nil
}

// discoverSidecars looks for well-known sidecar files in opts.BundleDir and
// fills any missing sidecar fields on opts.
func discoverSidecars(opts Options) Options {
	dir := opts.BundleDir
	if dir == "" {
		dir = "."
	}

	if opts.ReportMarkdown == nil {
		if data, err := os.ReadFile(filepath.Join(dir, "report.md")); err == nil {
			opts.ReportMarkdown = data
		}
	}
	if opts.ReportJSON == nil {
		if data, err := os.ReadFile(filepath.Join(dir, "report.json")); err == nil {
			opts.ReportJSON = json.RawMessage(data)
		}
	}
	if opts.MismatchesCSV == nil {
		if data, err := os.ReadFile(filepath.Join(dir, "mismatches.csv")); err == nil {
			opts.MismatchesCSV = data
		}
	}
	return opts
}

// WriteFile atomically writes the pack bytes to path.
func WriteFile(path string, packBytes []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("pack: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(packBytes); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("pack: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("pack: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("pack: rename temp file: %w", err)
	}
	ok = true
	return nil
}

// Decode parses and validates a pack document. It returns ErrUnknownSchema if
// the schema_version is not recognised.
func Decode(r io.Reader) (Document, error) {
	limited := &io.LimitedReader{R: r, N: maxPackBytes + 1}
	dec := json.NewDecoder(limited)

	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("pack: decode: %w", err)
	}
	if limited.N == 0 {
		return Document{}, fmt.Errorf("pack: document exceeds size limit")
	}
	if doc.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf("%w: %q", ErrUnknownSchema, doc.SchemaVersion)
	}
	return doc, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// maxPackBytes is the upper bound for Decode. Build can produce larger packs
// for runs with many captured reads; callers that need to decode large packs
// should stream the bundle field separately.
const maxPackBytes = 32 << 20 // 32 MiB

package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidSourceSelector = errors.New("kernel: invalid source selector")

// RunStatus is the lifecycle state of an in-process run.
type RunStatus string

const (
	// RunPending means the run has been declared but not started.
	RunPending RunStatus = "pending"
	// RunRunning means the run is actively executing.
	RunRunning RunStatus = "running"
	// RunCompleted means the run reached a terminal success state.
	RunCompleted RunStatus = "completed"
	// RunFailed means the run reached a terminal failure state.
	RunFailed RunStatus = "failed"
)

// Run identifies the tenant-scoped execution that owns tokens and audit entries.
type Run struct {
	ID       string    `json:"id"`
	TenantID string    `json:"tenant_id"`
	Status   RunStatus `json:"status"`
}

// Source identifies injected source bytes by logical path and content hash.
type Source struct {
	Path        string `json:"path"`
	MediaType   string `json:"media_type,omitempty"`
	SHA256      string `json:"sha256"`
	Description string `json:"description,omitempty"`
}

// SourceBytes is the in-memory input material available to a source reader.
type SourceBytes struct {
	Source Source `json:"source"`
	Bytes  []byte `json:"bytes"`
}

// SourceSelector declares the deterministic selector and window metadata for structured readers.
type SourceSelector struct {
	JSONPointer      string `json:"json_pointer"`
	JSONPath         string `json:"jsonpath,omitempty"`
	ByteSpan         string `json:"byte_span,omitempty"`
	LineWindow       string `json:"line_window,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
	RowWindow        string `json:"row_window,omitempty"`
	ColumnProjection string `json:"column_projection,omitempty"`
	Sheet            string `json:"sheet,omitempty"`
	Range            string `json:"range,omitempty"`
	TableName        string `json:"table_name,omitempty"`
	HeaderRow        int    `json:"header_row,omitempty"`
	Dialect          string `json:"dialect,omitempty"`
	Encoding         string `json:"encoding,omitempty"`
	FormulaPolicy    string `json:"formula_policy,omitempty"`
}

func (s SourceSelector) ValidatePhaseTwoSelector() error {
	if s.Cursor != "" && s.LineWindow == "" && s.RowWindow == "" {
		return fmt.Errorf("%w: cursor requires line_window or row_window", ErrInvalidSourceSelector)
	}

	if s.LineWindow != "" {
		if s.RowWindow != "" || s.ColumnProjection != "" || s.Sheet != "" || s.Range != "" || s.TableName != "" || s.HeaderRow != 0 || s.Dialect != "" || s.Encoding != "" || s.FormulaPolicy != "" {
			return fmt.Errorf("%w: ambiguous selector families", ErrInvalidSourceSelector)
		}
		return nil
	}

	if s.Sheet != "" || s.Range != "" || s.TableName != "" || s.FormulaPolicy != "" {
		if s.JSONPointer != "" || s.JSONPath != "" || s.ByteSpan != "" || s.HeaderRow != 0 || s.Dialect != "" || s.Encoding != "" || s.Cursor != "" {
			return fmt.Errorf("%w: ambiguous selector families", ErrInvalidSourceSelector)
		}
		if s.Sheet == "" && s.Range == "" && s.TableName == "" {
			return fmt.Errorf("%w: spreadsheet selector requires sheet, range, or table_name", ErrInvalidSourceSelector)
		}
		return nil
	}

	if s.RowWindow != "" || s.ColumnProjection != "" || s.HeaderRow != 0 || s.Dialect != "" || s.Encoding != "" || s.Cursor != "" {
		if s.JSONPointer != "" || s.JSONPath != "" || s.ByteSpan != "" {
			return fmt.Errorf("%w: ambiguous selector families", ErrInvalidSourceSelector)
		}
		if s.RowWindow == "" {
			return fmt.Errorf("%w: csv selector requires row_window", ErrInvalidSourceSelector)
		}
		return nil
	}

	if s.JSONPointer != "" || s.JSONPath != "" || s.ByteSpan != "" {
		if s.JSONPointer == "" {
			return fmt.Errorf("%w: json selector requires json_pointer", ErrInvalidSourceSelector)
		}
		return nil
	}

	return fmt.Errorf("%w: no supported selector family", ErrInvalidSourceSelector)
}

// CapturedRead records the deterministic evidence needed to replay a source read.
type CapturedRead struct {
	ID            string          `json:"id"`
	RunID         string          `json:"run_id"`
	SourcePath    string          `json:"source_path"`
	Origin        string          `json:"origin,omitempty"`
	Selector      SourceSelector  `json:"selector"`
	ReaderName    string          `json:"reader_name"`
	ReaderVersion string          `json:"reader_version"`
	ReaderOptions json.RawMessage `json:"reader_options,omitempty"`
	Locator       json.RawMessage `json:"locator,omitempty"`
	SourceSHA256  string          `json:"source_sha256"`
	OutputSHA256  string          `json:"output_sha256"`
	Output        json.RawMessage `json:"output"`
}

// Token is the kernel-owned provenance wrapper around one canonical value.
type Token struct {
	ID             string      `json:"id"`
	RunID          string      `json:"run_id"`
	TenantID       string      `json:"tenant_id"`
	ValueCanonical string      `json:"value_canonical"`
	Unit           string      `json:"unit,omitempty"`
	Source         TokenSource `json:"source"`
	Derivation     *Derivation `json:"derivation,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
}

// TokenSource links a token to the captured source read and path that produced it.
type TokenSource struct {
	CapturedReadID string         `json:"captured_read_id"`
	SourcePath     string         `json:"source_path"`
	SourceSelector SourceSelector `json:"source_selector"`
	OutputPath     string         `json:"output_path"`
	Origin         string         `json:"origin,omitempty"`
}

// Derivation records deterministic parent-token lineage for computed tokens.
type Derivation struct {
	Operation string   `json:"operation"`
	Inputs    []string `json:"inputs"`
	FormulaID string   `json:"formula_id,omitempty"`
}

// EntryKind names the deterministic audit event schema for an entry.
type EntryKind string

const (
	// EntryKindRunStarted records the start of a run.
	EntryKindRunStarted EntryKind = "run_started"
	// EntryKindTokenMint records a kernel-owned token mint.
	EntryKindTokenMint EntryKind = "token_mint"
	// EntryKindRunCompleted records successful run completion.
	EntryKindRunCompleted EntryKind = "run_completed"
	// EntryKindRunFailed records terminal run failure.
	EntryKindRunFailed EntryKind = "run_failed"
)

// AuditEntry is the in-memory shape projected to audit bundle rows later.
type AuditEntry struct {
	Seq      uint64          `json:"seq"`
	RunID    string          `json:"run_id"`
	TenantID string          `json:"tenant_id"`
	Kind     EntryKind       `json:"kind"`
	At       time.Time       `json:"at"`
	PrevHash []byte          `json:"prev_hash"`
	Hash     []byte          `json:"hash"`
	Payload  PayloadEnvelope `json:"payload"`
}

// PayloadEnvelope separates hash input from operational, non-deterministic data.
type PayloadEnvelope struct {
	Deterministic      json.RawMessage `json:"deterministic"`
	NonDeterministic   json.RawMessage `json:"non_deterministic,omitempty"`
	HashInputCanonical []byte          `json:"hash_input_canonical,omitempty"`
}

// ReadResult is the source-reader output plus replay evidence from one read.
type ReadResult struct {
	Output       json.RawMessage `json:"output"`
	CapturedRead CapturedRead    `json:"captured_read"`
}

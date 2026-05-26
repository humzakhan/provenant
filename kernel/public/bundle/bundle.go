package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/gowebpki/jcs"
	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/internal/phaseone"
)

const FormatVersion = "phase1.bundle/v1"

var (
	ErrTamperDetected  = errors.New("bundle: tamper detected")
	ErrChainTruncated  = errors.New("bundle: chain truncated")
	ErrMissingArtifact = errors.New("bundle: missing artifact")
)

// State is the minimal Phase 1 replayable state reconstructed from bundle bytes.
type State struct {
	Run           kernel.Run            `json:"run"`
	CapturedReads []kernel.CapturedRead `json:"captured_reads"`
	Tokens        []kernel.Token        `json:"tokens"`
	AuditEntries  []kernel.AuditEntry   `json:"-"`
}

type document struct {
	Format        string                `json:"format"`
	Run           kernel.Run            `json:"run"`
	CapturedReads []kernel.CapturedRead `json:"captured_reads"`
	Tokens        []kernel.Token        `json:"tokens"`
	AuditJSONL    string                `json:"audit_jsonl"`
}

type auditLine struct {
	At                      timeRFC3339Nano  `json:"at"`
	Hash                    string           `json:"hash"`
	Kind                    kernel.EntryKind `json:"kind"`
	PayloadDeterministic    json.RawMessage  `json:"payload_deterministic"`
	PayloadNonDeterministic json.RawMessage  `json:"payload_non_deterministic,omitempty"`
	PrevHash                string           `json:"prev_hash"`
	RunID                   string           `json:"run_id"`
	Seq                     uint64           `json:"seq"`
	TenantID                string           `json:"tenant_id"`
}

type timeRFC3339Nano struct {
	value string
}

func (t timeRFC3339Nano) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.value)
}

func (t *timeRFC3339Nano) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &t.value)
}

type tokenMintPayload struct {
	TokenID        string                `json:"token_id"`
	CapturedReadID string                `json:"captured_read_id"`
	SourcePath     string                `json:"source_path"`
	SourceSelector kernel.SourceSelector `json:"source_selector"`
	OutputPath     string                `json:"output_path"`
	Origin         string                `json:"origin,omitempty"`
	SourceName     string                `json:"source_name,omitempty"`
	ReaderName     string                `json:"reader_name"`
	ReaderVersion  string                `json:"reader_version"`
	ReaderOptions  json.RawMessage       `json:"reader_options,omitempty"`
	Locator        json.RawMessage       `json:"locator,omitempty"`
	SourceSHA256   string                `json:"source_sha256"`
	OutputSHA256   string                `json:"output_sha256"`
	SourceValueRaw string                `json:"source_value_raw"`
	ValueCanonical string                `json:"value_canonical"`
	Unit           string                `json:"unit,omitempty"`
}

// Emit returns the smallest deterministic Phase 1 bundle bytes for the provided in-memory state.
func Emit(state State) ([]byte, error) {
	normalized, err := normalizeState(state)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(normalized.CapturedReads, func(left, right kernel.CapturedRead) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(normalized.Tokens, func(left, right kernel.Token) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortFunc(normalized.AuditEntries, func(left, right kernel.AuditEntry) int {
		switch {
		case left.Seq < right.Seq:
			return -1
		case left.Seq > right.Seq:
			return 1
		default:
			return 0
		}
	})
	auditJSONL, err := marshalAuditJSONL(normalized.AuditEntries)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(document{
		Format:        FormatVersion,
		Run:           normalized.Run,
		CapturedReads: normalized.CapturedReads,
		Tokens:        normalized.Tokens,
		AuditJSONL:    string(auditJSONL),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal bundle: %v", ErrTamperDetected, err)
	}

	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical bundle: %v", ErrTamperDetected, err)
	}
	return canonical, nil
}

// Verify checks bundle integrity using only the provided bundle bytes.
func Verify(bundleBytes []byte) error {
	_, err := Replay(bundleBytes)
	return err
}

// Replay verifies bundle integrity and reconstructs deterministic Phase 1 state in memory.
func Replay(bundleBytes []byte) (State, error) {
	doc, err := parseDocument(bundleBytes)
	if err != nil {
		return State{}, err
	}
	if doc.Format != FormatVersion {
		return State{}, fmt.Errorf("%w: unexpected format %q", ErrTamperDetected, doc.Format)
	}
	if doc.AuditJSONL == "" {
		return State{}, fmt.Errorf("%w: audit_jsonl", ErrMissingArtifact)
	}

	entries, err := parseAuditJSONL(doc.AuditJSONL)
	if err != nil {
		return State{}, err
	}

	state := State{
		Run:           doc.Run,
		CapturedReads: doc.CapturedReads,
		Tokens:        doc.Tokens,
		AuditEntries:  entries,
	}
	normalized, err := normalizeState(state)
	if err != nil {
		return State{}, err
	}
	return normalized, nil
}

func parseDocument(bundleBytes []byte) (document, error) {
	if len(bundleBytes) == 0 {
		return document{}, ErrChainTruncated
	}

	dec := json.NewDecoder(bytes.NewReader(bundleBytes))
	dec.DisallowUnknownFields()

	var doc document
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return document{}, ErrChainTruncated
		}
		return document{}, fmt.Errorf("%w: parse bundle: %v", ErrTamperDetected, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return document{}, ErrChainTruncated
		}
		return document{}, fmt.Errorf("%w: trailing bundle bytes", ErrTamperDetected)
	}
	return doc, nil
}

func marshalAuditJSONL(entries []kernel.AuditEntry) ([]byte, error) {
	var buf bytes.Buffer
	for _, entry := range entries {
		line := auditLine{
			At:                   timeRFC3339Nano{value: entry.At.UTC().Format(time.RFC3339Nano)},
			Hash:                 hex.EncodeToString(entry.Hash),
			Kind:                 entry.Kind,
			PayloadDeterministic: append([]byte(nil), entry.Payload.Deterministic...),
			PrevHash:             hex.EncodeToString(entry.PrevHash),
			RunID:                entry.RunID,
			Seq:                  entry.Seq,
			TenantID:             entry.TenantID,
		}
		if len(entry.Payload.NonDeterministic) > 0 {
			line.PayloadNonDeterministic = append([]byte(nil), entry.Payload.NonDeterministic...)
		}

		raw, err := json.Marshal(line)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal audit line: %v", ErrTamperDetected, err)
		}
		canonical, err := jcs.Transform(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: canonical audit line: %v", ErrTamperDetected, err)
		}
		buf.Write(canonical)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func parseAuditJSONL(auditJSONL string) ([]kernel.AuditEntry, error) {
	if !strings.HasSuffix(auditJSONL, "\n") {
		return nil, ErrChainTruncated
	}

	body := strings.TrimSuffix(auditJSONL, "\n")
	if body == "" {
		return nil, nil
	}

	lines := strings.Split(body, "\n")
	entries := make([]kernel.AuditEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			return nil, ErrChainTruncated
		}

		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()

		var parsed auditLine
		if err := dec.Decode(&parsed); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, ErrChainTruncated
			}
			return nil, fmt.Errorf("%w: parse audit line: %v", ErrTamperDetected, err)
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return nil, ErrChainTruncated
			}
			return nil, fmt.Errorf("%w: trailing audit line bytes", ErrTamperDetected)
		}

		prevHash, err := hex.DecodeString(parsed.PrevHash)
		if err != nil {
			return nil, fmt.Errorf("%w: prev_hash: %v", ErrTamperDetected, err)
		}
		hash, err := hex.DecodeString(parsed.Hash)
		if err != nil {
			return nil, fmt.Errorf("%w: hash: %v", ErrTamperDetected, err)
		}
		deterministic, err := jcs.Transform(parsed.PayloadDeterministic)
		if err != nil {
			return nil, fmt.Errorf("%w: canonical deterministic payload: %v", ErrTamperDetected, err)
		}

		at, err := time.Parse(time.RFC3339Nano, parsed.At.value)
		if err != nil {
			return nil, fmt.Errorf("%w: parse audit time: %v", ErrTamperDetected, err)
		}

		entries = append(entries, kernel.AuditEntry{
			Seq:      parsed.Seq,
			RunID:    parsed.RunID,
			TenantID: parsed.TenantID,
			Kind:     parsed.Kind,
			At:       at,
			PrevHash: prevHash,
			Hash:     hash,
			Payload: kernel.PayloadEnvelope{
				Deterministic:      deterministic,
				NonDeterministic:   append([]byte(nil), parsed.PayloadNonDeterministic...),
				HashInputCanonical: deterministic,
			},
		})
	}
	return entries, nil
}

func normalizeState(state State) (State, error) {
	state = cloneState(state)
	if state.Run.ID == "" || state.Run.TenantID == "" {
		return State{}, fmt.Errorf("%w: missing run identity", ErrTamperDetected)
	}
	// Count audit entries that produce tokens (token_mint, token_derive).
	// Run lifecycle entries (run_started, run_completed, run_failed) do not
	// produce tokens and are not counted in the token constraint.
	tokenEntryCount := 0
	for _, e := range state.AuditEntries {
		if e.Kind == kernel.EntryKindTokenMint || e.Kind == kernel.EntryKindTokenDerive {
			tokenEntryCount++
		}
	}
	if tokenEntryCount != len(state.Tokens) {
		return State{}, fmt.Errorf("%w: expected one audit per token and captured reads referenced by tokens", ErrTamperDetected)
	}
	// If there are tokens, there must be at least one captured read and no more
	// captured reads than tokens (a captured read is a source for at least one token).
	if len(state.Tokens) > 0 && (len(state.CapturedReads) == 0 || len(state.CapturedReads) > len(state.Tokens)) {
		return State{}, fmt.Errorf("%w: expected one audit per token and captured reads referenced by tokens", ErrTamperDetected)
	}

	readsByID := make(map[string]kernel.CapturedRead, len(state.CapturedReads))
	for _, read := range state.CapturedReads {
		if read.ID == "" {
			return State{}, fmt.Errorf("%w: captured read id", ErrTamperDetected)
		}
		if read.RunID != state.Run.ID {
			return State{}, fmt.Errorf("%w: captured read %s run mismatch", ErrTamperDetected, read.ID)
		}
		if _, exists := readsByID[read.ID]; exists {
			return State{}, fmt.Errorf("%w: duplicate captured read %s", ErrTamperDetected, read.ID)
		}
		if sha256Hex(read.Output) != read.OutputSHA256 {
			return State{}, fmt.Errorf("%w: captured read %s output hash mismatch", ErrTamperDetected, read.ID)
		}
		readsByID[read.ID] = cloneCapturedRead(read)
	}

	tokensByID := make(map[string]kernel.Token, len(state.Tokens))
	for _, token := range state.Tokens {
		if token.ID == "" {
			return State{}, fmt.Errorf("%w: token id", ErrTamperDetected)
		}
		if token.RunID != state.Run.ID || token.TenantID != state.Run.TenantID {
			return State{}, fmt.Errorf("%w: token %s run or tenant mismatch", ErrTamperDetected, token.ID)
		}
		if _, exists := tokensByID[token.ID]; exists {
			return State{}, fmt.Errorf("%w: duplicate token %s", ErrTamperDetected, token.ID)
		}
		// Leaf tokens must reference a captured read; derived tokens must not.
		if token.Derivation == nil {
			if _, ok := readsByID[token.Source.CapturedReadID]; !ok {
				return State{}, fmt.Errorf("%w: captured read %s", ErrMissingArtifact, token.Source.CapturedReadID)
			}
		}
		tokensByID[token.ID] = cloneToken(token)
	}

	normalizedTokens := make([]kernel.Token, 0, len(state.AuditEntries))
	// seqByTokenID tracks the audit seq at which each token was minted or
	// derived, enabling forward-reference checks for derivation inputs.
	seqByTokenID := make(map[string]uint64, len(state.AuditEntries))
	referencedReadIDs := make(map[string]struct{}, len(state.CapturedReads))
	var prevHash []byte
	for index, entry := range state.AuditEntries {
		if entry.RunID != state.Run.ID || entry.TenantID != state.Run.TenantID {
			return State{}, fmt.Errorf("%w: audit entry %d run or tenant mismatch", ErrTamperDetected, entry.Seq)
		}
		if entry.Seq != uint64(index+1) {
			return State{}, fmt.Errorf("%w: unexpected audit seq %d", ErrTamperDetected, entry.Seq)
		}

		wantPrevHash := prevHash
		if index == 0 {
			wantPrevHash = genesisHash(state.Run.TenantID)
		}
		if !bytes.Equal(entry.PrevHash, wantPrevHash) {
			return State{}, fmt.Errorf("%w: invalid prev_hash at seq %d", ErrTamperDetected, entry.Seq)
		}

		deterministic, err := jcs.Transform(entry.Payload.Deterministic)
		if err != nil {
			return State{}, fmt.Errorf("%w: canonical deterministic payload at seq %d: %v", ErrTamperDetected, entry.Seq, err)
		}
		if len(entry.Payload.HashInputCanonical) > 0 && !bytes.Equal(entry.Payload.HashInputCanonical, deterministic) {
			return State{}, fmt.Errorf("%w: hash input canonical mismatch at seq %d", ErrTamperDetected, entry.Seq)
		}

		wantHash := auditHash(entry.PrevHash, entry.Seq, entry.RunID, entry.TenantID, entry.Kind, deterministic)
		if !bytes.Equal(entry.Hash, wantHash) {
			return State{}, fmt.Errorf("%w: invalid audit hash at seq %d", ErrTamperDetected, entry.Seq)
		}

		switch entry.Kind {
		case kernel.EntryKindTokenMint:
			payload, err := decodeTokenMintPayload(deterministic)
			if err != nil {
				return State{}, fmt.Errorf("%w: parse token_mint payload at seq %d: %v", ErrTamperDetected, entry.Seq, err)
			}

			token, ok := tokensByID[payload.TokenID]
			if !ok {
				return State{}, fmt.Errorf("%w: token %s", ErrMissingArtifact, payload.TokenID)
			}
			read, ok := readsByID[payload.CapturedReadID]
			if !ok {
				return State{}, fmt.Errorf("%w: captured read %s", ErrMissingArtifact, payload.CapturedReadID)
			}
			evidence, err := phaseone.ReconstructMintEvidence(state.Run.ID, read.ID, read.Output, payload.OutputPath, payload.Unit)
			if err != nil {
				return State{}, fmt.Errorf("%w: reconstruct mint evidence for %s: %v", ErrTamperDetected, read.ID, err)
			}
			if err := validateEvidenceAgainstPayload(evidence, payload); err != nil {
				return State{}, err
			}
			if err := validateTokenAgainstPayload(token, payload, state.Run, read, evidence); err != nil {
				return State{}, err
			}
			if err := validateCapturedReadAgainstPayload(read, payload); err != nil {
				return State{}, err
			}
			referencedReadIDs[payload.CapturedReadID] = struct{}{}
			normalizedTokens = append(normalizedTokens, deterministicToken(state.Run, read, payload, evidence))
			seqByTokenID[payload.TokenID] = entry.Seq

		case kernel.EntryKindTokenDerive:
			payload, err := decodeTokenDerivePayload(deterministic, seqByTokenID, entry.Seq)
			if err != nil {
				return State{}, fmt.Errorf("%w: parse token_derive payload at seq %d: %v", ErrTamperDetected, entry.Seq, err)
			}

			token, ok := tokensByID[payload.TokenID]
			if !ok {
				return State{}, fmt.Errorf("%w: token %s", ErrMissingArtifact, payload.TokenID)
			}
			if err := validateDerivedTokenAgainstPayload(token, payload, state.Run); err != nil {
				return State{}, err
			}
			normalizedTokens = append(normalizedTokens, deterministicDerivedToken(state.Run, payload))
			seqByTokenID[payload.TokenID] = entry.Seq

		case kernel.EntryKindRunStarted, kernel.EntryKindRunCompleted, kernel.EntryKindRunFailed:
			// Run lifecycle entries carry no token and no payload beyond
			// seq/prev_hash bookkeeping. Validate that the deterministic payload
			// is an empty JSON object so the hash chain is stable.
			if err := validateRunLifecyclePayload(deterministic, entry.Seq); err != nil {
				return State{}, err
			}
			// No token produced; no contribution to referencedReadIDs.

		case kernel.EntryKindAnswerSubmitted:
			// answer_submitted entries do not produce a token; they reference one
			// that must already exist earlier in the chain (strictly lower seq).
			payload, err := decodeAnswerSubmittedPayload(deterministic)
			if err != nil {
				return State{}, fmt.Errorf("%w: parse answer_submitted payload at seq %d: %v", ErrTamperDetected, entry.Seq, err)
			}
			tokenSeq, ok := seqByTokenID[payload.TokenID]
			if !ok {
				return State{}, fmt.Errorf("%w: answer_submitted at seq %d references unknown token %s", ErrTamperDetected, entry.Seq, payload.TokenID)
			}
			if tokenSeq >= entry.Seq {
				return State{}, fmt.Errorf("%w: answer_submitted at seq %d references token %s at seq %d (not strictly prior)", ErrTamperDetected, entry.Seq, payload.TokenID, tokenSeq)
			}
			// No token produced; no contribution to referencedReadIDs.

		default:
			return State{}, fmt.Errorf("%w: unsupported audit kind %s", ErrTamperDetected, entry.Kind)
		}

		prevHash = entry.Hash
	}
	// Only enforce the "every captured read is referenced" invariant when
	// there are tokens. A bundle containing only run lifecycle entries has
	// no captured reads, so there is nothing to check.
	if len(state.Tokens) > 0 {
		for _, read := range state.CapturedReads {
			if _, ok := referencedReadIDs[read.ID]; !ok {
				return State{}, fmt.Errorf("%w: unreferenced captured read %s", ErrTamperDetected, read.ID)
			}
		}
	}

	state.Tokens = normalizedTokens
	return cloneState(state), nil
}

func validateTokenAgainstPayload(token kernel.Token, payload tokenMintPayload, run kernel.Run, read kernel.CapturedRead, evidence phaseone.MintEvidence) error {
	want := deterministicToken(run, read, payload, evidence)
	switch {
	case token.Derivation != nil:
		// Leaf tokens minted from a captured read must not carry a Derivation.
		// This enforces the Token-shape invariant documented on kernel.Token:
		// exactly one of Source.CapturedReadID or Derivation must be set.
		return fmt.Errorf("%w: leaf token %s must not carry Derivation", ErrTamperDetected, token.ID)
	case token.ID != want.ID:
		return fmt.Errorf("%w: token id mismatch for %s", ErrTamperDetected, token.ID)
	case token.RunID != want.RunID:
		return fmt.Errorf("%w: token %s run mismatch", ErrTamperDetected, token.ID)
	case token.TenantID != want.TenantID:
		return fmt.Errorf("%w: token %s tenant mismatch", ErrTamperDetected, token.ID)
	case token.Source.CapturedReadID != want.Source.CapturedReadID:
		return fmt.Errorf("%w: token %s captured read mismatch", ErrTamperDetected, token.ID)
	case token.Source.SourcePath != want.Source.SourcePath:
		return fmt.Errorf("%w: token %s source path mismatch", ErrTamperDetected, token.ID)
	case token.Source.SourceSelector != want.Source.SourceSelector:
		return fmt.Errorf("%w: token %s selector mismatch", ErrTamperDetected, token.ID)
	case token.Source.OutputPath != want.Source.OutputPath:
		return fmt.Errorf("%w: token %s output path mismatch", ErrTamperDetected, token.ID)
	case token.Source.Origin != want.Source.Origin:
		return fmt.Errorf("%w: token %s origin mismatch", ErrTamperDetected, token.ID)
	case token.ValueCanonical != want.ValueCanonical:
		return fmt.Errorf("%w: token %s canonical value mismatch", ErrTamperDetected, token.ID)
	case token.Unit != want.Unit:
		return fmt.Errorf("%w: token %s unit mismatch", ErrTamperDetected, token.ID)
	default:
		return nil
	}
}

func validateCapturedReadAgainstPayload(read kernel.CapturedRead, payload tokenMintPayload) error {
	switch {
	case read.ID != payload.CapturedReadID:
		return fmt.Errorf("%w: captured read id mismatch for %s", ErrTamperDetected, read.ID)
	case read.SourcePath != payload.SourcePath:
		return fmt.Errorf("%w: captured read %s source path mismatch", ErrTamperDetected, read.ID)
	case read.Selector != payload.SourceSelector:
		return fmt.Errorf("%w: captured read %s selector mismatch", ErrTamperDetected, read.ID)
	case read.Origin != payload.Origin:
		return fmt.Errorf("%w: captured read %s origin mismatch", ErrTamperDetected, read.ID)
	case read.SourceName != payload.SourceName:
		return fmt.Errorf("%w: captured read %s source_name mismatch", ErrTamperDetected, read.ID)
	case read.ReaderName != payload.ReaderName:
		return fmt.Errorf("%w: captured read %s reader name mismatch", ErrTamperDetected, read.ID)
	case read.ReaderVersion != payload.ReaderVersion:
		return fmt.Errorf("%w: captured read %s reader version mismatch", ErrTamperDetected, read.ID)
	case !bytes.Equal(read.ReaderOptions, payload.ReaderOptions):
		return fmt.Errorf("%w: captured read %s reader options mismatch", ErrTamperDetected, read.ID)
	case !bytes.Equal(read.Locator, payload.Locator):
		return fmt.Errorf("%w: captured read %s locator mismatch", ErrTamperDetected, read.ID)
	case read.SourceSHA256 != payload.SourceSHA256:
		return fmt.Errorf("%w: captured read %s source hash mismatch", ErrTamperDetected, read.ID)
	case read.OutputSHA256 != payload.OutputSHA256:
		return fmt.Errorf("%w: captured read %s output hash mismatch", ErrTamperDetected, read.ID)
	default:
		return nil
	}
}

func validateEvidenceAgainstPayload(evidence phaseone.MintEvidence, payload tokenMintPayload) error {
	switch {
	case evidence.SourceValueRaw != payload.SourceValueRaw:
		return fmt.Errorf("%w: source value raw mismatch for %s", ErrTamperDetected, payload.CapturedReadID)
	case evidence.ValueCanonical != payload.ValueCanonical:
		return fmt.Errorf("%w: canonical value mismatch for %s", ErrTamperDetected, payload.CapturedReadID)
	case evidence.TokenID != payload.TokenID:
		return fmt.Errorf("%w: token id mismatch for %s", ErrTamperDetected, payload.CapturedReadID)
	default:
		return nil
	}
}

func deterministicToken(run kernel.Run, read kernel.CapturedRead, payload tokenMintPayload, evidence phaseone.MintEvidence) kernel.Token {
	return kernel.Token{
		ID:             evidence.TokenID,
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: evidence.ValueCanonical,
		Unit:           payload.Unit,
		Source: kernel.TokenSource{
			CapturedReadID: read.ID,
			SourcePath:     read.SourcePath,
			SourceSelector: read.Selector,
			OutputPath:     payload.OutputPath,
			Origin:         read.Origin,
		},
	}
}

func decodeTokenMintPayload(deterministic []byte) (tokenMintPayload, error) {
	dec := json.NewDecoder(bytes.NewReader(deterministic))
	dec.DisallowUnknownFields()

	var payload tokenMintPayload
	if err := dec.Decode(&payload); err != nil {
		return tokenMintPayload{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return tokenMintPayload{}, errors.New("trailing payload bytes")
	}
	selectorErr := payload.SourceSelector.ValidatePhaseTwoSelector()
	switch {
	case payload.TokenID == "":
		return tokenMintPayload{}, errors.New("missing token_id")
	case payload.CapturedReadID == "":
		return tokenMintPayload{}, errors.New("missing captured_read_id")
	case payload.SourcePath == "":
		return tokenMintPayload{}, errors.New("missing source_path")
	case selectorErr != nil:
		return tokenMintPayload{}, fmt.Errorf("invalid source_selector: %w", selectorErr)
	case payload.OutputPath == "":
		return tokenMintPayload{}, errors.New("missing output_path")
	case payload.ReaderName == "":
		return tokenMintPayload{}, errors.New("missing reader_name")
	case payload.ReaderVersion == "":
		return tokenMintPayload{}, errors.New("missing reader_version")
	case payload.SourceSHA256 == "":
		return tokenMintPayload{}, errors.New("missing source_sha256")
	case payload.OutputSHA256 == "":
		return tokenMintPayload{}, errors.New("missing output_sha256")
	case payload.SourceValueRaw == "":
		return tokenMintPayload{}, errors.New("missing source_value_raw")
	case payload.ValueCanonical == "":
		return tokenMintPayload{}, errors.New("missing value_canonical")
	default:
		return payload, nil
	}
}

// tokenDerivePayload is the in-memory representation of a token_derive
// deterministic audit payload. It mirrors tokenMintPayload for the leaf path.
// Matched and Predicate are only set for filter.sum_where.
type tokenDerivePayload struct {
	TokenID   string                `json:"token_id"`
	Operation string                `json:"operation"`
	Inputs    []string              `json:"inputs"`
	Matched   []string              `json:"matched,omitempty"`
	Predicate *kernel.RowPredicate  `json:"predicate,omitempty"`
	FormulaID string                `json:"formula_id,omitempty"`
	Rounding  kernel.RoundingPolicy `json:"rounding,omitempty"`
	Evaluated string                `json:"evaluated"`
	Unit      string                `json:"unit,omitempty"`
}

// validRoundingModes is the closed set of accepted rounding mode strings.
// The empty string represents the default "half_even" behaviour.
var validRoundingModes = map[string]bool{
	"":          true,
	"half_even": true,
	"half_up":   true,
	"down":      true,
}

// decodeTokenDerivePayload parses and validates a token_derive deterministic
// payload. seqByTokenID maps already-seen token IDs to their audit seq; currentSeq
// is the seq of the current entry, used to reject forward references.
func decodeTokenDerivePayload(deterministic []byte, seqByTokenID map[string]uint64, currentSeq uint64) (tokenDerivePayload, error) {
	dec := json.NewDecoder(bytes.NewReader(deterministic))
	dec.DisallowUnknownFields()

	var payload tokenDerivePayload
	if err := dec.Decode(&payload); err != nil {
		return tokenDerivePayload{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return tokenDerivePayload{}, errors.New("trailing payload bytes")
	}

	switch {
	case payload.TokenID == "":
		return tokenDerivePayload{}, errors.New("missing token_id")
	case payload.Operation == "":
		return tokenDerivePayload{}, errors.New("missing operation")
	case payload.Evaluated == "":
		return tokenDerivePayload{}, errors.New("missing evaluated")
	case len(payload.Inputs) == 0:
		return tokenDerivePayload{}, errors.New("inputs must be non-empty")
	case !validRoundingModes[payload.Rounding.Mode]:
		return tokenDerivePayload{}, fmt.Errorf("invalid rounding mode %q", payload.Rounding.Mode)
	case payload.Rounding.Scale < 0 || payload.Rounding.Scale > 18:
		return tokenDerivePayload{}, fmt.Errorf("rounding scale %d out of range [0, 18]", payload.Rounding.Scale)
	}

	for _, inputID := range payload.Inputs {
		inputSeq, ok := seqByTokenID[inputID]
		if !ok {
			return tokenDerivePayload{}, fmt.Errorf("input token %s not found in prior audit entries", inputID)
		}
		if inputSeq >= currentSeq {
			return tokenDerivePayload{}, fmt.Errorf("input token %s has seq %d, not strictly less than current seq %d (forward reference or cycle)", inputID, inputSeq, currentSeq)
		}
	}

	// filter.sum_where-specific field checks.
	isFilter := payload.Operation == "filter.sum_where"
	if !isFilter && (len(payload.Matched) > 0 || payload.Predicate != nil) {
		return tokenDerivePayload{}, fmt.Errorf("predicate and matched fields are only valid for filter.sum_where, got operation %q", payload.Operation)
	}
	if isFilter && len(payload.Matched) > 0 {
		if err := validateMatchedSubsetBundle(payload.Inputs, payload.Matched); err != nil {
			return tokenDerivePayload{}, err
		}
	}

	return payload, nil
}

// answerSubmittedPayload is the in-memory representation of an answer_submitted
// deterministic audit payload. It references a token that already exists in the
// chain.
type answerSubmittedPayload struct {
	TokenID        string `json:"token_id"`
	Label          string `json:"label"`
	ValueCanonical string `json:"value_canonical"`
	Unit           string `json:"unit,omitempty"`
}

func decodeAnswerSubmittedPayload(deterministic []byte) (answerSubmittedPayload, error) {
	dec := json.NewDecoder(bytes.NewReader(deterministic))
	dec.DisallowUnknownFields()

	var payload answerSubmittedPayload
	if err := dec.Decode(&payload); err != nil {
		return answerSubmittedPayload{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return answerSubmittedPayload{}, errors.New("trailing payload bytes")
	}
	switch {
	case payload.TokenID == "":
		return answerSubmittedPayload{}, errors.New("missing token_id")
	case payload.Label == "":
		return answerSubmittedPayload{}, errors.New("missing label")
	case payload.ValueCanonical == "":
		return answerSubmittedPayload{}, errors.New("missing value_canonical")
	}
	return payload, nil
}

// validateMatchedSubsetBundle checks that matched is a subsequence of inputs.
func validateMatchedSubsetBundle(inputs, matched []string) error {
	inputIdx := make(map[string]int, len(inputs))
	for i, id := range inputs {
		inputIdx[id] = i
	}
	prev := -1
	for _, id := range matched {
		pos, ok := inputIdx[id]
		if !ok {
			return fmt.Errorf("matched token %q not found in inputs", id)
		}
		if pos <= prev {
			return fmt.Errorf("matched tokens are not in input order at %q", id)
		}
		prev = pos
	}
	return nil
}

func validateDerivedTokenAgainstPayload(token kernel.Token, payload tokenDerivePayload, run kernel.Run) error {
	want := deterministicDerivedToken(run, payload)
	switch {
	case token.ID != want.ID:
		return fmt.Errorf("%w: derived token id mismatch for %s", ErrTamperDetected, token.ID)
	case token.RunID != want.RunID:
		return fmt.Errorf("%w: derived token %s run mismatch", ErrTamperDetected, token.ID)
	case token.TenantID != want.TenantID:
		return fmt.Errorf("%w: derived token %s tenant mismatch", ErrTamperDetected, token.ID)
	case token.Unit != want.Unit:
		return fmt.Errorf("%w: derived token %s unit mismatch", ErrTamperDetected, token.ID)
	case token.ValueCanonical != payload.Evaluated:
		return fmt.Errorf("%w: derived token %s value_canonical %q != payload evaluated %q", ErrTamperDetected, token.ID, token.ValueCanonical, payload.Evaluated)
	case token.Source != (kernel.TokenSource{}):
		return fmt.Errorf("%w: derived token %s must have zero Source", ErrTamperDetected, token.ID)
	case token.Derivation == nil:
		return fmt.Errorf("%w: derived token %s missing Derivation", ErrTamperDetected, token.ID)
	case token.Derivation.Operation != payload.Operation:
		return fmt.Errorf("%w: derived token %s operation mismatch", ErrTamperDetected, token.ID)
	case token.Derivation.FormulaID != payload.FormulaID:
		return fmt.Errorf("%w: derived token %s formula_id mismatch", ErrTamperDetected, token.ID)
	case token.Derivation.Rounding != payload.Rounding:
		return fmt.Errorf("%w: derived token %s rounding mismatch", ErrTamperDetected, token.ID)
	case token.Derivation.Evaluated != payload.Evaluated:
		return fmt.Errorf("%w: derived token %s derivation.evaluated mismatch", ErrTamperDetected, token.ID)
	}
	// Validate inputs slice equality.
	if len(token.Derivation.Inputs) != len(payload.Inputs) {
		return fmt.Errorf("%w: derived token %s inputs length mismatch", ErrTamperDetected, token.ID)
	}
	for i, id := range token.Derivation.Inputs {
		if id != payload.Inputs[i] {
			return fmt.Errorf("%w: derived token %s input[%d] mismatch", ErrTamperDetected, token.ID, i)
		}
	}
	return nil
}

// validateRunLifecyclePayload checks that a run lifecycle audit entry
// (run_started, run_completed, run_failed) carries an empty JSON object as its
// deterministic payload. An empty object is canonical: `{}`. Any other payload
// is rejected as a tamper attempt.
func validateRunLifecyclePayload(deterministic []byte, seq uint64) error {
	// The canonical form of an empty object is exactly `{}`.
	trimmed := bytes.TrimSpace(deterministic)
	if string(trimmed) != "{}" {
		return fmt.Errorf("%w: run lifecycle entry at seq %d must have empty payload, got %s", ErrTamperDetected, seq, trimmed)
	}
	return nil
}

func deterministicDerivedToken(run kernel.Run, payload tokenDerivePayload) kernel.Token {
	inputs := append([]string(nil), payload.Inputs...)
	return kernel.Token{
		ID:             payload.TokenID,
		RunID:          run.ID,
		TenantID:       run.TenantID,
		ValueCanonical: payload.Evaluated,
		Unit:           payload.Unit,
		Derivation: &kernel.Derivation{
			Operation: payload.Operation,
			Inputs:    inputs,
			FormulaID: payload.FormulaID,
			Rounding:  payload.Rounding,
			Evaluated: payload.Evaluated,
		},
	}
}

func cloneState(state State) State {
	cloned := State{
		Run:           state.Run,
		CapturedReads: make([]kernel.CapturedRead, len(state.CapturedReads)),
		Tokens:        make([]kernel.Token, len(state.Tokens)),
		AuditEntries:  make([]kernel.AuditEntry, len(state.AuditEntries)),
	}
	for i, read := range state.CapturedReads {
		cloned.CapturedReads[i] = cloneCapturedRead(read)
	}
	for i, token := range state.Tokens {
		cloned.Tokens[i] = cloneToken(token)
	}
	for i, entry := range state.AuditEntries {
		cloned.AuditEntries[i] = cloneAuditEntry(entry)
	}
	return cloned
}

func cloneCapturedRead(read kernel.CapturedRead) kernel.CapturedRead {
	cloned := read
	cloned.ReaderOptions = append([]byte(nil), read.ReaderOptions...)
	cloned.Locator = append([]byte(nil), read.Locator...)
	cloned.Output = append([]byte(nil), read.Output...)
	return cloned
}

func cloneToken(token kernel.Token) kernel.Token {
	cloned := token
	if token.Derivation != nil {
		derivation := *token.Derivation
		derivation.Inputs = append([]string(nil), token.Derivation.Inputs...)
		cloned.Derivation = &derivation
	}
	return cloned
}

func cloneAuditEntry(entry kernel.AuditEntry) kernel.AuditEntry {
	cloned := entry
	cloned.PrevHash = append([]byte(nil), entry.PrevHash...)
	cloned.Hash = append([]byte(nil), entry.Hash...)
	cloned.Payload.Deterministic = append([]byte(nil), entry.Payload.Deterministic...)
	cloned.Payload.NonDeterministic = append([]byte(nil), entry.Payload.NonDeterministic...)
	cloned.Payload.HashInputCanonical = append([]byte(nil), entry.Payload.HashInputCanonical...)
	return cloned
}

func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

func auditHash(prevHash []byte, seq uint64, runID, tenantID string, kind kernel.EntryKind, hashInput []byte) []byte {
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, seq)

	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(kind))
	h.Write(hashInput)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

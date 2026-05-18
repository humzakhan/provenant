package kernel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
	"github.com/shopspring/decimal"
)

var (
	ErrNilTransactionalScope          = errors.New("kernel: transactional scope is required")
	ErrNilSourceResolver              = errors.New("kernel: source resolver is required")
	ErrNilSourceReader                = errors.New("kernel: source reader is required")
	ErrNilStore                       = errors.New("kernel: store is required")
	ErrNilAuditSink                   = errors.New("kernel: audit sink is required")
	ErrNilClock                       = errors.New("kernel: clock is required")
	ErrMissingRunID                   = errors.New("kernel: run id is required")
	ErrMissingTenantID                = errors.New("kernel: tenant id is required")
	ErrMissingSourcePath              = errors.New("kernel: source path is required")
	ErrMissingOutputPath              = errors.New("kernel: output path is required")
	ErrUnsupportedOutputPath          = errors.New("kernel: unsupported output path")
	ErrOutputPathNotFound             = errors.New("kernel: output path not found")
	ErrMintValueNotNumeric            = errors.New("kernel: mint value is not numeric")
	ErrTransactionalScopeMismatch     = errors.New("kernel: transactional substrate mismatch")
	ErrCapturedReadRunIDMismatch      = errors.New("kernel: captured read run id mismatch")
	ErrCapturedReadPathMismatch       = errors.New("kernel: captured read source path mismatch")
	ErrCapturedReadSelectorMismatch   = errors.New("kernel: captured read selector mismatch")
	ErrCapturedReadSourceHashMismatch = errors.New("kernel: captured read source hash mismatch")
	ErrCapturedReadOutputHashMismatch = errors.New("kernel: captured read output hash mismatch")
	ErrCapturedReadOutputMismatch     = errors.New("kernel: captured read output mismatch")
)

var canonicalDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// ReadAndMintRequest identifies one source read plus the mintable output path.
type ReadAndMintRequest struct {
	SourcePath string         `json:"source_path"`
	Selector   SourceSelector `json:"selector"`
	OutputPath string         `json:"output_path"`
	Unit       string         `json:"unit,omitempty"`
}

// ReadAndMintResult returns the captured read, minted token, and appended audit entry.
type ReadAndMintResult struct {
	ReadResult ReadResult `json:"read_result"`
	Token      Token      `json:"token"`
	AuditEntry AuditEntry `json:"audit_entry"`
}

// ReadAndMint resolves one injected source, runs the reader, and mints a token from the selected output path.
func ReadAndMint(
	ctx context.Context,
	scope TransactionalScope,
	resolver SourceResolver,
	reader SourceReader,
	store Store,
	auditSink AuditSink,
	clock Clock,
	run Run,
	request ReadAndMintRequest,
) (ReadAndMintResult, error) {
	if scope == nil {
		return ReadAndMintResult{}, ErrNilTransactionalScope
	}
	if resolver == nil {
		return ReadAndMintResult{}, ErrNilSourceResolver
	}
	if reader == nil {
		return ReadAndMintResult{}, ErrNilSourceReader
	}
	if store == nil {
		return ReadAndMintResult{}, ErrNilStore
	}
	if auditSink == nil {
		return ReadAndMintResult{}, ErrNilAuditSink
	}
	if clock == nil {
		return ReadAndMintResult{}, ErrNilClock
	}
	if run.ID == "" {
		return ReadAndMintResult{}, ErrMissingRunID
	}
	if run.TenantID == "" {
		return ReadAndMintResult{}, ErrMissingTenantID
	}
	if request.SourcePath == "" {
		return ReadAndMintResult{}, ErrMissingSourcePath
	}
	if request.OutputPath == "" {
		return ReadAndMintResult{}, ErrMissingOutputPath
	}
	if scope.TransactionalSubstrate() != store.TransactionalSubstrate() || scope.TransactionalSubstrate() != auditSink.TransactionalSubstrate() {
		return ReadAndMintResult{}, ErrTransactionalScopeMismatch
	}

	var result ReadAndMintResult
	if err := scope.WithTx(ctx, func(ctx context.Context, tx Tx) error {
		source, err := resolver.ResolveSource(ctx, request.SourcePath)
		if err != nil {
			return err
		}

		read, err := reader.Read(ctx, run, source, request.Selector)
		if err != nil {
			return err
		}
		if err := validateCapturedRead(source, read, run, request); err != nil {
			return err
		}

		rawValue, err := extractMintValue(read.Output, request.OutputPath)
		if err != nil {
			return err
		}
		sourceValueRaw, err := normalizeSourceValueRaw(rawValue)
		if err != nil {
			return err
		}

		valueCanonical, err := canonicalizeNumeric(rawValue)
		if err != nil {
			return err
		}

		now := clock.Now().UTC()
		token := Token{
			ID:             tokenID(run.ID, read.CapturedRead.ID, request.OutputPath, request.Unit, sourceValueRaw, valueCanonical),
			RunID:          run.ID,
			TenantID:       run.TenantID,
			ValueCanonical: valueCanonical,
			Unit:           request.Unit,
			Source: TokenSource{
				CapturedReadID: read.CapturedRead.ID,
				SourcePath:     read.CapturedRead.SourcePath,
				SourceSelector: read.CapturedRead.Selector,
				OutputPath:     request.OutputPath,
				Origin:         read.CapturedRead.Origin,
			},
			CreatedAt: now,
		}

		entries, err := auditSink.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}

		deterministic, err := canonicalizeDeterministicPayload(tokenMintPayload{
			TokenID:        token.ID,
			CapturedReadID: read.CapturedRead.ID,
			SourcePath:     read.CapturedRead.SourcePath,
			SourceSelector: read.CapturedRead.Selector,
			OutputPath:     request.OutputPath,
			Origin:         read.CapturedRead.Origin,
			ReaderName:     read.CapturedRead.ReaderName,
			ReaderVersion:  read.CapturedRead.ReaderVersion,
			ReaderOptions:  read.CapturedRead.ReaderOptions,
			Locator:        read.CapturedRead.Locator,
			SourceSHA256:   read.CapturedRead.SourceSHA256,
			OutputSHA256:   read.CapturedRead.OutputSHA256,
			SourceValueRaw: sourceValueRaw,
			ValueCanonical: valueCanonical,
			Unit:           request.Unit,
		})
		if err != nil {
			return err
		}

		prevHash := genesisHash(run.TenantID)
		seq := uint64(1)
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			prevHash = append([]byte(nil), last.Hash...)
			seq = last.Seq + 1
		}

		hashInput := append([]byte(nil), deterministic...)
		entry := AuditEntry{
			Seq:      seq,
			RunID:    run.ID,
			TenantID: run.TenantID,
			Kind:     EntryKindTokenMint,
			At:       now,
			PrevHash: prevHash,
			Hash:     auditHash(prevHash, seq, run.ID, run.TenantID, EntryKindTokenMint, hashInput),
			Payload: PayloadEnvelope{
				Deterministic:      deterministic,
				HashInputCanonical: hashInput,
			},
		}

		if err := store.PutToken(ctx, tx, token); err != nil {
			return err
		}

		appended, err := auditSink.AppendAuditEntry(ctx, tx, entry)
		if err != nil {
			return err
		}

		result = ReadAndMintResult{
			ReadResult: read,
			Token:      token,
			AuditEntry: appended,
		}

		return nil
	}); err != nil {
		return ReadAndMintResult{}, err
	}

	return result, nil
}

type tokenMintPayload struct {
	TokenID        string          `json:"token_id"`
	CapturedReadID string          `json:"captured_read_id"`
	SourcePath     string          `json:"source_path"`
	SourceSelector SourceSelector  `json:"source_selector"`
	OutputPath     string          `json:"output_path"`
	Origin         string          `json:"origin,omitempty"`
	ReaderName     string          `json:"reader_name"`
	ReaderVersion  string          `json:"reader_version"`
	ReaderOptions  json.RawMessage `json:"reader_options,omitempty"`
	Locator        json.RawMessage `json:"locator,omitempty"`
	SourceSHA256   string          `json:"source_sha256"`
	OutputSHA256   string          `json:"output_sha256"`
	SourceValueRaw string          `json:"source_value_raw"`
	ValueCanonical string          `json:"value_canonical"`
	Unit           string          `json:"unit,omitempty"`
}

func extractMintValue(output []byte, outputPath string) (string, error) {
	if outputPath != "$.value" {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOutputPath, outputPath)
	}

	dec := json.NewDecoder(bytes.NewReader(output))
	dec.UseNumber()

	var payload struct {
		Value any `json:"value"`
	}
	if err := dec.Decode(&payload); err != nil {
		return "", fmt.Errorf("%w: %s", ErrOutputPathNotFound, outputPath)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", fmt.Errorf("%w: %s", ErrOutputPathNotFound, outputPath)
	}

	switch value := payload.Value.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrMintValueNotNumeric, outputPath)
	}
}

func validateCapturedRead(source SourceBytes, read ReadResult, run Run, request ReadAndMintRequest) error {
	if read.CapturedRead.RunID != run.ID {
		return ErrCapturedReadRunIDMismatch
	}
	if read.CapturedRead.SourcePath != request.SourcePath || read.CapturedRead.SourcePath != source.Source.Path {
		return ErrCapturedReadPathMismatch
	}
	if read.CapturedRead.Selector != request.Selector {
		return ErrCapturedReadSelectorMismatch
	}

	sourceSHA256 := sha256Hex(source.Bytes)
	if read.CapturedRead.SourceSHA256 != sourceSHA256 {
		return ErrCapturedReadSourceHashMismatch
	}

	outputSHA256 := sha256Hex(read.Output)
	if read.CapturedRead.OutputSHA256 != outputSHA256 {
		return ErrCapturedReadOutputHashMismatch
	}

	if !bytes.Equal(read.CapturedRead.Output, read.Output) {
		return ErrCapturedReadOutputMismatch
	}

	return nil
}

func normalizeSourceValueRaw(raw string) (string, error) {
	components, err := parseDecimalString(raw)
	if err != nil {
		return "", err
	}
	return components.serialize(true), nil
}

func canonicalizeNumeric(raw string) (string, error) {
	_, err := parseDecimalString(raw)
	if err != nil {
		return "", err
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
	}

	rounded := value.RoundBank(9)
	if rounded.IsZero() {
		return "0.000000000", nil
	}

	text := rounded.StringFixedBank(9)
	if strings.HasPrefix(text, "-0.") {
		return "0.000000000", nil
	}
	return text, nil
}

func canonicalizeDeterministicPayload(payload tokenMintPayload) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}

func tokenID(runID, capturedReadID, outputPath, unit, rawValue, valueCanonical string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		runID,
		capturedReadID,
		outputPath,
		unit,
		rawValue,
		valueCanonical,
	}, "\n")))
	return "tok_" + hex.EncodeToString(sum[:16])
}

func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

func auditHash(prevHash []byte, seq uint64, runID, tenantID string, kind EntryKind, hashInput []byte) []byte {
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

type decimalParts struct {
	negative bool
	digits   string
	scale    int
	zero     bool
}

func parseDecimalString(raw string) (decimalParts, error) {
	if !canonicalDecimalPattern.MatchString(raw) {
		return decimalParts{}, fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
	}

	negative := strings.HasPrefix(raw, "-")
	unsigned := strings.TrimPrefix(raw, "-")

	mantissa := unsigned
	exponent := 0
	if index := strings.IndexAny(unsigned, "eE"); index >= 0 {
		mantissa = unsigned[:index]
		parsedExponent, err := strconv.Atoi(unsigned[index+1:])
		if err != nil {
			return decimalParts{}, fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
		}
		exponent = parsedExponent
	}

	integerPart := mantissa
	fractionalPart := ""
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		integerPart = mantissa[:dot]
		fractionalPart = mantissa[dot+1:]
	}

	digits := integerPart + fractionalPart
	scale := len(fractionalPart) - exponent

	zero := true
	for _, ch := range digits {
		if ch != '0' {
			zero = false
			break
		}
	}

	return decimalParts{
		negative: negative,
		digits:   digits,
		scale:    scale,
		zero:     zero,
	}, nil
}

func (parts decimalParts) serialize(preserveTrailingZeros bool) string {
	if parts.zero {
		if parts.scale <= 0 {
			return "0"
		}
		return "0." + strings.Repeat("0", parts.scale)
	}

	sign := ""
	if parts.negative {
		sign = "-"
	}

	if parts.scale <= 0 {
		integer := trimLeadingZeros(parts.digits + strings.Repeat("0", -parts.scale))
		return sign + integer
	}

	point := len(parts.digits) - parts.scale
	var integerPart string
	var fractionalPart string
	switch {
	case point <= 0:
		integerPart = "0"
		fractionalPart = strings.Repeat("0", -point) + parts.digits
	default:
		integerPart = trimLeadingZeros(parts.digits[:point])
		fractionalPart = parts.digits[point:]
	}

	if !preserveTrailingZeros {
		fractionalPart = strings.TrimRight(fractionalPart, "0")
		if fractionalPart == "" {
			return sign + integerPart
		}
	}

	return sign + integerPart + "." + fractionalPart
}

func trimLeadingZeros(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}
	return value
}

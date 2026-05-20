package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/extractcsvrows"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/extractjsonl"
	"github.com/provenant/provenant/kernel/public/extractspreadsheet"
	"github.com/provenant/provenant/reconcile/request"
)

func Route(ctx context.Context, req request.Request, opts Options) ([]NormalizedFile, error) {
	if err := request.Validate(&req); err != nil {
		return nil, err
	}

	readers := opts.readersOrDefault()
	maxBytes := opts.maxFileBytesOrDefault()
	run := kernel.Run{ID: req.Run.ID, TenantID: req.Run.TenantID}
	files := make([]NormalizedFile, 0, len(req.Files))

	for _, file := range req.Files {
		resolvedPath, source, err := loadSource(file.Path, opts.BaseDir, maxBytes)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", file.ID, err)
		}

		effectiveSelector, err := resolveSelector(file, source.Bytes)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", file.ID, err)
		}

		reader, err := readers.readerFor(file.Format)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", file.ID, err)
		}

		result, err := reader.Read(ctx, run, source, effectiveSelector)
		if err != nil {
			return nil, fmt.Errorf("route %s: reader failed: %w", file.ID, err)
		}
		if err := validateCapturedRead(run, file, source, effectiveSelector, result); err != nil {
			return nil, fmt.Errorf("route %s: %w", file.ID, err)
		}

		rows, err := normalizeRows(file, effectiveSelector, result.Output)
		if err != nil {
			return nil, fmt.Errorf("route %s: %w", file.ID, err)
		}

		normalized := NormalizedFile{
			FileID:            file.ID,
			Purpose:           file.Purpose,
			Format:            file.Format,
			Reader:            file.Reader,
			ReaderVersion:     result.CapturedRead.ReaderVersion,
			SourcePath:        resolvedPath,
			SourceHash:        source.Source.SHA256,
			DeclaredSelector:  file.Selector,
			EffectiveSelector: effectiveSelector,
			CapturedRead:      result.CapturedRead,
			Rows:              rows,
		}

		outputHash, err := normalizedOutputHash(normalized)
		if err != nil {
			return nil, fmt.Errorf("route %s: output hash: %w", file.ID, err)
		}
		normalized.OutputHash = outputHash
		files = append(files, normalized)
	}

	return files, nil
}

func loadSource(path, baseDir string, maxBytes int64) (string, kernel.SourceBytes, error) {
	resolvedPath, err := resolvePath(path, baseDir)
	if err != nil {
		return "", kernel.SourceBytes{}, err
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return "", kernel.SourceBytes{}, err
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil {
		if !info.Mode().IsRegular() {
			return "", kernel.SourceBytes{}, fmt.Errorf("input_invalid: %s is not a regular file", path)
		}
		if info.Size() > maxBytes {
			return "", kernel.SourceBytes{}, fmt.Errorf("input_too_large: %s exceeds %d bytes", path, maxBytes)
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", kernel.SourceBytes{}, err
	}
	if int64(len(data)) > maxBytes {
		return "", kernel.SourceBytes{}, fmt.Errorf("input_too_large: %s exceeds %d bytes", path, maxBytes)
	}
	checksum := sha256.Sum256(data)
	return resolvedPath, kernel.SourceBytes{
		Source: kernel.Source{
			Path:   resolvedPath,
			SHA256: hex.EncodeToString(checksum[:]),
		},
		Bytes: data,
	}, nil
}

func resolvePath(path, baseDir string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path_invalid: absolute path rejected")
	}
	if baseDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		baseDir = cwd
	}
	baseDir, err := canonicalPath(baseDir)
	if err != nil {
		return "", err
	}
	resolvedPath := filepath.Clean(filepath.Join(baseDir, path))
	rel, err := filepath.Rel(baseDir, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("path_invalid: base_dir resolution failed")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path_invalid: path escapes base_dir")
	}
	canonicalTarget, err := filepath.EvalSymlinks(resolvedPath)
	if err != nil {
		return "", err
	}
	canonicalTarget = filepath.Clean(canonicalTarget)
	rel, err = filepath.Rel(baseDir, canonicalTarget)
	if err != nil {
		return "", fmt.Errorf("path_invalid: base_dir resolution failed")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path_invalid: path escapes base_dir")
	}
	return canonicalTarget, nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func validateCapturedRead(run kernel.Run, file request.FileConfig, source kernel.SourceBytes, selector kernel.SourceSelector, result kernel.ReadResult) error {
	read := result.CapturedRead
	if read.RunID != run.ID {
		return fmt.Errorf("captured_read_invalid: run_id")
	}
	if read.SourcePath != source.Source.Path {
		return fmt.Errorf("captured_read_invalid: source_path")
	}
	if read.Selector != selector {
		return fmt.Errorf("captured_read_invalid: selector")
	}
	if read.SourceSHA256 != source.Source.SHA256 || read.SourceSHA256 != sha256Hex(source.Bytes) {
		return fmt.Errorf("captured_read_invalid: source_hash")
	}
	if read.OutputSHA256 != sha256Hex(result.Output) {
		return fmt.Errorf("captured_read_invalid: output_hash")
	}
	if !bytes.Equal(read.Output, result.Output) {
		return fmt.Errorf("captured_read_invalid: output")
	}
	if read.ReaderName != file.Reader {
		return fmt.Errorf("captured_read_invalid: reader_name")
	}
	return nil
}

func normalizedOutputHash(file NormalizedFile) (string, error) {
	payload := struct {
		FileID            string                 `json:"file_id"`
		Format            string                 `json:"format"`
		Reader            string                 `json:"reader"`
		SourceHash        string                 `json:"source_hash"`
		DeclaredSelector  request.SelectorConfig `json:"declared_selector"`
		EffectiveSelector kernel.SourceSelector  `json:"effective_selector"`
		Rows              []NormalizedRow        `json:"rows"`
	}{
		FileID:            file.FileID,
		Format:            file.Format,
		Reader:            file.Reader,
		SourceHash:        file.SourceHash,
		DeclaredSelector:  file.DeclaredSelector,
		EffectiveSelector: file.EffectiveSelector,
		Rows:              file.Rows,
	}
	raw, err := jsonMarshal(payload)
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (o Options) readersOrDefault() Readers {
	readers := o.Readers
	if readers.CSV == nil {
		readers.CSV = extractcsvrows.Reader{}
	}
	if readers.Spreadsheet == nil {
		readers.Spreadsheet = extractspreadsheet.Reader{}
	}
	if readers.JSON == nil {
		readers.JSON = extractjson.Reader{}
	}
	if readers.JSONL == nil {
		readers.JSONL = extractjsonl.Reader{}
	}
	return readers
}

func (o Options) maxFileBytesOrDefault() int64 {
	if o.MaxFileBytes > 0 {
		return o.MaxFileBytes
	}
	return defaultMaxFileBytes
}

func (r Readers) readerFor(format string) (kernel.SourceReader, error) {
	switch format {
	case request.FormatCSV:
		return r.CSV, nil
	case request.FormatXLSX:
		return r.Spreadsheet, nil
	case request.FormatJSON:
		return r.JSON, nil
	case request.FormatJSONL:
		return r.JSONL, nil
	default:
		return nil, fmt.Errorf("reader_unsupported: %s", format)
	}
}

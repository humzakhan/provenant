package router

import (
	"encoding/json"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/reconcile/request"
)

const defaultMaxFileBytes int64 = 8 << 20

type Options struct {
	BaseDir      string
	MaxFileBytes int64
	Readers      Readers
}

type Readers struct {
	CSV         kernel.SourceReader
	Spreadsheet kernel.SourceReader
	JSON        kernel.SourceReader
	JSONL       kernel.SourceReader
}

type NormalizedFile struct {
	FileID            string                 `json:"file_id"`
	Purpose           string                 `json:"purpose"`
	Format            string                 `json:"format"`
	Reader            string                 `json:"reader"`
	ReaderVersion     string                 `json:"reader_version"`
	SourcePath        string                 `json:"source_path"`
	SourceHash        string                 `json:"source_hash"`
	DeclaredSelector  request.SelectorConfig `json:"declared_selector"`
	EffectiveSelector kernel.SourceSelector  `json:"effective_selector"`
	OutputHash        string                 `json:"output_hash"`
	CapturedRead      kernel.CapturedRead    `json:"captured_read"`
	Rows              []NormalizedRow        `json:"rows"`
}

type NormalizedRow struct {
	RowKey    string                     `json:"row_key"`
	Position  RowPosition                `json:"position"`
	RecordRaw string                     `json:"record_raw,omitempty"`
	Values    map[string]NormalizedValue `json:"values"`
}

type RowPosition struct {
	Index int    `json:"index,omitempty"`
	Line  int    `json:"line,omitempty"`
	Row   int    `json:"row,omitempty"`
	Sheet string `json:"sheet,omitempty"`
}

type NormalizedValue struct {
	LogicalField string          `json:"logical_field"`
	Role         string          `json:"role"`
	Source       string          `json:"source"`
	Raw          string          `json:"raw,omitempty"`
	RawExact     bool            `json:"raw_exact,omitempty"`
	Canonical    string          `json:"canonical"`
	Empty        bool            `json:"empty"`
	Value        json.RawMessage `json:"value,omitempty"`
}

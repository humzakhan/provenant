package bundle_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/kernel/public/extractcsvrows"
	"github.com/provenant/provenant/kernel/public/extractjson"
	"github.com/provenant/provenant/kernel/public/extractjsonl"
	"github.com/provenant/provenant/kernel/public/extractspreadsheet"
	"github.com/provenant/provenant/kernel/public/memio"
)

func TestEmitReplayPreservesStructuredReaderCapturedReadMetadata(t *testing.T) {
	t.Parallel()

	type replayCase struct {
		name           string
		runID          string
		source         kernel.SourceBytes
		reader         kernel.SourceReader
		selector       kernel.SourceSelector
		wantReaderName string
		wantOrigin     string
		wantCanonical  string
		wantOutputPart []string
	}

	tests := []replayCase{
		{
			name:           "json",
			runID:          "run_json",
			source:         sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)),
			reader:         extractjson.Reader{},
			selector:       kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"},
			wantReaderName: "extract.json",
			wantOrigin:     "fixture://invoice.json#ptr=/invoice/items/1/amount&byte=50-57",
			wantCanonical:  "56.780000000",
			wantOutputPart: []string{`"json_pointer":"/invoice/items/1/amount"`, `"raw_value":"\"56.78\""`, `"value":"56.78"`},
		},
		{
			name:           "jsonl",
			runID:          "run_jsonl_matrix",
			source:         sourceWithHash("fixture://amounts.jsonl", []byte("{\"amount\":\"1.10\",\"note\":\"a\"}\n{\"amount\":\"2.20\",\"note\":\"b\"}\n{\"amount\":\"3.30\",\"note\":\"c\"}\n{\"amount\":\"4.40\",\"note\":\"d\"}\n{\"amount\":\"5.50\",\"note\":\"e\"}")),
			reader:         extractjsonl.Reader{},
			selector:       kernel.SourceSelector{LineWindow: "1-1", Cursor: "line:2", JSONPointer: "/amount"},
			wantReaderName: "extract.jsonl",
			wantOrigin:     "fixture://amounts.jsonl#line=3-3&ptr=/amount&byte=68-74",
			wantCanonical:  "3.300000000",
			wantOutputPart: []string{`"line_window":"1-1"`, `"line":3`, `"raw_value":"\"3.30\""`, `"value":"3.30"`},
		},
		{
			name:           "csv",
			runID:          "run_csv_matrix",
			source:         sourceWithHash("fixture://amounts.csv", []byte("account,amount,note\ncash,1.10,a\nar,2.20,b\nap,3.30,c")),
			reader:         extractcsvrows.Reader{},
			selector:       kernel.SourceSelector{RowWindow: "1-1", Cursor: "row:2", ColumnProjection: "amount", HeaderRow: 1, Dialect: "excel", Encoding: "utf-8"},
			wantReaderName: "extract.csv_rows",
			wantOrigin:     "fixture://amounts.csv#row=3&col=2&header_row=1&header=amount",
			wantCanonical:  "2.200000000",
			wantOutputPart: []string{`"column_projection":"amount"`, `"header_row":1`, `"row":3`, `"value":"2.20"`},
		},
		{
			name:           "spreadsheet",
			runID:          "run_sheet_matrix",
			source:         sourceWithHash("fixture://amounts.xlsx", spreadsheetWorkbookBytes(t)),
			reader:         extractspreadsheet.Reader{},
			selector:       kernel.SourceSelector{TableName: "Amounts", RowWindow: "3-3", ColumnProjection: "B"},
			wantReaderName: "extract.spreadsheet",
			wantOrigin:     "fixture://amounts.xlsx#sheet=TrialBalance&row=3&col=2&a1=B3",
			wantCanonical:  "2.200000000",
			wantOutputPart: []string{`"sheet":"TrialBalance"`, `"table_name":"Amounts"`, `"raw_value":"=B2*2"`, `"value":"2.20"`},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := kernel.Run{ID: tt.runID, TenantID: "tenant_1", Status: kernel.RunRunning}
			binding := memio.NewBinding(tt.source)
			clock := &sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}}

			result, err := kernel.ReadAndMint(
				context.Background(),
				binding,
				binding,
				tt.reader,
				binding,
				binding,
				clock,
				run,
				kernel.ReadAndMintRequest{
					SourcePath: tt.source.Source.Path,
					Selector:   tt.selector,
					OutputPath: "$.value",
				},
			)
			if err != nil {
				t.Fatalf("ReadAndMint() error = %v", err)
			}
			for _, fragment := range tt.wantOutputPart {
				if !bytes.Contains(result.ReadResult.Output, []byte(fragment)) {
					t.Fatalf("ReadResult.Output = %s, want fragment %q", result.ReadResult.Output, fragment)
				}
			}
			if result.ReadResult.CapturedRead.ReaderName != tt.wantReaderName {
				t.Fatalf("ReaderName = %q, want %q", result.ReadResult.CapturedRead.ReaderName, tt.wantReaderName)
			}
			if result.Token.ValueCanonical != tt.wantCanonical {
				t.Fatalf("Token.ValueCanonical = %s, want %s", result.Token.ValueCanonical, tt.wantCanonical)
			}

			state := snapshotState(t, binding, run)
			bundleBytes, err := bundle.Emit(state)
			if err != nil {
				t.Fatalf("Emit() error = %v", err)
			}
			replayed, err := bundle.Replay(bundleBytes)
			if err != nil {
				t.Fatalf("Replay() error = %v", err)
			}
			if len(replayed.CapturedReads) != 1 {
				t.Fatalf("len(Replay().CapturedReads) = %d, want 1", len(replayed.CapturedReads))
			}
			if len(replayed.Tokens) != 1 {
				t.Fatalf("len(Replay().Tokens) = %d, want 1", len(replayed.Tokens))
			}

			got := replayed.CapturedReads[0]
			if got.ID != result.ReadResult.CapturedRead.ID {
				t.Fatalf("Replay().CapturedReads[0].ID = %s, want %s", got.ID, result.ReadResult.CapturedRead.ID)
			}
			if got.Selector != tt.selector {
				t.Fatalf("Replay().CapturedReads[0].Selector = %#v, want %#v", got.Selector, tt.selector)
			}
			if got.Origin != tt.wantOrigin {
				t.Fatalf("Replay().CapturedReads[0].Origin = %s, want %s", got.Origin, tt.wantOrigin)
			}
			if string(got.ReaderOptions) != string(result.ReadResult.CapturedRead.ReaderOptions) {
				t.Fatalf("Replay().CapturedReads[0].ReaderOptions = %s, want %s", got.ReaderOptions, result.ReadResult.CapturedRead.ReaderOptions)
			}
			if string(got.Locator) != string(result.ReadResult.CapturedRead.Locator) {
				t.Fatalf("Replay().CapturedReads[0].Locator = %s, want %s", got.Locator, result.ReadResult.CapturedRead.Locator)
			}
			if !bytes.Equal(got.Output, result.ReadResult.Output) {
				t.Fatalf("Replay().CapturedReads[0].Output = %s, want %s", got.Output, result.ReadResult.Output)
			}
			if replayed.Tokens[0].ValueCanonical != tt.wantCanonical {
				t.Fatalf("Replay().Tokens[0].ValueCanonical = %s, want %s", replayed.Tokens[0].ValueCanonical, tt.wantCanonical)
			}
		})
	}
}

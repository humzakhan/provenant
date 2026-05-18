package extractjson

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
)

func TestReaderReadSuccess(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}],"total":"69.12"}}`)
	source := kernel.SourceBytes{
		Source: kernel.Source{
			Path:      "fixture://invoice.json",
			MediaType: "application/json",
			SHA256:    sha256Hex(sourceBytes),
		},
		Bytes: sourceBytes,
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"json_pointer":"/invoice/items/1/amount","value":"56.78"}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
	if string(result.CapturedRead.Output) != wantOutput {
		t.Fatalf("CapturedRead.Output = %s, want %s", result.CapturedRead.Output, wantOutput)
	}
	if result.CapturedRead.ReaderName != readerName {
		t.Fatalf("ReaderName = %s, want %s", result.CapturedRead.ReaderName, readerName)
	}
	if result.CapturedRead.ReaderVersion != readerVersion {
		t.Fatalf("ReaderVersion = %s, want %s", result.CapturedRead.ReaderVersion, readerVersion)
	}
	if result.CapturedRead.SourcePath != source.Source.Path {
		t.Fatalf("SourcePath = %s, want %s", result.CapturedRead.SourcePath, source.Source.Path)
	}
	if result.CapturedRead.SourceSHA256 != source.Source.SHA256 {
		t.Fatalf("SourceSHA256 = %s, want %s", result.CapturedRead.SourceSHA256, source.Source.SHA256)
	}
	if result.CapturedRead.OutputSHA256 != sha256Hex([]byte(wantOutput)) {
		t.Fatalf("OutputSHA256 = %s, want %s", result.CapturedRead.OutputSHA256, sha256Hex([]byte(wantOutput)))
	}
	if string(result.CapturedRead.ReaderOptions) != `{"selector_mode":"json_pointer"}` {
		t.Fatalf("ReaderOptions = %s", result.CapturedRead.ReaderOptions)
	}
	if string(result.CapturedRead.Locator) != `{"json_pointer":"/invoice/items/1/amount"}` {
		t.Fatalf("Locator = %s", result.CapturedRead.Locator)
	}
	if result.CapturedRead.Origin != "fixture://invoice.json#ptr=/invoice/items/1/amount" {
		t.Fatalf("Origin = %s", result.CapturedRead.Origin)
	}
	if result.CapturedRead.ID == "" {
		t.Fatal("CapturedRead.ID is empty")
	}

	resultAgain, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"})
	if err != nil {
		t.Fatalf("Read() second call error = %v", err)
	}
	if string(resultAgain.Output) != string(result.Output) {
		t.Fatalf("second Output = %s, want %s", resultAgain.Output, result.Output)
	}
	if resultAgain.CapturedRead.ID != result.CapturedRead.ID {
		t.Fatalf("second ID = %s, want %s", resultAgain.CapturedRead.ID, result.CapturedRead.ID)
	}
}

func TestReaderReadObjectPreservesDocumentOrder(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"b":2,"a":1}}`)
	source := kernel.SourceBytes{
		Source: kernel.Source{
			Path:   "fixture://invoice.json",
			SHA256: sha256Hex(sourceBytes),
		},
		Bytes: sourceBytes,
	}

	result, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, source, kernel.SourceSelector{JSONPointer: "/invoice"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	wantOutput := `{"json_pointer":"/invoice","value":{"b":2,"a":1}}`
	if string(result.Output) != wantOutput {
		t.Fatalf("Output = %s, want %s", result.Output, wantOutput)
	}
}

func TestReaderReadFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   kernel.SourceBytes
		selector kernel.SourceSelector
		wantErr  error
	}{
		{
			name: "missing bytes",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: "abc"},
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrMissingSourceBytes,
		},
		{
			name: "missing source hash",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json"},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrMissingSourceSHA256,
		},
		{
			name: "missing selector",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":1}`))},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{},
			wantErr:  ErrMissingJSONPointer,
		},
		{
			name: "source hash mismatch",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: "deadbeef"},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrSourceHashMismatch,
		},
		{
			name: "malformed json",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":`))},
				Bytes:  []byte(`{"invoice":`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice"},
			wantErr:  ErrInvalidJSON,
		},
		{
			name: "duplicate keys",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":{"total":1,"total":2}}`))},
				Bytes:  []byte(`{"invoice":{"total":1,"total":2}}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice/total"},
			wantErr:  ErrInvalidJSON,
		},
		{
			name: "invalid pointer",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":1}`))},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "invoice"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name: "invalid pointer bare tilde escape",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":{"~":1}}`))},
				Bytes:  []byte(`{"invoice":{"~":1}}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice/~"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name: "invalid pointer unknown tilde escape",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":{"~2":1}}`))},
				Bytes:  []byte(`{"invoice":{"~2":1}}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice/~2"},
			wantErr:  ErrInvalidJSONPointer,
		},
		{
			name: "selector not found",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":1}`))},
				Bytes:  []byte(`{"invoice":1}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/missing"},
			wantErr:  ErrSelectorNotFound,
		},
		{
			name: "oversized array index",
			source: kernel.SourceBytes{
				Source: kernel.Source{Path: "fixture://invoice.json", SHA256: sha256Hex([]byte(`{"invoice":[1,2,3]}`))},
				Bytes:  []byte(`{"invoice":[1,2,3]}`),
			},
			selector: kernel.SourceSelector{JSONPointer: "/invoice/999999999999999999999999999999999999"},
			wantErr:  ErrSelectorNotFound,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := (Reader{}).Read(context.Background(), kernel.Run{ID: "run_1"}, tt.source, tt.selector)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Read() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

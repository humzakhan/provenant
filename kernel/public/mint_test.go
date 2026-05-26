package kernel_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	kernel "github.com/humzakhan/provenant/kernel/public"
	"github.com/humzakhan/provenant/kernel/public/extractjson"
	"github.com/humzakhan/provenant/kernel/public/memio"
)

func TestReadAndMintSuccess(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"total":"69.12"}}`)
	binding := memio.NewBinding(sourceWithHash("fixture://invoice.json", sourceBytes))
	clock := sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}}
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}

	result, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractjson.Reader{},
		binding,
		binding,
		&clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("ReadAndMint() error = %v", err)
	}

	if result.Token.ValueCanonical != "69.120000000" {
		t.Fatalf("Token.ValueCanonical = %s, want 69.120000000", result.Token.ValueCanonical)
	}
	if result.Token.Source.CapturedReadID != result.ReadResult.CapturedRead.ID {
		t.Fatalf("Token.Source.CapturedReadID = %s, want %s", result.Token.Source.CapturedReadID, result.ReadResult.CapturedRead.ID)
	}
	if result.Token.Source.OutputPath != "$.value" {
		t.Fatalf("Token.Source.OutputPath = %s", result.Token.Source.OutputPath)
	}
	if result.AuditEntry.Seq != 1 {
		t.Fatalf("AuditEntry.Seq = %d, want 1", result.AuditEntry.Seq)
	}
	if !equalBytes(result.AuditEntry.PrevHash, genesisHash(run.TenantID)) {
		t.Fatalf("AuditEntry.PrevHash = %x, want genesis", result.AuditEntry.PrevHash)
	}
	if !equalBytes(result.AuditEntry.Payload.Deterministic, result.AuditEntry.Payload.HashInputCanonical) {
		t.Fatalf("HashInputCanonical does not match Deterministic payload")
	}

	var payload map[string]any
	if err := json.Unmarshal(result.AuditEntry.Payload.Deterministic, &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if payload["source_sha256"] != sha256Hex(sourceBytes) {
		t.Fatalf("payload source_sha256 = %v, want %s", payload["source_sha256"], sha256Hex(sourceBytes))
	}
	if payload["output_sha256"] != result.ReadResult.CapturedRead.OutputSHA256 {
		t.Fatalf("payload output_sha256 = %v, want %s", payload["output_sha256"], result.ReadResult.CapturedRead.OutputSHA256)
	}
	if payload["source_value_raw"] != "69.12" {
		t.Fatalf("payload source_value_raw = %v, want 69.12", payload["source_value_raw"])
	}

	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		reads, err := binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(reads) != 1 {
			t.Fatalf("stored captured reads = %d, want 1", len(reads))
		}
		if reads[0].ID != result.ReadResult.CapturedRead.ID {
			t.Fatalf("stored captured read id = %s, want %s", reads[0].ID, result.ReadResult.CapturedRead.ID)
		}

		tokens, err := binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(tokens) != 1 {
			t.Fatalf("stored tokens = %d, want 1", len(tokens))
		}

		entries, err := binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			t.Fatalf("stored audit entries = %d, want 1", len(entries))
		}
		if !equalBytes(entries[0].Hash, result.AuditEntry.Hash) {
			t.Fatalf("stored audit hash = %x, want %x", entries[0].Hash, result.AuditEntry.Hash)
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}

func TestReadAndMintAuditChainDeterministic(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"items":[{"amount":"12.34"},{"amount":"56.78"}]}}`)
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}

	runSequence := func(times []time.Time) ([]kernel.ReadAndMintResult, error) {
		binding := memio.NewBinding(sourceWithHash("fixture://invoice.json", sourceBytes))
		clock := sequenceClock{times: times}

		first, err := kernel.ReadAndMint(
			context.Background(),
			binding,
			binding,
			extractjson.Reader{},
			binding,
			binding,
			&clock,
			run,
			kernel.ReadAndMintRequest{
				SourcePath: "fixture://invoice.json",
				Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/0/amount"},
				OutputPath: "$.value",
			},
		)
		if err != nil {
			return nil, err
		}

		second, err := kernel.ReadAndMint(
			context.Background(),
			binding,
			binding,
			extractjson.Reader{},
			binding,
			binding,
			&clock,
			run,
			kernel.ReadAndMintRequest{
				SourcePath: "fixture://invoice.json",
				Selector:   kernel.SourceSelector{JSONPointer: "/invoice/items/1/amount"},
				OutputPath: "$.value",
			},
		)
		if err != nil {
			return nil, err
		}

		return []kernel.ReadAndMintResult{first, second}, nil
	}

	left, err := runSequence([]time.Time{
		time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 18, 10, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("left runSequence() error = %v", err)
	}
	right, err := runSequence([]time.Time{
		time.Date(2026, 5, 19, 3, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 19, 8, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("right runSequence() error = %v", err)
	}

	if left[1].AuditEntry.Seq != 2 {
		t.Fatalf("second AuditEntry.Seq = %d, want 2", left[1].AuditEntry.Seq)
	}
	if !equalBytes(left[1].AuditEntry.PrevHash, left[0].AuditEntry.Hash) {
		t.Fatalf("second PrevHash = %x, want first hash %x", left[1].AuditEntry.PrevHash, left[0].AuditEntry.Hash)
	}

	for i := range left {
		if left[i].Token.ID != right[i].Token.ID {
			t.Fatalf("token[%d].ID mismatch: %s vs %s", i, left[i].Token.ID, right[i].Token.ID)
		}
		if !equalBytes(left[i].AuditEntry.Hash, right[i].AuditEntry.Hash) {
			t.Fatalf("audit hash[%d] mismatch: %x vs %x", i, left[i].AuditEntry.Hash, right[i].AuditEntry.Hash)
		}
		if string(left[i].AuditEntry.Payload.Deterministic) != string(right[i].AuditEntry.Payload.Deterministic) {
			t.Fatalf("deterministic payload[%d] mismatch", i)
		}
		if left[i].AuditEntry.At.Equal(right[i].AuditEntry.At) {
			t.Fatalf("audit timestamps[%d] should differ to prove wall clock is excluded", i)
		}
	}
}

func TestReadAndMintCanonicalizesHashInput(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1000}}`))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	timestamp := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)

	readerA := staticReader{
		result: readResultWithCapturedRead(
			run,
			source,
			kernel.SourceSelector{JSONPointer: "/invoice/total"},
			[]byte(`{"json_pointer":"/invoice/total","value":1000}`),
			json.RawMessage("{\n\t\"\":2,\n\t\"a\":1E30,\n\t\"𐀀\":1\n}"),
			json.RawMessage("{\"z\":[1E30,2e-3],\"\":2,\"x\":4.50,\"𐀀\":1}"),
		),
	}
	readerB := staticReader{
		result: readResultWithCapturedRead(
			run,
			source,
			kernel.SourceSelector{JSONPointer: "/invoice/total"},
			[]byte(`{"json_pointer":"/invoice/total","value":1000}`),
			json.RawMessage("{\"a\":1e+30,\"𐀀\":1,\"\":2}"),
			json.RawMessage("{\"x\":4.5,\"z\":[1e+30,0.002],\"𐀀\":1,\"\":2}"),
		),
	}

	leftBinding := memio.NewBinding(source)
	left, err := kernel.ReadAndMint(
		context.Background(),
		leftBinding,
		leftBinding,
		readerA,
		leftBinding,
		leftBinding,
		&sequenceClock{times: []time.Time{timestamp}},
		run,
		kernel.ReadAndMintRequest{
			SourcePath: source.Source.Path,
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("left ReadAndMint() error = %v", err)
	}

	rightBinding := memio.NewBinding(source)
	right, err := kernel.ReadAndMint(
		context.Background(),
		rightBinding,
		rightBinding,
		readerB,
		rightBinding,
		rightBinding,
		&sequenceClock{times: []time.Time{timestamp}},
		run,
		kernel.ReadAndMintRequest{
			SourcePath: source.Source.Path,
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
			OutputPath: "$.value",
		},
	)
	if err != nil {
		t.Fatalf("right ReadAndMint() error = %v", err)
	}

	if !equalBytes(left.AuditEntry.Payload.HashInputCanonical, right.AuditEntry.Payload.HashInputCanonical) {
		t.Fatalf("canonical hash input mismatch:\nleft:  %s\nright: %s", left.AuditEntry.Payload.HashInputCanonical, right.AuditEntry.Payload.HashInputCanonical)
	}
	if !equalBytes(left.AuditEntry.Hash, right.AuditEntry.Hash) {
		t.Fatalf("audit hash mismatch: %x vs %x", left.AuditEntry.Hash, right.AuditEntry.Hash)
	}

	if !bytes.Contains(left.AuditEntry.Payload.HashInputCanonical, []byte(`"reader_options":{"a":1e+30,"𐀀":1,"":2}`)) {
		t.Fatalf("canonical hash input missing canonical reader_options: %s", left.AuditEntry.Payload.HashInputCanonical)
	}
	if !bytes.Contains(left.AuditEntry.Payload.HashInputCanonical, []byte(`"locator":{"x":4.5,"z":[1e+30,0.002],"𐀀":1,"":2}`)) {
		t.Fatalf("canonical hash input missing canonical locator: %s", left.AuditEntry.Payload.HashInputCanonical)
	}
}

func TestMintFromCapturedReadSuccess(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":"69.12"}}`))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	read := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{JSONPointer: "/invoice/total"},
		[]byte(`{"json_pointer":"/invoice/total","value":"69.12"}`),
		json.RawMessage(`{"selector_mode":"json_pointer"}`),
		json.RawMessage(`{"json_pointer":"/invoice/total"}`),
	).CapturedRead
	binding := memio.NewBinding(source)

	result, err := kernel.MintFromCapturedRead(
		context.Background(),
		binding,
		binding,
		binding,
		&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
		run,
		kernel.MintFromCapturedReadRequest{
			CapturedRead: read,
			Source:       source.Source,
			OutputPath:   "$.value",
		},
	)
	if err != nil {
		t.Fatalf("MintFromCapturedRead() error = %v", err)
	}

	if result.Token.ValueCanonical != "69.120000000" {
		t.Fatalf("Token.ValueCanonical = %s, want 69.120000000", result.Token.ValueCanonical)
	}
	if result.Token.Source.CapturedReadID != read.ID {
		t.Fatalf("Token.Source.CapturedReadID = %s, want %s", result.Token.Source.CapturedReadID, read.ID)
	}
	if result.AuditEntry.Kind != kernel.EntryKindTokenMint {
		t.Fatalf("AuditEntry.Kind = %s, want %s", result.AuditEntry.Kind, kernel.EntryKindTokenMint)
	}

	var payload map[string]any
	if err := json.Unmarshal(result.AuditEntry.Payload.Deterministic, &payload); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v", err)
	}
	if payload["captured_read_id"] != read.ID {
		t.Fatalf("payload captured_read_id = %v, want %s", payload["captured_read_id"], read.ID)
	}
	if payload["source_value_raw"] != "69.12" {
		t.Fatalf("payload source_value_raw = %v, want 69.12", payload["source_value_raw"])
	}
	if payload["output_sha256"] != read.OutputSHA256 {
		t.Fatalf("payload output_sha256 = %v, want %s", payload["output_sha256"], read.OutputSHA256)
	}

	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		reads, err := binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(reads) != 0 {
			t.Fatalf("stored captured reads = %d, want 0", len(reads))
		}

		tokens, err := binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(tokens) != 1 {
			t.Fatalf("stored tokens = %d, want 1", len(tokens))
		}

		entries, err := binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			t.Fatalf("stored audit entries = %d, want 1", len(entries))
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}

func TestMintFromCapturedReadJSONPointerOutputPath(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://bank.csv", []byte("id,amount\na,10.00\nb,20.00\n"))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	read := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{RowWindow: "2:3", HeaderRow: 1},
		[]byte(`{"rows":[{"row":2,"cells":[{"column":1,"header":"id","value":"a"},{"column":2,"header":"amount","value":"10.00"}]},{"row":3,"cells":[{"column":1,"header":"id","value":"b"},{"column":2,"header":"amount","value":"20.00"}]}]}`),
		json.RawMessage(`{"selector_mode":"row_window"}`),
		json.RawMessage(`{"row_window":"2:3"}`),
	).CapturedRead
	binding := memio.NewBinding(source)

	result, err := kernel.MintFromCapturedRead(
		context.Background(),
		binding,
		binding,
		binding,
		&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
		run,
		kernel.MintFromCapturedReadRequest{CapturedRead: read, Source: source.Source, OutputPath: "/rows/1/cells/1/value"},
	)
	if err != nil {
		t.Fatalf("MintFromCapturedRead() error = %v", err)
	}
	if result.Token.ValueCanonical != "20.000000000" {
		t.Fatalf("Token.ValueCanonical = %s, want 20.000000000", result.Token.ValueCanonical)
	}
	if result.Token.Source.OutputPath != "/rows/1/cells/1/value" {
		t.Fatalf("Token.Source.OutputPath = %s", result.Token.Source.OutputPath)
	}
}

func TestMintFromCapturedReadJSONPointerFailures(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://bank.csv", []byte("id,amount\na,10.00\n"))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	read := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{RowWindow: "2:2", HeaderRow: 1},
		[]byte(`{"rows":[{"row":2,"cells":[{"column":1,"header":"id","value":"a"},{"column":2,"header":"amount","value":"10.00"}]}]}`),
		json.RawMessage(`{"selector_mode":"row_window"}`),
		json.RawMessage(`{"row_window":"2:2"}`),
	).CapturedRead

	tests := []struct {
		name       string
		outputPath string
		wantErr    error
	}{
		{name: "missing", outputPath: "/rows/1/cells/1/value", wantErr: kernel.ErrOutputPathNotFound},
		{name: "non numeric", outputPath: "/rows/0/cells/0/value", wantErr: kernel.ErrMintValueNotNumeric},
		{name: "unsupported", outputPath: "$.rows[0].cells[1].value", wantErr: kernel.ErrUnsupportedOutputPath},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binding := memio.NewBinding(source)
			_, err := kernel.MintFromCapturedRead(
				context.Background(),
				binding,
				binding,
				binding,
				&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
				run,
				kernel.MintFromCapturedReadRequest{CapturedRead: read, Source: source.Source, OutputPath: tt.outputPath},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MintFromCapturedRead() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestMintFromCapturedReadTokenIDIncludesOutputPath(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://bank.csv", []byte("id,amount,fee\na,10.00,10.00\n"))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	read := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{RowWindow: "2:2", HeaderRow: 1},
		[]byte(`{"rows":[{"row":2,"cells":[{"column":2,"header":"amount","value":"10.00"},{"column":3,"header":"fee","value":"10.00"}]}]}`),
		json.RawMessage(`{"selector_mode":"row_window"}`),
		json.RawMessage(`{"row_window":"2:2"}`),
	).CapturedRead

	mint := func(outputPath string) kernel.MintFromCapturedReadResult {
		binding := memio.NewBinding(source)
		result, err := kernel.MintFromCapturedRead(
			context.Background(),
			binding,
			binding,
			binding,
			&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
			run,
			kernel.MintFromCapturedReadRequest{CapturedRead: read, Source: source.Source, OutputPath: outputPath},
		)
		if err != nil {
			t.Fatalf("MintFromCapturedRead(%s) error = %v", outputPath, err)
		}
		return result
	}

	amount := mint("/rows/0/cells/0/value")
	fee := mint("/rows/0/cells/1/value")
	if amount.Token.ID == fee.Token.ID {
		t.Fatalf("Token.ID should differ by output path: %s", amount.Token.ID)
	}
}

func TestMintFromCapturedReadAuditDeterministic(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1000}}`))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	read := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{JSONPointer: "/invoice/total"},
		[]byte(`{"json_pointer":"/invoice/total","value":1000}`),
		json.RawMessage("{\n\t\"\":2,\n\t\"a\":1E30,\n\t\"𐀀\":1\n}"),
		json.RawMessage(`{"z":[1E30,2e-3],"":2,"x":4.50,"𐀀":1}`),
	).CapturedRead

	runMint := func(at time.Time) (kernel.MintFromCapturedReadResult, error) {
		binding := memio.NewBinding(source)
		return kernel.MintFromCapturedRead(
			context.Background(),
			binding,
			binding,
			binding,
			&sequenceClock{times: []time.Time{at}},
			run,
			kernel.MintFromCapturedReadRequest{
				CapturedRead: read,
				Source:       source.Source,
				OutputPath:   "$.value",
			},
		)
	}

	left, err := runMint(time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("left MintFromCapturedRead() error = %v", err)
	}
	right, err := runMint(time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("right MintFromCapturedRead() error = %v", err)
	}

	if left.Token.ID != right.Token.ID {
		t.Fatalf("Token.ID mismatch: %s vs %s", left.Token.ID, right.Token.ID)
	}
	if !equalBytes(left.AuditEntry.Hash, right.AuditEntry.Hash) {
		t.Fatalf("audit hash mismatch: %x vs %x", left.AuditEntry.Hash, right.AuditEntry.Hash)
	}
	if string(left.AuditEntry.Payload.Deterministic) != string(right.AuditEntry.Payload.Deterministic) {
		t.Fatalf("deterministic payload mismatch")
	}
	if left.AuditEntry.At.Equal(right.AuditEntry.At) {
		t.Fatalf("audit timestamps should differ to prove wall clock is excluded")
	}
}

func TestMintFromCapturedReadValidatesCapturedRead(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1000}}`))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	base := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{JSONPointer: "/invoice/total"},
		[]byte(`{"json_pointer":"/invoice/total","value":1000}`),
		json.RawMessage(`{"selector_mode":"json_pointer"}`),
		json.RawMessage(`{"json_pointer":"/invoice/total"}`),
	).CapturedRead

	tests := []struct {
		name       string
		mutateRead func(read kernel.CapturedRead) kernel.CapturedRead
		mutateReq  func(request kernel.MintFromCapturedReadRequest) kernel.MintFromCapturedReadRequest
		wantErr    error
	}{
		{
			name: "run id mismatch",
			mutateRead: func(read kernel.CapturedRead) kernel.CapturedRead {
				read.RunID = "run_other"
				return read
			},
			wantErr: kernel.ErrCapturedReadRunIDMismatch,
		},
		{
			name: "source path mismatch",
			mutateReq: func(request kernel.MintFromCapturedReadRequest) kernel.MintFromCapturedReadRequest {
				request.Source.Path = "fixture://other.json"
				return request
			},
			wantErr: kernel.ErrCapturedReadPathMismatch,
		},
		{
			name: "source sha mismatch",
			mutateReq: func(request kernel.MintFromCapturedReadRequest) kernel.MintFromCapturedReadRequest {
				request.Source.SHA256 = "deadbeef"
				return request
			},
			wantErr: kernel.ErrCapturedReadSourceHashMismatch,
		},
		{
			name: "output sha mismatch",
			mutateRead: func(read kernel.CapturedRead) kernel.CapturedRead {
				read.OutputSHA256 = "deadbeef"
				return read
			},
			wantErr: kernel.ErrCapturedReadOutputHashMismatch,
		},
		{
			name: "tampered output mismatches hash",
			mutateRead: func(read kernel.CapturedRead) kernel.CapturedRead {
				read.Output = []byte(`{"json_pointer":"/invoice/total","value":999}`)
				return read
			},
			wantErr: kernel.ErrCapturedReadOutputHashMismatch,
		},
		{
			name: "tampered value fails numeric validation",
			mutateRead: func(read kernel.CapturedRead) kernel.CapturedRead {
				read.Output = []byte(`{"json_pointer":"/invoice/total","value":"12,34"}`)
				read.OutputSHA256 = sha256Hex(read.Output)
				return read
			},
			wantErr: kernel.ErrMintValueNotNumeric,
		},
		{
			name: "path not found",
			mutateRead: func(read kernel.CapturedRead) kernel.CapturedRead {
				read.Output = []byte(`{"json_pointer":"/invoice/total","amount":1000}`)
				read.OutputSHA256 = sha256Hex(read.Output)
				return read
			},
			wantErr: kernel.ErrMintValueNotNumeric,
		},
		{
			name: "unsupported path",
			mutateReq: func(request kernel.MintFromCapturedReadRequest) kernel.MintFromCapturedReadRequest {
				request.OutputPath = "$.amount"
				return request
			},
			wantErr: kernel.ErrUnsupportedOutputPath,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			read := cloneCapturedRead(base)
			if tt.mutateRead != nil {
				read = tt.mutateRead(read)
			}
			request := kernel.MintFromCapturedReadRequest{
				CapturedRead: read,
				Source:       source.Source,
				OutputPath:   "$.value",
			}
			if tt.mutateReq != nil {
				request = tt.mutateReq(request)
			}

			binding := memio.NewBinding(source)
			_, err := kernel.MintFromCapturedRead(
				context.Background(),
				binding,
				binding,
				binding,
				&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
				run,
				request,
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("MintFromCapturedRead() error = %v, want %v", err, tt.wantErr)
			}

			assertNoStoredState(t, binding, run)
		})
	}
}

func TestReadAndMintValidatesCapturedRead(t *testing.T) {
	t.Parallel()

	source := sourceWithHash("fixture://invoice.json", []byte(`{"invoice":{"total":1000}}`))
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	base := readResultWithCapturedRead(
		run,
		source,
		kernel.SourceSelector{JSONPointer: "/invoice/total"},
		[]byte(`{"json_pointer":"/invoice/total","value":1000}`),
		json.RawMessage(`{"selector_mode":"json_pointer"}`),
		json.RawMessage(`{"json_pointer":"/invoice/total"}`),
	)

	tests := []struct {
		name    string
		mutate  func(result kernel.ReadResult) kernel.ReadResult
		wantErr error
	}{
		{
			name: "run id mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.RunID = "run_other"
				return result
			},
			wantErr: kernel.ErrCapturedReadRunIDMismatch,
		},
		{
			name: "source path mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.SourcePath = "fixture://other.json"
				return result
			},
			wantErr: kernel.ErrCapturedReadPathMismatch,
		},
		{
			name: "selector mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.Selector = kernel.SourceSelector{JSONPointer: "/invoice/other"}
				return result
			},
			wantErr: kernel.ErrCapturedReadSelectorMismatch,
		},
		{
			name: "source sha mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.SourceSHA256 = "deadbeef"
				return result
			},
			wantErr: kernel.ErrCapturedReadSourceHashMismatch,
		},
		{
			name: "output sha mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.OutputSHA256 = "deadbeef"
				return result
			},
			wantErr: kernel.ErrCapturedReadOutputHashMismatch,
		},
		{
			name: "output bytes mismatch",
			mutate: func(result kernel.ReadResult) kernel.ReadResult {
				result.CapturedRead.Output = []byte(`{"json_pointer":"/invoice/total","value":999}`)
				return result
			},
			wantErr: kernel.ErrCapturedReadOutputMismatch,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binding := memio.NewBinding(source)
			reader := staticReader{result: tt.mutate(cloneReadResult(base))}

			_, err := kernel.ReadAndMint(
				context.Background(),
				binding,
				binding,
				reader,
				binding,
				binding,
				&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
				run,
				kernel.ReadAndMintRequest{
					SourcePath: source.Source.Path,
					Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
					OutputPath: "$.value",
				},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadAndMint() error = %v, want %v", err, tt.wantErr)
			}

			assertNoStoredState(t, binding, run)
		})
	}
}

func TestReadAndMintNormalizesSourceValueRaw(t *testing.T) {
	t.Parallel()

	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}
	tests := []struct {
		name          string
		sourceBytes   []byte
		wantRaw       string
		wantCanonical string
	}{
		{
			name:          "exponent form",
			sourceBytes:   []byte(`{"invoice":{"total":1e3}}`),
			wantRaw:       "1000",
			wantCanonical: "1000.000000000",
		},
		{
			name:          "plain integer",
			sourceBytes:   []byte(`{"invoice":{"total":1000}}`),
			wantRaw:       "1000",
			wantCanonical: "1000.000000000",
		},
		{
			name:          "negative zero",
			sourceBytes:   []byte(`{"invoice":{"total":-0.0}}`),
			wantRaw:       "0.0",
			wantCanonical: "0.000000000",
		},
		{
			name:          "positive zero",
			sourceBytes:   []byte(`{"invoice":{"total":0.0}}`),
			wantRaw:       "0.0",
			wantCanonical: "0.000000000",
		},
		{
			name:          "nonzero trailing zeros",
			sourceBytes:   []byte(`{"invoice":{"total":1.2300}}`),
			wantRaw:       "1.2300",
			wantCanonical: "1.230000000",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := sourceWithHash("fixture://invoice.json", tt.sourceBytes)
			binding := memio.NewBinding(source)

			result, err := kernel.ReadAndMint(
				context.Background(),
				binding,
				binding,
				extractjson.Reader{},
				binding,
				binding,
				&sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}},
				run,
				kernel.ReadAndMintRequest{
					SourcePath: source.Source.Path,
					Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
					OutputPath: "$.value",
				},
			)
			if err != nil {
				t.Fatalf("ReadAndMint() error = %v", err)
			}

			if result.Token.ValueCanonical != tt.wantCanonical {
				t.Fatalf("Token.ValueCanonical = %s, want %s", result.Token.ValueCanonical, tt.wantCanonical)
			}

			var payload map[string]any
			if err := json.Unmarshal(result.AuditEntry.Payload.Deterministic, &payload); err != nil {
				t.Fatalf("json.Unmarshal(payload) error = %v", err)
			}
			if payload["source_value_raw"] != tt.wantRaw {
				t.Fatalf("payload source_value_raw = %v, want %s", payload["source_value_raw"], tt.wantRaw)
			}
		})
	}
}

func TestReadAndMintFailsClosedOnInvalidNumericValue(t *testing.T) {
	t.Parallel()

	sourceBytes := []byte(`{"invoice":{"total":"12,34"}}`)
	binding := memio.NewBinding(sourceWithHash("fixture://invoice.json", sourceBytes))
	clock := sequenceClock{times: []time.Time{time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)}}
	run := kernel.Run{ID: "run_1", TenantID: "tenant_1", Status: kernel.RunRunning}

	_, err := kernel.ReadAndMint(
		context.Background(),
		binding,
		binding,
		extractjson.Reader{},
		binding,
		binding,
		&clock,
		run,
		kernel.ReadAndMintRequest{
			SourcePath: "fixture://invoice.json",
			Selector:   kernel.SourceSelector{JSONPointer: "/invoice/total"},
			OutputPath: "$.value",
		},
	)
	if !errors.Is(err, kernel.ErrMintValueNotNumeric) {
		t.Fatalf("ReadAndMint() error = %v, want %v", err, kernel.ErrMintValueNotNumeric)
	}

	assertNoStoredState(t, binding, run)
}

func sourceWithHash(path string, sourceBytes []byte) kernel.SourceBytes {
	return kernel.SourceBytes{
		Source: kernel.Source{
			Path:      path,
			MediaType: "application/json",
			SHA256:    sha256Hex(sourceBytes),
		},
		Bytes: sourceBytes,
	}
}

func genesisHash(tenantID string) []byte {
	sum := sha256.Sum256([]byte("provenant-genesis:" + tenantID))
	return sum[:]
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertNoStoredState(t *testing.T, binding *memio.Binding, run kernel.Run) {
	t.Helper()

	if err := binding.WithTx(context.Background(), func(ctx context.Context, tx kernel.Tx) error {
		reads, err := binding.CapturedReads(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(reads) != 0 {
			t.Fatalf("stored captured reads = %d, want 0", len(reads))
		}

		tokens, err := binding.Tokens(ctx, tx, run.ID)
		if err != nil {
			return err
		}
		if len(tokens) != 0 {
			t.Fatalf("stored tokens = %d, want 0", len(tokens))
		}

		entries, err := binding.AuditEntries(ctx, tx, run.TenantID, run.ID)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			t.Fatalf("stored audit entries = %d, want 0", len(entries))
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTx() error = %v", err)
	}
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

type sequenceClock struct {
	times []time.Time
	index int
}

func (c *sequenceClock) Now() time.Time {
	if len(c.times) == 0 {
		return time.Time{}
	}
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	current := c.times[c.index]
	c.index++
	return current
}

type staticReader struct {
	result kernel.ReadResult
	err    error
}

func (r staticReader) Read(context.Context, kernel.Run, kernel.SourceBytes, kernel.SourceSelector) (kernel.ReadResult, error) {
	if r.err != nil {
		return kernel.ReadResult{}, r.err
	}
	return cloneReadResult(r.result), nil
}

func readResultWithCapturedRead(
	run kernel.Run,
	source kernel.SourceBytes,
	selector kernel.SourceSelector,
	output []byte,
	readerOptions json.RawMessage,
	locator json.RawMessage,
) kernel.ReadResult {
	outputHash := sha256Hex(output)
	readIDHash := sha256Sum([]byte(run.ID + source.Source.Path + selector.JSONPointer))
	return kernel.ReadResult{
		Output: append([]byte(nil), output...),
		CapturedRead: kernel.CapturedRead{
			ID:            "read_" + hex.EncodeToString(readIDHash[:16]),
			RunID:         run.ID,
			SourcePath:    source.Source.Path,
			Origin:        source.Source.Path + "#ptr=" + selector.JSONPointer,
			Selector:      selector,
			ReaderName:    "extract.json",
			ReaderVersion: "v1",
			ReaderOptions: append([]byte(nil), readerOptions...),
			Locator:       append([]byte(nil), locator...),
			SourceSHA256:  sha256Hex(source.Bytes),
			OutputSHA256:  outputHash,
			Output:        append([]byte(nil), output...),
		},
	}
}

func cloneReadResult(result kernel.ReadResult) kernel.ReadResult {
	cloned := result
	cloned.Output = append([]byte(nil), result.Output...)
	cloned.CapturedRead = cloneCapturedRead(result.CapturedRead)
	return cloned
}

func cloneCapturedRead(read kernel.CapturedRead) kernel.CapturedRead {
	cloned := read
	cloned.ReaderOptions = append([]byte(nil), read.ReaderOptions...)
	cloned.Locator = append([]byte(nil), read.Locator...)
	cloned.Output = append([]byte(nil), read.Output...)
	return cloned
}

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

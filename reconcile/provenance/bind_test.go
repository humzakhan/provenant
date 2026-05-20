package provenance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/bundle"
	"github.com/provenant/provenant/reconcile/engine"
	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
)

func TestBindAttachesMatchedAndMismatchEvidence(t *testing.T) {
	req := testRequest()
	files := testFiles()
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{
		{
			ID:      "res_matched",
			Outcome: engine.OutcomeMatched,
			Key:     []engine.KeyValue{{Field: "id", Value: "a"}},
			Left:    rowRef("left", "row:1"),
			Right:   rowRef("right", "row:1"),
		},
		{
			ID:          "res_mismatch",
			Outcome:     engine.OutcomeMismatch,
			Key:         []engine.KeyValue{{Field: "id", Value: "b"}},
			Left:        rowRef("left", "row:2"),
			Right:       rowRef("right", "row:2"),
			Differences: []engine.Difference{{Field: "amount", Left: "20.00", Right: "21.00"}},
			Reason:      "amount mismatch",
		},
	}}

	bound, err := Bind(context.Background(), req, files, set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if got, want := len(bound.Evidence), 2; got != want {
		t.Fatalf("evidence count = %d, want %d", got, want)
	}
	if got, want := len(bound.Evidence[0].RowRefs), 2; got != want {
		t.Fatalf("matched row refs = %d, want %d", got, want)
	}
	if got, want := len(bound.Evidence[0].ValueRefs), 4; got != want {
		t.Fatalf("matched value refs = %d, want %d", got, want)
	}
	if got, want := len(bound.Evidence[1].ValueRefs), 4; got != want {
		t.Fatalf("mismatch value refs = %d, want %d", got, want)
	}
	if bound.Evidence[1].ValueRefs[3].CapturedReadID == "" || bound.Evidence[1].ValueRefs[3].Selector == "" {
		t.Fatalf("mismatch evidence missing captured-read selector: %#v", bound.Evidence[1].ValueRefs[3])
	}
}

func TestBindMissingSideAttachesPresentSideOnly(t *testing.T) {
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_missing_left",
		Outcome: engine.OutcomeMissingLeft,
		Key:     []engine.KeyValue{{Field: "id", Value: "a"}},
		Right:   rowRef("right", "row:1"),
	}}}

	bound, err := Bind(context.Background(), testRequest(), testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if got, want := len(bound.Evidence[0].RowRefs), 1; got != want {
		t.Fatalf("row refs = %d, want %d", got, want)
	}
	if got := bound.Evidence[0].RowRefs[0].Side; got != "right" {
		t.Fatalf("side = %q, want right", got)
	}
	for _, ref := range bound.Evidence[0].ValueRefs {
		if ref.Side != "right" {
			t.Fatalf("value ref side = %q, want right", ref.Side)
		}
	}
}

func TestBindGroupedRefs(t *testing.T) {
	req := testRequest()
	req.Reconcile.Mode = request.ModeOneToMany
	req.Reconcile.Group = &request.GroupConfig{LeftKey: []string{"id"}, RightKey: []string{"id"}}
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeOneToMany, Results: []engine.Result{{
		ID:           "res_group",
		Outcome:      engine.OutcomeMatched,
		Key:          []engine.KeyValue{{Field: "id", Value: "a"}},
		Left:         rowRef("left", "row:1"),
		Right:        rowRef("right", "row:1"),
		GroupRowKeys: []engine.RowRef{*rowRef("right", "row:1"), *rowRef("right", "row:2")},
	}}}

	bound, err := Bind(context.Background(), req, testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if got, want := len(bound.Evidence[0].RowRefs), 4; got != want {
		t.Fatalf("row refs = %d, want %d: %#v", got, want, bound.Evidence[0].RowRefs)
	}
	foundGrouped := false
	for _, ref := range bound.Evidence[0].RowRefs {
		if ref.Side == "right_group" && ref.RowKey == "row:2" {
			foundGrouped = true
		}
	}
	if !foundGrouped {
		t.Fatalf("missing grouped row ref: %#v", bound.Evidence[0].RowRefs)
	}
}

func TestBindExcludesUnreferencedNormalizedFiles(t *testing.T) {
	files := append(testFiles(), testFile("extra", "cr-extra", []router.NormalizedRow{
		testRow("row:1", "z", "99.00"),
	}))
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_matched",
		Outcome: engine.OutcomeMatched,
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	bound, err := Bind(context.Background(), testRequest(), files, set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if got, want := len(bound.CapturedReads), 2; got != want {
		t.Fatalf("captured reads = %d, want %d: %#v", got, want, bound.CapturedReads)
	}
	for _, read := range bound.CapturedReads {
		if read.FileID == "extra" {
			t.Fatalf("unreferenced file exported as captured read: %#v", bound.CapturedReads)
		}
	}
	for _, evidence := range bound.Evidence {
		for _, ref := range evidence.RowRefs {
			if ref.FileID == "extra" {
				t.Fatalf("unreferenced file exported as row evidence: %#v", evidence.RowRefs)
			}
		}
		for _, ref := range evidence.ValueRefs {
			if ref.FileID == "extra" {
				t.Fatalf("unreferenced file exported as value evidence: %#v", evidence.ValueRefs)
			}
		}
	}
}

func TestBindRejectsResultRefsOutsideSideScope(t *testing.T) {
	files := append(testFiles(), testFile("extra", "cr-extra", []router.NormalizedRow{
		testRow("row:1", "z", "99.00"),
	}))
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_out_of_scope",
		Outcome: engine.OutcomeMatched,
		Left:    rowRef("extra", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	_, err := Bind(context.Background(), testRequest(), files, set, Options{})
	if err == nil || !strings.Contains(err.Error(), "left row ref file \"extra\" outside reconcile scope \"left\"") {
		t.Fatalf("Bind error = %v, want left scope failure", err)
	}
}

func TestBindRejectsGroupRefsOutsideReconcileScope(t *testing.T) {
	files := append(testFiles(), testFile("extra", "cr-extra", []router.NormalizedRow{
		testRow("row:1", "z", "99.00"),
	}))
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeOneToMany, Results: []engine.Result{{
		ID:           "res_group_out_of_scope",
		Outcome:      engine.OutcomeMatched,
		Left:         rowRef("left", "row:1"),
		Right:        rowRef("right", "row:1"),
		GroupRowKeys: []engine.RowRef{*rowRef("extra", "row:1")},
	}}}

	_, err := Bind(context.Background(), testRequest(), files, set, Options{})
	if err == nil || !strings.Contains(err.Error(), "group row ref file \"extra\" outside reconcile scope \"left\"/\"right\"") {
		t.Fatalf("Bind error = %v, want group scope failure", err)
	}
}

func TestBindMissingProvenanceFailsClosed(t *testing.T) {
	files := testFiles()
	delete(files[0].Rows[0].Values, "amount")
	set := &engine.ResultSet{RunID: "run-1", Results: []engine.Result{{
		ID:      "res_missing_provenance",
		Outcome: engine.OutcomeMatched,
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	_, err := Bind(context.Background(), testRequest(), files, set, Options{})
	if err == nil || !strings.Contains(err.Error(), "missing logical field \"amount\" provenance") {
		t.Fatalf("Bind error = %v, want missing logical field provenance", err)
	}
}

func TestBindMintsAmountTokenFromCapturedReadAndVerifiesBundle(t *testing.T) {
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_matched",
		Outcome: engine.OutcomeMatched,
		Key:     []engine.KeyValue{{Field: "id", Value: "a"}},
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	bound, err := Bind(context.Background(), testRequest(), testMintableFiles("10.00", "10.00"), set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if got, want := len(bound.Tokens), 2; got != want {
		t.Fatalf("tokens = %d, want %d: %#v", got, want, bound.Tokens)
	}
	if got, want := len(bound.AuditEntries), 2; got != want {
		t.Fatalf("audit entries = %d, want %d: %#v", got, want, bound.AuditEntries)
	}
	if bound.Bundle == nil || len(bound.Bundle.Bytes) == 0 || bound.Bundle.SHA256 == "" {
		t.Fatalf("bundle missing: %#v", bound.Bundle)
	}
	if got, want := bound.Bundle.SHA256, sha256HexForTest(bound.Bundle.Bytes); got != want {
		t.Fatalf("bundle hash = %s, want %s", got, want)
	}
	if err := bundle.Verify(bound.Bundle.Bytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}

	amountTokenRefs := 0
	for _, ref := range bound.Evidence[0].ValueRefs {
		if ref.Role == request.RoleAmount {
			amountTokenRefs++
			if ref.TokenID == "" {
				t.Fatalf("amount ref missing token id: %#v", ref)
			}
		} else if ref.TokenID != "" {
			t.Fatalf("non-amount ref received token id: %#v", ref)
		}
	}
	if amountTokenRefs != 2 {
		t.Fatalf("amount token refs = %d, want 2", amountTokenRefs)
	}
}

func TestBindMintsAmountTokensFromRoutedCSVOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/left.csv", []byte("ID,Description,Amount,Currency\na,coffee,10.00,USD\nb,tea,20.00,USD\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(left) error = %v", err)
	}
	if err := os.WriteFile(dir+"/right.csv", []byte("ID,Description,Amount,Currency\na,coffee,10.00,USD\nb,tea,20.00,USD\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(right) error = %v", err)
	}

	req := csvRouteRequest()
	files, err := router.Route(context.Background(), req, router.Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	results, err := engine.Reconcile(context.Background(), req, files)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	bound, err := Bind(context.Background(), req, files, results, Options{})
	if err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if err := bundle.Verify(bound.Bundle.Bytes); err != nil {
		t.Fatalf("bundle.Verify() error = %v", err)
	}

	amountRefs := 0
	for _, evidence := range bound.Evidence {
		for _, ref := range evidence.ValueRefs {
			if ref.Role != request.RoleAmount {
				if ref.TokenID != "" || ref.OutputPath != "" {
					t.Fatalf("non-amount ref received mint evidence: %#v", ref)
				}
				continue
			}
			amountRefs++
			if ref.TokenID == "" {
				t.Fatalf("amount ref missing token id: %#v", ref)
			}
			if ref.OutputPath != "/rows/0/cells/1/value" && ref.OutputPath != "/rows/1/cells/1/value" {
				t.Fatalf("amount ref output path = %q", ref.OutputPath)
			}
		}
	}
	if amountRefs != 4 {
		t.Fatalf("amount refs = %d, want 4", amountRefs)
	}
	if got, want := len(bound.Tokens), 4; got != want {
		t.Fatalf("tokens = %d, want %d: %#v", got, want, bound.Tokens)
	}
}

func TestBindBundleHashStableAcrossRepeatedBind(t *testing.T) {
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_stable",
		Outcome: engine.OutcomeMatched,
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	first, err := Bind(context.Background(), testRequest(), testMintableFiles("10.00", "10.00"), set, Options{})
	if err != nil {
		t.Fatalf("first Bind returned error: %v", err)
	}
	second, err := Bind(context.Background(), testRequest(), testMintableFiles("10.00", "10.00"), set, Options{})
	if err != nil {
		t.Fatalf("second Bind returned error: %v", err)
	}
	if first.Bundle == nil || second.Bundle == nil {
		t.Fatalf("bundle missing: first=%#v second=%#v", first.Bundle, second.Bundle)
	}
	if first.Bundle.SHA256 != second.Bundle.SHA256 || !bytes.Equal(first.Bundle.Bytes, second.Bundle.Bytes) {
		t.Fatalf("bundle changed across repeated bind:\nfirst  %s\nsecond %s", first.Bundle.SHA256, second.Bundle.SHA256)
	}
}

func TestBindMintEvidenceTamperFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]router.NormalizedFile)
	}{
		{
			name: "tampered output hash",
			mutate: func(files []router.NormalizedFile) {
				files[0].CapturedRead.OutputSHA256 = strings.Repeat("0", 64)
			},
		},
		{
			name: "missing output hash",
			mutate: func(files []router.NormalizedFile) {
				files[0].CapturedRead.OutputSHA256 = ""
			},
		},
		{
			name: "captured value mismatch",
			mutate: func(files []router.NormalizedFile) {
				output := json.RawMessage(`{"value":"99.00"}`)
				files[0].CapturedRead.Output = output
				files[0].CapturedRead.OutputSHA256 = sha256HexForTest(output)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := testMintableFiles("10.00", "10.00")
			tt.mutate(files)
			set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
				ID:      "res_tamper",
				Outcome: engine.OutcomeMatched,
				Left:    rowRef("left", "row:1"),
				Right:   rowRef("right", "row:1"),
			}}}

			_, err := Bind(context.Background(), testRequest(), files, set, Options{})
			if err == nil || (!strings.Contains(err.Error(), "mint amount token") && !strings.Contains(err.Error(), "captured read value mismatch") && !strings.Contains(err.Error(), "cannot map captured read output path")) {
				t.Fatalf("Bind error = %v, want fail-closed mint evidence failure", err)
			}
		})
	}
}

func TestBindNonMintableFieldsRemainCapturedReadOnly(t *testing.T) {
	req := testRequest()
	req.Reconcile.Mode = request.ModeExistence
	req.Reconcile.Compare = nil
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_non_mintable",
		Outcome: engine.OutcomeMatched,
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	bound, err := Bind(context.Background(), req, testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if len(bound.Tokens) != 0 || len(bound.AuditEntries) != 0 || bound.Bundle != nil {
		t.Fatalf("non-mintable bind produced minted evidence: tokens=%#v audits=%#v bundle=%#v", bound.Tokens, bound.AuditEntries, bound.Bundle)
	}
	for _, ref := range bound.Evidence[0].ValueRefs {
		if ref.CapturedReadID == "" || ref.TokenID != "" {
			t.Fatalf("non-mintable ref should keep captured read only: %#v", ref)
		}
	}
}

func TestBindPreservesResultOutcomesReasonsAndIDs(t *testing.T) {
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_immutable",
		Outcome: engine.OutcomeMismatch,
		Key:     []engine.KeyValue{{Field: "id", Value: "a"}},
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
		Reason:  "original reason",
	}}}
	want := copyResultSet(*set)

	bound, err := Bind(context.Background(), testRequest(), testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("Bind returned error: %v", err)
	}
	if !reflect.DeepEqual(bound.Results, want) {
		t.Fatalf("bound results changed Packet 02 output:\n got %#v\nwant %#v", bound.Results, want)
	}
	if !reflect.DeepEqual(*set, want) {
		t.Fatalf("input results mutated:\n got %#v\nwant %#v", *set, want)
	}
}

func TestBindOutputDeterministic(t *testing.T) {
	set := &engine.ResultSet{RunID: "run-1", Mode: request.ModeExactMatch, Results: []engine.Result{{
		ID:      "res_deterministic",
		Outcome: engine.OutcomeMatched,
		Key:     []engine.KeyValue{{Field: "id", Value: "a"}},
		Left:    rowRef("left", "row:1"),
		Right:   rowRef("right", "row:1"),
	}}}

	first, err := Bind(context.Background(), testRequest(), testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("first Bind returned error: %v", err)
	}
	second, err := Bind(context.Background(), testRequest(), testFiles(), set, Options{})
	if err != nil {
		t.Fatalf("second Bind returned error: %v", err)
	}
	firstRaw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstRaw) != string(secondRaw) {
		t.Fatalf("Bind output changed across repeated calls:\nfirst  %s\nsecond %s", firstRaw, secondRaw)
	}
}

func testRequest() request.Request {
	return request.Request{
		Run: request.RunConfig{ID: "run-1", TenantID: "tenant-1"},
		Files: []request.FileConfig{
			{ID: "left", Purpose: "left"},
			{ID: "right", Purpose: "right"},
		},
		Reconcile: request.ReconcileConfig{
			Mode:    request.ModeExactMatch,
			Left:    "left",
			Right:   "right",
			MatchOn: []string{"id"},
			Compare: []string{"amount"},
		},
	}
}

func csvRouteRequest() request.Request {
	return request.Request{
		Run: request.RunConfig{ID: "run-1", TenantID: "tenant-1"},
		Files: []request.FileConfig{
			csvRouteFile("left", "left.csv"),
			csvRouteFile("right", "right.csv"),
		},
		Reconcile: request.ReconcileConfig{
			Mode:    request.ModeExactMatch,
			Left:    "left",
			Right:   "right",
			MatchOn: []string{"id"},
			Compare: []string{"amount", "currency"},
		},
	}
}

func csvRouteFile(id, path string) request.FileConfig {
	return request.FileConfig{
		ID:      id,
		Purpose: id,
		Format:  request.FormatCSV,
		Path:    path,
		Reader:  request.ReaderExtractCSVRows,
		Selector: request.SelectorConfig{
			RowWindow: "2-3",
			HeaderRow: 1,
			Encoding:  "utf-8",
			Dialect:   "excel",
		},
		Columns: request.Columns{
			"id":       {Source: "ID", Role: request.RoleKey, Required: true},
			"amount":   {Source: "Amount", Role: request.RoleAmount, Required: true, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
			"currency": {Source: "Currency", Role: request.RoleCurrency, Required: true},
		},
	}
}

func testFiles() []router.NormalizedFile {
	return []router.NormalizedFile{
		testFile("left", "cr-left", []router.NormalizedRow{
			testRow("row:1", "a", "10.00"),
			testRow("row:2", "b", "20.00"),
		}),
		testFile("right", "cr-right", []router.NormalizedRow{
			testRow("row:1", "a", "10.00"),
			testRow("row:2", "b", "21.00"),
		}),
	}
}

func testMintableFiles(leftAmount, rightAmount string) []router.NormalizedFile {
	files := []router.NormalizedFile{
		testFile("left", "cr-left", []router.NormalizedRow{testRow("row:1", "a", leftAmount)}),
		testFile("right", "cr-right", []router.NormalizedRow{testRow("row:1", "a", rightAmount)}),
	}
	for i := range files {
		amount := leftAmount
		if files[i].FileID == "right" {
			amount = rightAmount
		}
		output := json.RawMessage(`{"value":"` + amount + `"}`)
		files[i].CapturedRead.Output = output
		files[i].CapturedRead.OutputSHA256 = sha256HexForTest(output)
	}
	return files
}

func testFile(fileID, capturedReadID string, rows []router.NormalizedRow) router.NormalizedFile {
	output := testRowsOutput(rows)
	return router.NormalizedFile{
		FileID:     fileID,
		SourcePath: fileID + ".csv",
		SourceHash: fileID + "-sha256",
		CapturedRead: kernel.CapturedRead{
			ID:            capturedReadID,
			RunID:         "run-1",
			SourcePath:    fileID + ".csv",
			SourceSHA256:  fileID + "-sha256",
			Selector:      kernel.SourceSelector{RowWindow: "2:"},
			ReaderName:    request.ReaderExtractCSVRows,
			ReaderVersion: "test",
			Output:        output,
			OutputSHA256:  sha256HexForTest(output),
		},
		Rows: rows,
	}
}

func testRowsOutput(rows []router.NormalizedRow) json.RawMessage {
	type cell struct {
		Column int    `json:"column"`
		Header string `json:"header"`
		Value  string `json:"value"`
	}
	type row struct {
		Row         int    `json:"row"`
		WindowIndex int    `json:"window_index"`
		Cells       []cell `json:"cells"`
	}
	payload := struct {
		Rows []row `json:"rows"`
	}{Rows: make([]row, 0, len(rows))}
	for index, normalized := range rows {
		payload.Rows = append(payload.Rows, row{
			Row:         normalized.Position.Row,
			WindowIndex: index,
			Cells: []cell{
				{Column: 1, Header: "ID", Value: normalized.Values["id"].Canonical},
				{Column: 2, Header: "Amount", Value: normalized.Values["amount"].Canonical},
			},
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		panic(err)
	}
	return canonical
}

func testRow(rowKey, id, amount string) router.NormalizedRow {
	rowNumber := 1
	if suffix, ok := strings.CutPrefix(rowKey, "row:"); ok {
		if parsed, err := strconv.Atoi(suffix); err == nil {
			rowNumber = parsed
		}
	}
	return router.NormalizedRow{
		RowKey:   rowKey,
		Position: router.RowPosition{Row: rowNumber},
		Values: map[string]router.NormalizedValue{
			"id":     testValue("id", request.RoleKey, "ID", id),
			"amount": testValue("amount", request.RoleAmount, "Amount", amount),
		},
	}
}

func testValue(field, role, source, value string) router.NormalizedValue {
	return router.NormalizedValue{
		LogicalField: field,
		Role:         role,
		Source:       source,
		Raw:          value,
		RawExact:     true,
		Canonical:    value,
		Value:        json.RawMessage(`"` + value + `"`),
	}
}

func rowRef(fileID, rowKey string) *engine.RowRef {
	return &engine.RowRef{FileID: fileID, RowKey: rowKey, Position: router.RowPosition{Row: 1}}
}

func sha256HexForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

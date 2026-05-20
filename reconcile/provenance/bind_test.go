package provenance

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	kernel "github.com/provenant/provenant/kernel/public"
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

func testFile(fileID, capturedReadID string, rows []router.NormalizedRow) router.NormalizedFile {
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
		},
		Rows: rows,
	}
}

func testRow(rowKey, id, amount string) router.NormalizedRow {
	return router.NormalizedRow{
		RowKey:   rowKey,
		Position: router.RowPosition{Row: 1},
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

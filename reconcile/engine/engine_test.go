package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/provenant/provenant/reconcile/request"
	"github.com/provenant/provenant/reconcile/router"
)

func TestReconcileExactOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
	}{
		{
			name:     "matched",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMatched},
		},
		{
			name:     "mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "11.00", "USD")},
			outcomes: []Outcome{OutcomeMismatch},
		},
		{
			name:     "missing left and right",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMissingRight, OutcomeMissingLeft},
		},
		{
			name:     "duplicate key",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeDuplicateKey, OutcomeDuplicateKey},
		},
		{
			name:     "invalid missing logical field",
			left:     []router.NormalizedRow{missingFieldRow("l1", "transaction_id")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMissingLeft, OutcomeInvalidRecord},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), exactRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
		})
	}
}

func TestReconcileExactMismatchDifferences(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), exactRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "11.00", "EUR")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(set.Results) != 1 || set.Results[0].Outcome != OutcomeMismatch {
		t.Fatalf("results = %#v", set.Results)
	}
	want := []Difference{{Field: "amount", Left: "10.00", Right: "11.00"}, {Field: "currency", Left: "USD", Right: "EUR"}}
	if !reflect.DeepEqual(set.Results[0].Differences, want) {
		t.Fatalf("differences = %#v, want %#v", set.Results[0].Differences, want)
	}
}

func TestReconcileExactRightDuplicateReferencesRightSide(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), exactRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "10.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, result := range set.Results {
		if result.Outcome != OutcomeDuplicateKey {
			continue
		}
		if result.Left != nil || result.Right == nil || result.Right.FileID != "right" {
			t.Fatalf("duplicate result side = %#v", result)
		}
	}
}

func TestReconcileExactInvalidEmptyMatchField(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), exactRequest(), files(
		[]router.NormalizedRow{emptyKeyRow("l1")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, result := range set.Results {
		if result.Outcome == OutcomeInvalidRecord && strings.Contains(result.Reason, "empty match field") {
			return
		}
	}
	t.Fatalf("results = %#v, want invalid_record with empty match field reason", set.Results)
}

func TestReconcileExactShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := exactRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A2", "20.00", "USD"), row("l3", "A3", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A2", "21.00", "USD"), row("r4", "A4", "40.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r4", "A4", "40.00", "USD"), row("r2", "A2", "21.00", "USD"), row("r1", "A1", "10.00", "USD")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l3", "A3", "30.00", "USD"), row("l1", "A1", "10.00", "USD"), row("l2", "A2", "20.00", "USD")}},
	})
	if err != nil {
		t.Fatalf("Reconcile() shuffled error = %v", err)
	}
	if !reflect.DeepEqual(resultIDs(ordered.Results), resultIDs(shuffled.Results)) {
		t.Fatalf("result IDs changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
	if !reflect.DeepEqual(outcomes(ordered.Results), outcomes(shuffled.Results)) {
		t.Fatalf("outcomes changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
}

func TestReconcileAmountToleranceOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
	}{
		{
			name:     "within tolerance matched",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.01", "USD")},
			outcomes: []Outcome{OutcomeMatched},
		},
		{
			name:     "outside tolerance mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.02", "USD")},
			outcomes: []Outcome{OutcomeMismatch},
		},
		{
			name:     "currency mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "EUR")},
			outcomes: []Outcome{OutcomeMismatch},
		},
		{
			name:     "malformed amount invalid record",
			left:     []router.NormalizedRow{row("l1", "A1", "not-money", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMissingLeft},
		},
		{
			name:     "missing side",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMissingRight, OutcomeMissingLeft},
		},
		{
			name:     "duplicate key",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "10.01", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeDuplicateKey, OutcomeDuplicateKey},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), amountToleranceRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
		})
	}
}

func TestReconcileAmountToleranceDeltaDifference(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), amountToleranceRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.02", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	want := []Difference{{Field: "amount", Left: "10.00", Right: "10.02 (delta 0.02)"}}
	if len(set.Results) != 1 || !reflect.DeepEqual(set.Results[0].Differences, want) {
		t.Fatalf("differences = %#v, want %#v", set.Results, want)
	}
	if got, want := set.Results[0].Reason, "amount delta 0.02 exceeds tolerance 0.01"; got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

func TestReconcileAmountToleranceMalformedAmountReason(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), amountToleranceRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "bad", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, result := range set.Results {
		if result.Outcome == OutcomeInvalidRecord && strings.Contains(result.Reason, "malformed amount field") {
			return
		}
	}
	t.Fatalf("results = %#v, want invalid_record with malformed amount reason", set.Results)
}

func TestReconcileAmountToleranceShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := amountToleranceRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A2", "20.00", "USD"), row("l3", "A3", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.01", "USD"), row("r2", "A2", "20.02", "USD"), row("r4", "A4", "40.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r4", "A4", "40.00", "USD"), row("r2", "A2", "20.02", "USD"), row("r1", "A1", "10.01", "USD")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l3", "A3", "30.00", "USD"), row("l1", "A1", "10.00", "USD"), row("l2", "A2", "20.00", "USD")}},
	})
	if err != nil {
		t.Fatalf("Reconcile() shuffled error = %v", err)
	}
	if !reflect.DeepEqual(resultIDs(ordered.Results), resultIDs(shuffled.Results)) {
		t.Fatalf("result IDs changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
	if !reflect.DeepEqual(outcomes(ordered.Results), outcomes(shuffled.Results)) {
		t.Fatalf("outcomes changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
}

func TestReconcilePreflightReferences(t *testing.T) {
	t.Parallel()

	_, err := Reconcile(context.Background(), exactRequest(), []router.NormalizedFile{{FileID: "left"}})
	if err == nil || !strings.Contains(err.Error(), "right file") {
		t.Fatalf("Reconcile() error = %v, want missing right file", err)
	}
}

func exactRequest() request.Request {
	return request.Request{
		Run: request.RunConfig{ID: "run_1", TenantID: "tenant_1"},
		Reconcile: request.ReconcileConfig{
			Mode:    request.ModeExactMatch,
			Left:    "left",
			Right:   "right",
			MatchOn: []string{"transaction_id"},
			Compare: []string{"amount", "currency"},
		},
	}
}

func amountToleranceRequest() request.Request {
	req := exactRequest()
	req.Files = []request.FileConfig{
		{ID: "left", Columns: columns()},
		{ID: "right", Columns: columns()},
	}
	req.Reconcile.Mode = request.ModeAmountTolerance
	req.Reconcile.Tolerance = &request.ToleranceConfig{Amount: "0.01"}
	return req
}

func columns() request.Columns {
	return request.Columns{
		"transaction_id": {Role: request.RoleKey},
		"amount":         {Role: request.RoleAmount, Numeric: &request.NumericConfig{Scale: 2, CurrencyField: "currency"}},
		"currency":       {Role: request.RoleCurrency},
	}
}

func files(left, right []router.NormalizedRow) []router.NormalizedFile {
	return []router.NormalizedFile{{FileID: "left", Rows: left}, {FileID: "right", Rows: right}}
}

func row(rowKey, transactionID, amount, currency string) router.NormalizedRow {
	return router.NormalizedRow{
		RowKey: rowKey,
		Values: map[string]router.NormalizedValue{
			"transaction_id": value("transaction_id", request.RoleKey, transactionID),
			"amount":         value("amount", request.RoleAmount, amount),
			"currency":       value("currency", request.RoleCurrency, currency),
		},
	}
}

func missingFieldRow(rowKey, field string) router.NormalizedRow {
	row := row(rowKey, "A1", "10.00", "USD")
	delete(row.Values, field)
	return row
}

func emptyKeyRow(rowKey string) router.NormalizedRow {
	row := row(rowKey, "", "10.00", "USD")
	row.Values["transaction_id"] = router.NormalizedValue{LogicalField: "transaction_id", Role: request.RoleKey, Empty: true}
	return row
}

func value(field, role, canonical string) router.NormalizedValue {
	return router.NormalizedValue{LogicalField: field, Role: role, Canonical: canonical, Empty: canonical == ""}
}

func outcomes(results []Result) []Outcome {
	got := make([]Outcome, len(results))
	for i, result := range results {
		got[i] = result.Outcome
	}
	return got
}

func resultIDs(results []Result) []string {
	got := make([]string, len(results))
	for i, result := range results {
		got[i] = result.ID
	}
	return got
}

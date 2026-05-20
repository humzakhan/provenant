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

func TestReconcileExactInvalidRecordSideAttribution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		left      []router.NormalizedRow
		right     []router.NormalizedRow
		wantLeft  bool
		wantFile  string
		wantRowID string
	}{
		{
			name:      "left invalid row uses left slot",
			left:      []router.NormalizedRow{emptyKeyRow("l_bad")},
			right:     []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			wantLeft:  true,
			wantFile:  "left",
			wantRowID: "l_bad",
		},
		{
			name:      "right invalid row uses right slot",
			left:      []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:     []router.NormalizedRow{emptyKeyRow("r_bad")},
			wantFile:  "right",
			wantRowID: "r_bad",
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

			result := invalidResultForTest(t, set.Results)
			if tt.wantLeft {
				if result.Left == nil || result.Right != nil || result.Left.FileID != tt.wantFile || result.Left.RowKey != tt.wantRowID {
					t.Fatalf("invalid result side = %#v", result)
				}
				return
			}
			if result.Left != nil || result.Right == nil || result.Right.FileID != tt.wantFile || result.Right.RowKey != tt.wantRowID {
				t.Fatalf("invalid result side = %#v", result)
			}
		})
	}
}

func TestReconcileExactInvalidRecordStableOrderAndID(t *testing.T) {
	t.Parallel()

	req := exactRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{emptyKeyRow("l_bad"), row("l1", "A1", "10.00", "USD")},
		[]router.NormalizedRow{emptyKeyRow("r_bad"), row("r1", "A1", "10.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r1", "A1", "10.00", "USD"), emptyKeyRow("r_bad")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), emptyKeyRow("l_bad")}},
	})
	if err != nil {
		t.Fatalf("Reconcile() shuffled error = %v", err)
	}

	want := []resultSignature{
		{outcome: OutcomeMatched, leftRow: "l1", rightRow: "r1"},
		{outcome: OutcomeInvalidRecord, rightRow: "r_bad"},
		{outcome: OutcomeInvalidRecord, leftRow: "l_bad"},
	}
	if got := resultSignatures(ordered.Results); !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered results = %#v, want %#v; results = %#v", got, want, ordered.Results)
	}
	if !reflect.DeepEqual(resultSignatures(ordered.Results), resultSignatures(shuffled.Results)) {
		t.Fatalf("result order changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
	if !reflect.DeepEqual(resultIDs(ordered.Results), resultIDs(shuffled.Results)) {
		t.Fatalf("result IDs changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
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

func TestReconcileOneToManyOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
	}{
		{
			name:     "exact sum matched",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "USD")},
			outcomes: []Outcome{OutcomeMatched},
		},
		{
			name:     "sum mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.02", "USD")},
			outcomes: []Outcome{OutcomeMismatch},
		},
		{
			name:     "missing parent and children",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "30.00", "USD")},
			outcomes: []Outcome{OutcomeMissingRight, OutcomeMissingLeft},
		},
		{
			name:     "duplicate parent group key",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD"), row("l2", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "USD")},
			outcomes: []Outcome{OutcomeDuplicateKey, OutcomeDuplicateKey},
		},
		{
			name:     "malformed child amount invalid record",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "bad", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMissingRight},
		},
		{
			name:     "currency mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "EUR")},
			outcomes: []Outcome{OutcomeMismatch},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), oneToManyRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
		})
	}
}

func TestReconcileOneToManyMalformedAmountReason(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), oneToManyRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "bad", "USD")},
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

func TestReconcileOneToManyUsesConfiguredTolerance(t *testing.T) {
	t.Parallel()

	req := oneToManyRequest()
	req.Reconcile.Tolerance = &request.ToleranceConfig{Amount: "0.01"}
	set, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "30.01", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := outcomes(set.Results); !reflect.DeepEqual(got, []Outcome{OutcomeMatched}) {
		t.Fatalf("outcomes = %#v, want matched; results = %#v", got, set.Results)
	}
}

func TestReconcileOneToManyShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := oneToManyRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "30.00", "USD"), row("l2", "A2", "15.00", "USD"), row("l3", "A3", "40.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "USD"), row("r3", "A2", "14.99", "USD"), row("r4", "A4", "40.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r4", "A4", "40.00", "USD"), row("r2", "A1", "20.00", "USD"), row("r3", "A2", "14.99", "USD"), row("r1", "A1", "10.00", "USD")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l3", "A3", "40.00", "USD"), row("l1", "A1", "30.00", "USD"), row("l2", "A2", "15.00", "USD")}},
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
	if !reflect.DeepEqual(groupRowKeys(ordered.Results), groupRowKeys(shuffled.Results)) {
		t.Fatalf("group row keys changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
}

func TestReconcileOneToManyPreservesSortedGroupRowKeys(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), oneToManyRequest(), files(
		[]router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
		[]router.NormalizedRow{row("r2", "A1", "20.00", "USD"), row("r1", "A1", "10.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(set.Results) != 1 {
		t.Fatalf("results = %#v, want one grouped result", set.Results)
	}
	result := set.Results[0]
	if result.Right == nil || result.Right.RowKey != "r1" {
		t.Fatalf("right ref = %#v, want first sorted child r1", result.Right)
	}
	if got, want := groupRowKeys(set.Results), [][]string{{"r1", "r2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group row keys = %#v, want %#v; result = %#v", got, want, result)
	}
}

func TestReconcileOneToManyResultIDIncludesGroupMembership(t *testing.T) {
	t.Parallel()

	req := oneToManyRequest()
	first, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r2", "A1", "20.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() first error = %v", err)
	}
	changed, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "10.00", "USD"), row("r3", "A1", "20.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() changed error = %v", err)
	}
	if len(first.Results) != 1 || len(changed.Results) != 1 {
		t.Fatalf("results = %#v and %#v, want one grouped result each", first.Results, changed.Results)
	}
	if first.Results[0].Right == nil || changed.Results[0].Right == nil || first.Results[0].Right.RowKey != changed.Results[0].Right.RowKey {
		t.Fatalf("right refs = %#v and %#v, want same first child", first.Results[0].Right, changed.Results[0].Right)
	}
	if first.Results[0].ID == changed.Results[0].ID {
		t.Fatalf("result ID did not change when group membership changed: %#v vs %#v", first.Results[0], changed.Results[0])
	}
	if got, want := groupRowKeys(changed.Results), [][]string{{"r1", "r3"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed group row keys = %#v, want %#v", got, want)
	}
}

func TestReconcileManyToOneOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
	}{
		{
			name:     "exact sum matched",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
			outcomes: []Outcome{OutcomeMatched},
		},
		{
			name:     "sum mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.02", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
			outcomes: []Outcome{OutcomeMismatch},
		},
		{
			name:     "missing parent and children",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "30.00", "USD")},
			outcomes: []Outcome{OutcomeMissingRight, OutcomeMissingLeft},
		},
		{
			name:     "duplicate ambiguous parent",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "30.00", "USD"), row("r2", "A1", "30.00", "USD")},
			outcomes: []Outcome{OutcomeDuplicateKey, OutcomeDuplicateKey},
		},
		{
			name:     "malformed parent amount invalid record",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "bad", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMissingRight},
		},
		{
			name:     "currency mismatch",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "EUR")},
			right:    []router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
			outcomes: []Outcome{OutcomeMismatch},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), oneToManyRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
		})
	}
}

func TestReconcileManyToOneShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := oneToManyRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD"), row("l3", "A2", "15.01", "USD"), row("l4", "A4", "40.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "30.00", "USD"), row("r2", "A2", "15.00", "USD"), row("r3", "A3", "40.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r3", "A3", "40.00", "USD"), row("r2", "A2", "15.00", "USD"), row("r1", "A1", "30.00", "USD")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l4", "A4", "40.00", "USD"), row("l2", "A1", "20.00", "USD"), row("l3", "A2", "15.01", "USD"), row("l1", "A1", "10.00", "USD")}},
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
	if !reflect.DeepEqual(groupRowKeys(ordered.Results), groupRowKeys(shuffled.Results)) {
		t.Fatalf("group row keys changed across shuffled input: %#v vs %#v", ordered.Results, shuffled.Results)
	}
}

func TestReconcileManyToOnePreservesSortedGroupRowKeys(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), oneToManyRequest(), files(
		[]router.NormalizedRow{row("l2", "A1", "20.00", "USD"), row("l1", "A1", "10.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(set.Results) != 1 {
		t.Fatalf("results = %#v, want one grouped result", set.Results)
	}
	result := set.Results[0]
	if result.Left == nil || result.Left.RowKey != "l1" {
		t.Fatalf("left ref = %#v, want first sorted child l1", result.Left)
	}
	if got, want := groupRowKeys(set.Results), [][]string{{"l1", "l2"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group row keys = %#v, want %#v; result = %#v", got, want, result)
	}
}

func TestReconcileManyToOneResultIDIncludesGroupMembership(t *testing.T) {
	t.Parallel()

	req := oneToManyRequest()
	first, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "20.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() first error = %v", err)
	}
	changed, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l3", "A1", "20.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "30.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() changed error = %v", err)
	}
	if len(first.Results) != 1 || len(changed.Results) != 1 {
		t.Fatalf("results = %#v and %#v, want one grouped result each", first.Results, changed.Results)
	}
	if first.Results[0].Left == nil || changed.Results[0].Left == nil || first.Results[0].Left.RowKey != changed.Results[0].Left.RowKey {
		t.Fatalf("left refs = %#v and %#v, want same first child", first.Results[0].Left, changed.Results[0].Left)
	}
	if first.Results[0].ID == changed.Results[0].ID {
		t.Fatalf("result ID did not change when group membership changed: %#v vs %#v", first.Results[0], changed.Results[0])
	}
	if got, want := groupRowKeys(changed.Results), [][]string{{"l1", "l3"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changed group row keys = %#v, want %#v", got, want)
	}
}

func TestReconcileExistenceOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
	}{
		{
			name:     "exists both sides",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "11.00", "EUR")},
			outcomes: []Outcome{OutcomeMatched},
		},
		{
			name:     "missing left and right",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMissingRight, OutcomeMissingLeft},
		},
		{
			name:     "duplicate key",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A1", "11.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A1", "12.00", "USD")},
			outcomes: []Outcome{OutcomeDuplicateKey, OutcomeDuplicateKey},
		},
		{
			name:     "invalid empty key",
			left:     []router.NormalizedRow{emptyKeyRow("l1")},
			right:    []router.NormalizedRow{row("r1", "A1", "10.00", "USD")},
			outcomes: []Outcome{OutcomeMissingLeft, OutcomeInvalidRecord},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), existenceRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
			for _, result := range set.Results {
				if result.Outcome == OutcomeMismatch {
					t.Fatalf("existence result included mismatch: %#v", set.Results)
				}
			}
		})
	}
}

func TestReconcileExistenceInvalidEmptyMatchField(t *testing.T) {
	t.Parallel()

	set, err := Reconcile(context.Background(), existenceRequest(), files(
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

func TestReconcileExistenceShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := existenceRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "A2", "20.00", "USD"), row("l3", "A3", "30.00", "USD")},
		[]router.NormalizedRow{row("r1", "A1", "11.00", "EUR"), row("r2", "A2", "21.00", "USD"), row("r4", "A4", "40.00", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r4", "A4", "40.00", "USD"), row("r2", "A2", "21.00", "USD"), row("r1", "A1", "11.00", "EUR")}},
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

func TestReconcileSchemaValidationOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		left     []router.NormalizedRow
		right    []router.NormalizedRow
		outcomes []Outcome
		reason   string
	}{
		{
			name:     "valid rows",
			left:     []router.NormalizedRow{row("l1", "A1", "10.00", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "11.00", "USD")},
			outcomes: []Outcome{OutcomeMatched, OutcomeMatched},
		},
		{
			name:     "required missing",
			left:     []router.NormalizedRow{missingFieldRow("l1", "transaction_id")},
			right:    []router.NormalizedRow{row("r1", "A2", "11.00", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMatched},
			reason:   "missing required field \"transaction_id\"",
		},
		{
			name:     "required empty",
			left:     []router.NormalizedRow{emptyKeyRow("l1")},
			right:    []router.NormalizedRow{row("r1", "A2", "11.00", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMatched},
			reason:   "empty required field \"transaction_id\"",
		},
		{
			name:     "malformed numeric",
			left:     []router.NormalizedRow{row("l1", "A1", "not-money", "USD")},
			right:    []router.NormalizedRow{row("r1", "A2", "11.00", "USD")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMatched},
			reason:   "malformed numeric field \"amount\"",
		},
		{
			name:     "valid date",
			left:     []router.NormalizedRow{rowWithDate("l1", "A1", "10.00", "USD", "2026-05-19")},
			right:    []router.NormalizedRow{rowWithDate("r1", "A2", "11.00", "USD", "2026-05-20")},
			outcomes: []Outcome{OutcomeMatched, OutcomeMatched},
		},
		{
			name:     "malformed date",
			left:     []router.NormalizedRow{rowWithDate("l1", "A1", "10.00", "USD", "05/19/2026")},
			right:    []router.NormalizedRow{rowWithDate("r1", "A2", "11.00", "USD", "2026-05-20")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMatched},
			reason:   "malformed date field \"posted_date\"",
		},
		{
			name:     "empty optional date",
			left:     []router.NormalizedRow{rowWithDate("l1", "A1", "10.00", "USD", "")},
			right:    []router.NormalizedRow{rowWithDate("r1", "A2", "11.00", "USD", "2026-05-20")},
			outcomes: []Outcome{OutcomeMatched, OutcomeMatched},
		},
		{
			name:     "malformed reasons are stable",
			left:     []router.NormalizedRow{rowWithDate("l1", "A1", "not-money", "USD", "05/19/2026")},
			right:    []router.NormalizedRow{rowWithDate("r1", "A2", "11.00", "USD", "2026-05-20")},
			outcomes: []Outcome{OutcomeInvalidRecord, OutcomeMatched},
			reason:   "malformed numeric field \"amount\"; malformed date field \"posted_date\"",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			set, err := Reconcile(context.Background(), schemaValidationRequest(), files(tt.left, tt.right))
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := outcomes(set.Results); !reflect.DeepEqual(got, tt.outcomes) {
				t.Fatalf("outcomes = %#v, want %#v; results = %#v", got, tt.outcomes, set.Results)
			}
			if tt.reason != "" && set.Results[0].Reason != tt.reason {
				t.Fatalf("reason = %q, want %q; results = %#v", set.Results[0].Reason, tt.reason, set.Results)
			}
		})
	}
}

func TestReconcileSchemaValidationShuffledInputOrdering(t *testing.T) {
	t.Parallel()

	req := schemaValidationRequest()
	ordered, err := Reconcile(context.Background(), req, files(
		[]router.NormalizedRow{row("l1", "A1", "10.00", "USD"), row("l2", "", "20.00", "USD")},
		[]router.NormalizedRow{row("r1", "A2", "11.00", "USD"), row("r2", "A3", "bad", "USD")},
	))
	if err != nil {
		t.Fatalf("Reconcile() ordered error = %v", err)
	}
	shuffled, err := Reconcile(context.Background(), req, []router.NormalizedFile{
		{FileID: "right", Rows: []router.NormalizedRow{row("r2", "A3", "bad", "USD"), row("r1", "A2", "11.00", "USD")}},
		{FileID: "left", Rows: []router.NormalizedRow{row("l2", "", "20.00", "USD"), row("l1", "A1", "10.00", "USD")}},
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

func TestReconcileSchemaValidationInvalidReferences(t *testing.T) {
	t.Parallel()

	_, err := Reconcile(context.Background(), schemaValidationRequest(), []router.NormalizedFile{{FileID: "left"}})
	if err == nil || !strings.Contains(err.Error(), "right file") {
		t.Fatalf("Reconcile() error = %v, want missing right file", err)
	}

	bad := row("l1", "A1", "10.00", "USD")
	bad.Values["amount"] = router.NormalizedValue{LogicalField: "total", Role: request.RoleAmount, Canonical: "10.00"}
	set, err := Reconcile(context.Background(), schemaValidationRequest(), files([]router.NormalizedRow{bad}, []router.NormalizedRow{row("r1", "A2", "11.00", "USD")}))
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := outcomes(set.Results); !reflect.DeepEqual(got, []Outcome{OutcomeInvalidRecord, OutcomeMatched}) {
		t.Fatalf("outcomes = %#v, want invalid_record and matched; results = %#v", got, set.Results)
	}
	if !strings.Contains(set.Results[0].Reason, "invalid normalized value reference \"amount\"") {
		t.Fatalf("reason = %q, want invalid normalized value reference", set.Results[0].Reason)
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

func existenceRequest() request.Request {
	req := exactRequest()
	req.Reconcile.Mode = request.ModeExistence
	req.Reconcile.Compare = nil
	return req
}

func oneToManyRequest() request.Request {
	req := exactRequest()
	req.Files = []request.FileConfig{
		{ID: "left", Columns: columns()},
		{ID: "right", Columns: columns()},
	}
	req.Reconcile.Mode = request.ModeOneToMany
	req.Reconcile.Group = &request.GroupConfig{LeftKey: []string{"transaction_id"}, RightKey: []string{"transaction_id"}}
	return req
}

func schemaValidationRequest() request.Request {
	req := exactRequest()
	req.Files = []request.FileConfig{
		{ID: "left", Columns: schemaValidationColumns()},
		{ID: "right", Columns: schemaValidationColumns()},
	}
	req.Reconcile.Mode = request.ModeSchemaValidation
	req.Reconcile.MatchOn = nil
	req.Reconcile.Compare = nil
	return req
}

func schemaValidationColumns() request.Columns {
	columns := columns()
	columns["transaction_id"] = request.ColumnConfig{Role: request.RoleKey, Required: true}
	columns["posted_date"] = request.ColumnConfig{Role: request.RoleDate, Format: "YYYY-MM-DD"}
	return columns
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

func rowWithDate(rowKey, transactionID, amount, currency, postedDate string) router.NormalizedRow {
	row := row(rowKey, transactionID, amount, currency)
	row.Values["posted_date"] = value("posted_date", request.RoleDate, postedDate)
	return row
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

func invalidResultForTest(t *testing.T, results []Result) Result {
	t.Helper()
	for _, result := range results {
		if result.Outcome == OutcomeInvalidRecord {
			return result
		}
	}
	t.Fatalf("results = %#v, want invalid_record", results)
	return Result{}
}

type resultSignature struct {
	outcome  Outcome
	leftRow  string
	rightRow string
}

func resultSignatures(results []Result) []resultSignature {
	got := make([]resultSignature, len(results))
	for i, result := range results {
		got[i].outcome = result.Outcome
		if result.Left != nil {
			got[i].leftRow = result.Left.RowKey
		}
		if result.Right != nil {
			got[i].rightRow = result.Right.RowKey
		}
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

func groupRowKeys(results []Result) [][]string {
	got := make([][]string, len(results))
	for i, result := range results {
		got[i] = make([]string, len(result.GroupRowKeys))
		for j, ref := range result.GroupRowKeys {
			got[i][j] = ref.RowKey
		}
	}
	return got
}

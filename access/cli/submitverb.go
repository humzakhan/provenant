package cli

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gowebpki/jcs"
	kernel "github.com/provenant/provenant/kernel/public"
	"github.com/provenant/provenant/kernel/public/runfile"
)

// submitAnswerRequest is the JSON body for `submit-answer`.
//
// IMPORTANT: This struct intentionally has NO `value` or `value_canonical` field.
// DisallowUnknownFields on decoding guarantees that passing a literal number
// via a "value" field causes a hard JSON decode failure (exit 2, request_invalid).
// This is the structural ungrounded-number gate described in the spec.
type submitAnswerRequest struct {
	RunFile string `json:"run_file"`
	TokenID string `json:"token_id"`
	Label   string `json:"label"`
}

// submitAnswerAnswer is the answer portion of the response.
type submitAnswerAnswer struct {
	ValueCanonical string `json:"value_canonical"`
	Unit           string `json:"unit,omitempty"`
}

// submitAnswerResponse is written to stdout on `submit-answer` success.
type submitAnswerResponse struct {
	Token      kernel.Token      `json:"token"`
	AuditEntry kernel.AuditEntry `json:"audit_entry"`
	Answer     submitAnswerAnswer `json:"answer"`
}

func (r Runner) runSubmitAnswer(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	req, err := decodeJSONRequest[submitAnswerRequest](args, stdin)
	if err != nil {
		return err
	}
	if req.RunFile == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("run_file is required"))
	}
	if req.TokenID == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("token_id is required"))
	}
	if req.Label == "" {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("label is required"))
	}

	// Open the run partfile (replays existing state).
	run, err := runfile.Recover(req.RunFile)
	if err != nil {
		return normalizeRunfileError(err)
	}
	run.SetClock(fixedClock{t: r.now()})
	kernRun := run.RunIdentity()

	// Verify the token exists in the run.
	var tok kernel.Token
	found := false
	for _, t := range run.Tokens() {
		if t.ID == req.TokenID {
			tok = t
			found = true
			break
		}
	}
	if !found {
		_ = run.Abort()
		return newPacketError(packetErrorTokenNotFound, fmt.Errorf("token %q not found in run %s", req.TokenID, kernRun.ID))
	}

	// Build the deterministic payload.
	type answerPayload struct {
		TokenID        string `json:"token_id"`
		Label          string `json:"label"`
		ValueCanonical string `json:"value_canonical"`
		Unit           string `json:"unit,omitempty"`
	}
	ap := answerPayload{
		TokenID:        tok.ID,
		Label:          req.Label,
		ValueCanonical: tok.ValueCanonical,
		Unit:           tok.Unit,
	}
	rawDet, err := json.Marshal(ap)
	if err != nil {
		_ = run.Abort()
		return fmt.Errorf("submit-answer: marshal payload: %w", err)
	}
	det, err := jcs.Transform(rawDet)
	if err != nil {
		_ = run.Abort()
		return fmt.Errorf("submit-answer: canonicalize payload: %w", err)
	}

	// Build the audit entry for answer_submitted.
	seq := run.LastSeq() + 1
	prevHash := run.LastHash()
	hash := answerAuditHash(prevHash, seq, kernRun.ID, kernRun.TenantID, det)
	entry := kernel.AuditEntry{
		Seq:      seq,
		RunID:    kernRun.ID,
		TenantID: kernRun.TenantID,
		Kind:     kernel.EntryKindAnswerSubmitted,
		At:       r.now().UTC(),
		PrevHash: prevHash,
		Hash:     hash,
		Payload: kernel.PayloadEnvelope{
			Deterministic:      det,
			HashInputCanonical: det,
		},
	}

	// Append the entry (no token, no captured read) to the runfile.
	if err := run.AppendEntry(entry, nil, nil); err != nil {
		_ = run.Abort()
		return normalizeRunfileError(err)
	}

	if err := run.Close(); err != nil {
		return fmt.Errorf("runfile: close after submit-answer: %w", err)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(submitAnswerResponse{
		Token:      tok,
		AuditEntry: entry,
		Answer: submitAnswerAnswer{
			ValueCanonical: tok.ValueCanonical,
			Unit:           tok.Unit,
		},
	})
}

// answerAuditHash computes the hash for an answer_submitted entry.
// It mirrors the kernel's auditHash function.
func answerAuditHash(prevHash []byte, seq uint64, runID, tenantID string, det []byte) []byte {
	seqBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seqBytes, seq)

	h := sha256.New()
	h.Write(prevHash)
	h.Write(seqBytes)
	h.Write([]byte(runID))
	h.Write([]byte(tenantID))
	h.Write([]byte(kernel.EntryKindAnswerSubmitted))
	h.Write(det)
	return h.Sum(nil)
}


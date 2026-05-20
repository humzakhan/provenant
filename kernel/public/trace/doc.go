// Package trace provides the trace-back tool for the Provenant kernel.
//
// Given a finalized bundle and a token ID, Trace reconstructs the full
// contribution graph from that token down to its leaf captured reads. For each
// leaf the graph records: file path, byte/row span (selector), source SHA-256,
// canonical leaf value, and the signed numeric contribution that leaf made to
// the root token's value.
//
// # Contribution semantics
//
// Contribution is the signed amount each leaf adds to the root value. For
// additive ops (calc.add, calc.subtract, calc.sum, filter.sum_where,
// select.passthrough) contributions sum exactly to the root value, with
// negation applied for subtracted operands and zero for filtered-out inputs.
//
// For multiplicative ops the decomposition is necessarily non-additive:
//
//   - calc.multiply: when one input is dimensionless (a scaling factor) and the
//     other carries a currency unit, the currency-side leaves are each multiplied
//     by the dimensionless input's canonical value. The dimensionless input's own
//     leaves carry Contribution = "0". This models the auditor narrative that
//     "the FX rate did not add money; it scaled the EUR balance into USD."
//     There is no unique additive decomposition for multiplication, so we choose
//     the convention that maximises interpretability: the currency side carries
//     the full scaled amount and the scalar side is zero.
//
//   - calc.divide: the numerator (position 0) leaves are each divided by the
//     denominator's canonical value. The denominator (position 1) leaves carry
//     Contribution = "0". A synthetic <rounding> residual captures any
//     non-terminating-decimal loss.
//
// # Rounding residual
//
// For round.half_even and calc.divide, the sum of leaf contributions may
// differ from the root value by a small rounding residual. The graph records
// this residual as a synthetic LeafContribution with TokenID = "" and
// SourceName = "<rounding>". The invariant
//
//	sum(contributions) + sum(rounding residuals) == root value
//
// holds for every op set supported by this package.
//
// # Integrity invariant
//
// Trace asserts the contribution invariant before returning. If the invariant
// is violated (which indicates a kernel-integrity failure, not a caller error)
// it returns ErrContributionMismatch.
package trace

// Package grounding extracts numeric claims from released answer text.
//
// The PR1 extractor is intentionally narrow. It recognizes currency amounts
// and plain integer or decimal claims whose absolute value is at least 1. It
// ignores common false positives: dates, percentages and basis points,
// citation markers, list ordinals, ordinal words such as "1st", version-like
// dotted identifiers, and standalone calendar years from 1900 through 2099.
// Grouping spaces are accepted only for validated thousands groups such as
// "1 234" or "1 234 567"; malformed spacing such as "12 34" is ignored.
// Currency fractions with no leading zero, such as "$.50", are normalized.
// For MVP clarity, ambiguous single-dot euro amounts such as "€1.234" are
// ignored rather than guessed as either 1234 or 1.234.
//
// A numeric-looking value may be deliberately exempted by placing the marker
// [[unverified]] immediately after it, separated only by whitespace. Later
// guard layers can report or reject these exemptions as policy requires; this
// extractor simply omits them from the grounding set.
package grounding

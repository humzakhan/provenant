package grounding

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

const (
	defaultScale      = 9
	exemptionMarker   = "[[unverified]]"
	minPlainMagnitude = 1
	currencySymbols   = "$€£¥"
)

// NumericClaim is a numeric value asserted by answer text.
type NumericClaim struct {
	ValueCanonical string
	Raw            string
	Start          int
	End            int
}

// ExtractNumericClaims returns grounded-answer candidate numbers from answer.
// Start and End are byte offsets into answer.
func ExtractNumericClaims(answer string) []NumericClaim {
	var claims []NumericClaim
	for i := 0; i < len(answer); {
		start, end, ok := scanCandidate(answer, i)
		if !ok {
			i += runeSize(answer[i:])
			continue
		}
		raw := answer[start:end]
		if shouldIgnore(answer, start, end, raw) {
			i = end
			continue
		}
		canonical, ok := canonicalize(raw)
		if !ok {
			i = end
			continue
		}
		if !isCurrency(raw) && belowPlainMagnitude(canonical) {
			i = end
			continue
		}
		claims = append(claims, NumericClaim{
			ValueCanonical: canonical,
			Raw:            raw,
			Start:          start,
			End:            end,
		})
		i = end
	}
	return claims
}

func shouldIgnore(answer string, start, end int, raw string) bool {
	return touchesWord(answer, start, end) ||
		hasImmediateExemption(answer[end:]) ||
		isCitation(answer, start, end) ||
		isListOrdinal(answer, start, end) ||
		isOrdinal(answer, end) ||
		isPercentOrBPS(answer, end) ||
		isDatePart(answer, start, end) ||
		isTextualDatePart(answer, start, end) ||
		isVersionLike(answer, start, end) ||
		isCalendarYear(raw)
}

func scanCandidate(s string, i int) (int, int, bool) {
	r, size := utf8.DecodeRuneInString(s[i:])
	if isCurrencyRune(r) {
		j := i + size
		for j < len(s) {
			next, nextSize := utf8.DecodeRuneInString(s[j:])
			if !unicode.IsSpace(next) {
				break
			}
			j += nextSize
		}
		if j < len(s) && (s[j] == '-' || s[j] == '+') {
			j++
		}
		if !startsNumberBody(s, j) {
			return 0, 0, false
		}
		return i, scanNumberBody(s, j), true
	}
	if (r == '-' || r == '+') && i+size < len(s) && isASCIIDigit(s[i+size]) {
		return i, scanNumberBody(s, i+size), true
	}
	if isASCIIDigitByte(s[i]) {
		return i, scanNumberBody(s, i), true
	}
	return 0, 0, false
}

func scanNumberBody(s string, i int) int {
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if isASCIIDigitRune(r) {
			i += size
			continue
		}
		if isNumberSeparator(r) && i+size < len(s) && isASCIIDigit(s[i+size]) {
			i += size
			continue
		}
		return i
	}
	return i
}

func canonicalize(raw string) (string, bool) {
	cleaned := strings.TrimSpace(raw)
	isEuroCurrency := false
	if len(cleaned) > 0 {
		r, size := utf8.DecodeRuneInString(cleaned)
		if isCurrencyRune(r) {
			isEuroCurrency = r == '€'
			cleaned = cleaned[size:]
		}
	}
	cleaned = strings.TrimSpace(cleaned)
	var ok bool
	cleaned, ok = normalizeSeparators(cleaned, isEuroCurrency)
	if !ok {
		return "", false
	}
	d, err := decimal.NewFromString(cleaned)
	if err != nil {
		return "", false
	}
	text := d.RoundBank(defaultScale).StringFixedBank(defaultScale)
	if text == "-0."+strings.Repeat("0", defaultScale) {
		return "0." + strings.Repeat("0", defaultScale), true
	}
	return text, true
}

func normalizeSeparators(s string, isEuroCurrency bool) (string, bool) {
	sign := ""
	unsigned := s
	if strings.HasPrefix(unsigned, "-") || strings.HasPrefix(unsigned, "+") {
		sign = unsigned[:1]
		unsigned = unsigned[1:]
	}
	var ok bool
	unsigned, ok = stripGroupingSpaces(unsigned)
	if !ok {
		return "", false
	}
	if unsigned == "" {
		return "", false
	}
	if strings.HasPrefix(unsigned, ".") || strings.HasPrefix(unsigned, ",") {
		unsigned = "0" + unsigned
	}
	if isEuroCurrency && isAmbiguousSingleDotThousands(unsigned) {
		return "", false
	}

	lastComma := strings.LastIndex(unsigned, ",")
	lastDot := strings.LastIndex(unsigned, ".")
	switch {
	case lastComma >= 0 && lastDot >= 0:
		if lastComma > lastDot {
			integer, fraction, _ := strings.Cut(unsigned, ",")
			if !validThousands(integer, ".") || strings.ContainsAny(fraction, ".,") {
				return "", false
			}
			return sign + strings.ReplaceAll(integer, ".", "") + "." + fraction, true
		}
		integer, fraction, _ := strings.Cut(unsigned, ".")
		if !validThousands(integer, ",") || strings.ContainsAny(fraction, ".,") {
			return "", false
		}
		return sign + strings.ReplaceAll(integer, ",", "") + "." + fraction, true
	case lastComma >= 0:
		if strings.Count(unsigned, ",") > 1 {
			if !validThousands(unsigned, ",") {
				return "", false
			}
			return sign + strings.ReplaceAll(unsigned, ",", ""), true
		}
		left, right, _ := strings.Cut(unsigned, ",")
		if len(right) == 3 && validThousands(unsigned, ",") {
			return sign + left + right, true
		}
		return sign + left + "." + right, true
	case lastDot >= 0:
		if strings.Count(unsigned, ".") > 1 {
			if !validThousands(unsigned, ".") {
				return "", false
			}
			return sign + strings.ReplaceAll(unsigned, ".", ""), true
		}
		return sign + unsigned, true
	default:
		return sign + unsigned, true
	}
}

func stripGroupingSpaces(s string) (string, bool) {
	normalized := strings.Map(func(r rune) rune {
		if isGroupingSpace(r) {
			return ' '
		}
		return r
	}, s)
	if !strings.ContainsRune(normalized, ' ') {
		return normalized, true
	}
	parts := strings.Fields(normalized)
	if len(parts) < 2 || len(parts[0]) == 0 || len(parts[0]) > 3 || !allASCIIDigits(parts[0]) {
		return "", false
	}
	for _, part := range parts[1 : len(parts)-1] {
		if len(part) != 3 || !allASCIIDigits(part) {
			return "", false
		}
	}
	lastDigits := leadingDigitCount(parts[len(parts)-1])
	if lastDigits != 3 {
		return "", false
	}
	return strings.Join(parts, ""), true
}

func validThousands(s, sep string) bool {
	parts := strings.Split(s, sep)
	if len(parts) < 2 || len(parts[0]) == 0 || len(parts[0]) > 3 {
		return false
	}
	for _, part := range parts[1:] {
		if len(part) != 3 || !allASCIIDigits(part) {
			return false
		}
	}
	return allASCIIDigits(parts[0])
}

func belowPlainMagnitude(canonical string) bool {
	d, err := decimal.NewFromString(canonical)
	if err != nil {
		return true
	}
	return d.Abs().LessThan(decimal.NewFromInt(minPlainMagnitude))
}

func isCurrency(raw string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(raw))
	return isCurrencyRune(r)
}

func hasImmediateExemption(suffix string) bool {
	return strings.HasPrefix(strings.TrimLeftFunc(suffix, unicode.IsSpace), exemptionMarker)
}

func isCitation(answer string, start, end int) bool {
	return (start > 0 && end < len(answer) && answer[start-1] == '[' && answer[end] == ']') ||
		isCitationCluster(answer, start, end)
}

func isListOrdinal(answer string, start, end int) bool {
	lineStart := strings.LastIndex(answer[:start], "\n") + 1
	if strings.TrimSpace(answer[lineStart:start]) != "" {
		return false
	}
	if end >= len(answer) {
		return false
	}
	return answer[end] == '.' || answer[end] == ')'
}

func isOrdinal(answer string, end int) bool {
	if end+2 > len(answer) {
		return false
	}
	suffix := strings.ToLower(answer[end:min(end+2, len(answer))])
	return suffix == "st" || suffix == "nd" || suffix == "rd" || suffix == "th"
}

func isPercentOrBPS(answer string, end int) bool {
	suffix := strings.TrimLeftFunc(answer[end:], unicode.IsSpace)
	if strings.HasPrefix(suffix, "%") {
		return true
	}
	suffix = strings.ToLower(suffix)
	return strings.HasPrefix(suffix, "bps") || strings.HasPrefix(suffix, "bp")
}

func isDatePart(answer string, start, end int) bool {
	return adjacent(answer, start, '-') || adjacent(answer, end, '-') ||
		adjacent(answer, start, '/') || adjacent(answer, end, '/')
}

func isVersionLike(answer string, start, end int) bool {
	raw := answer[start:end]
	if isCurrency(raw) {
		return false
	}
	if strings.Count(raw, ".") > 1 {
		return true
	}
	if start > 0 {
		prev := rune(answer[start-1])
		if prev == 'v' || prev == 'V' || prev == '.' {
			return true
		}
	}
	if end < len(answer) && answer[end] == '.' {
		return end+1 < len(answer) && isASCIIDigit(answer[end+1])
	}
	word := strings.ToLower(previousWord(answer, start))
	return strings.Contains(raw, ".") &&
		(word == "version" || word == "ver" || word == "v" || word == "release")
}

func isCalendarYear(raw string) bool {
	if len(raw) != 4 {
		return false
	}
	d, err := decimal.NewFromString(raw)
	if err != nil || !d.Equal(d.Truncate(0)) {
		return false
	}
	year := d.IntPart()
	return year >= 1900 && year <= 2099
}

func isTextualDatePart(answer string, start, end int) bool {
	return isMonthName(previousWord(answer, start)) || isMonthName(nextWord(answer, end))
}

func touchesWord(answer string, start, end int) bool {
	if start > 0 {
		prev := rune(answer[start-1])
		if unicode.IsLetter(prev) || prev == '_' {
			return true
		}
	}
	if end < len(answer) {
		next := rune(answer[end])
		if unicode.IsLetter(next) || next == '_' {
			return true
		}
	}
	return false
}

func adjacent(answer string, index int, b byte) bool {
	if index > 0 && answer[index-1] == b {
		return true
	}
	return index < len(answer) && answer[index] == b
}

func previousWord(s string, end int) string {
	i := end
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if !unicode.IsSpace(r) && r != ',' {
			break
		}
		i -= size
	}
	wordEnd := i
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if !unicode.IsLetter(r) {
			break
		}
		i -= size
	}
	return s[i:wordEnd]
}

func nextWord(s string, start int) string {
	i := start
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) && r != ',' {
			break
		}
		i += size
	}
	wordStart := i
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsLetter(r) {
			break
		}
		i += size
	}
	return s[wordStart:i]
}

func isMonthName(s string) bool {
	switch strings.ToLower(s) {
	case "jan", "january", "feb", "february", "mar", "march", "apr", "april",
		"may", "jun", "june", "jul", "july", "aug", "august", "sep", "sept",
		"september", "oct", "october", "nov", "november", "dec", "december":
		return true
	default:
		return false
	}
}

func isCurrencyRune(r rune) bool {
	return strings.ContainsRune(currencySymbols, r)
}

func isNumberSeparator(r rune) bool {
	return r == ',' || r == '.' || isGroupingSpace(r)
}

func isGroupingSpace(r rune) bool {
	return r == ' ' || r == '\u00a0' || r == '\u202f' || r == '\u2009'
}

func startsNumberBody(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	if isASCIIDigit(s[i]) {
		return true
	}
	return (s[i] == '.' || s[i] == ',') && i+1 < len(s) && isASCIIDigit(s[i+1])
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isASCIIDigitByte(b byte) bool {
	return b >= '0' && b <= '9'
}

func isASCIIDigitRune(r rune) bool {
	return r >= '0' && r <= '9'
}

func allASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			return false
		}
	}
	return true
}

func leadingDigitCount(s string) int {
	count := 0
	for count < len(s) && isASCIIDigit(s[count]) {
		count++
	}
	return count
}

func isAmbiguousSingleDotThousands(s string) bool {
	if strings.Count(s, ".") != 1 || strings.Contains(s, ",") {
		return false
	}
	left, right, ok := strings.Cut(s, ".")
	return ok && len(right) == 3 && allASCIIDigits(left) && allASCIIDigits(right)
}

func isCitationCluster(answer string, start, end int) bool {
	open := strings.LastIndex(answer[:start], "[")
	if open < 0 || strings.LastIndex(answer[:start], "]") > open {
		return false
	}
	closeOffset := strings.IndexByte(answer[end:], ']')
	if closeOffset < 0 {
		return false
	}
	if nextOpen := strings.IndexByte(answer[end:], '['); nextOpen >= 0 && nextOpen < closeOffset {
		return false
	}
	cluster := answer[open+1 : end+closeOffset]
	if cluster == "" {
		return false
	}
	for _, r := range cluster {
		if unicode.IsSpace(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case ',', ';', '-', '\u2013':
			continue
		default:
			return false
		}
	}
	return true
}

func runeSize(s string) int {
	_, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return 1
	}
	return size
}

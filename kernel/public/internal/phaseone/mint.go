package phaseone

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrUnsupportedOutputPath = errors.New("phaseone: unsupported output path")
	ErrOutputPathNotFound    = errors.New("phaseone: output path not found")
	ErrMintValueNotNumeric   = errors.New("phaseone: mint value is not numeric")
)

var canonicalDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

type MintEvidence struct {
	SourceValueRaw string
	ValueCanonical string
	TokenID        string
}

func ReconstructMintEvidence(runID, capturedReadID string, output []byte, outputPath, unit string) (MintEvidence, error) {
	rawValue, err := ExtractMintValue(output, outputPath)
	if err != nil {
		return MintEvidence{}, err
	}
	sourceValueRaw, err := NormalizeSourceValueRaw(rawValue)
	if err != nil {
		return MintEvidence{}, err
	}
	valueCanonical, err := CanonicalizeNumeric(rawValue)
	if err != nil {
		return MintEvidence{}, err
	}
	return MintEvidence{
		SourceValueRaw: sourceValueRaw,
		ValueCanonical: valueCanonical,
		TokenID:        TokenID(runID, capturedReadID, outputPath, unit, sourceValueRaw, valueCanonical),
	}, nil
}

func ExtractMintValue(output []byte, outputPath string) (string, error) {
	if outputPath != "$.value" {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOutputPath, outputPath)
	}

	dec := json.NewDecoder(bytes.NewReader(output))
	dec.UseNumber()

	var payload struct {
		Value any `json:"value"`
	}
	if err := dec.Decode(&payload); err != nil {
		return "", fmt.Errorf("%w: %s", ErrOutputPathNotFound, outputPath)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", fmt.Errorf("%w: %s", ErrOutputPathNotFound, outputPath)
	}

	switch value := payload.Value.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrMintValueNotNumeric, outputPath)
	}
}

func NormalizeSourceValueRaw(raw string) (string, error) {
	components, err := parseDecimalString(raw)
	if err != nil {
		return "", err
	}
	return components.serialize(true), nil
}

func CanonicalizeNumeric(raw string) (string, error) {
	_, err := parseDecimalString(raw)
	if err != nil {
		return "", err
	}

	value, err := decimal.NewFromString(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
	}

	rounded := value.RoundBank(9)
	if rounded.IsZero() {
		return "0.000000000", nil
	}

	text := rounded.StringFixedBank(9)
	if strings.HasPrefix(text, "-0.") {
		return "0.000000000", nil
	}
	return text, nil
}

func TokenID(runID, capturedReadID, outputPath, unit, rawValue, valueCanonical string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		runID,
		capturedReadID,
		outputPath,
		unit,
		rawValue,
		valueCanonical,
	}, "\n")))
	return "tok_" + hex.EncodeToString(sum[:16])
}

type decimalParts struct {
	negative bool
	digits   string
	scale    int
	zero     bool
}

func parseDecimalString(raw string) (decimalParts, error) {
	if !canonicalDecimalPattern.MatchString(raw) {
		return decimalParts{}, fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
	}

	negative := strings.HasPrefix(raw, "-")
	unsigned := strings.TrimPrefix(raw, "-")

	mantissa := unsigned
	exponent := 0
	if index := strings.IndexAny(unsigned, "eE"); index >= 0 {
		mantissa = unsigned[:index]
		parsedExponent, err := strconv.Atoi(unsigned[index+1:])
		if err != nil {
			return decimalParts{}, fmt.Errorf("%w: %s", ErrMintValueNotNumeric, raw)
		}
		exponent = parsedExponent
	}

	integerPart := mantissa
	fractionalPart := ""
	if dot := strings.IndexByte(mantissa, '.'); dot >= 0 {
		integerPart = mantissa[:dot]
		fractionalPart = mantissa[dot+1:]
	}

	digits := integerPart + fractionalPart
	scale := len(fractionalPart) - exponent

	zero := true
	for _, ch := range digits {
		if ch != '0' {
			zero = false
			break
		}
	}

	return decimalParts{
		negative: negative,
		digits:   digits,
		scale:    scale,
		zero:     zero,
	}, nil
}

func (parts decimalParts) serialize(preserveTrailingZeros bool) string {
	if parts.zero {
		if parts.scale <= 0 {
			return "0"
		}
		return "0." + strings.Repeat("0", parts.scale)
	}

	sign := ""
	if parts.negative {
		sign = "-"
	}

	if parts.scale <= 0 {
		integer := trimLeadingZeros(parts.digits + strings.Repeat("0", -parts.scale))
		return sign + integer
	}

	point := len(parts.digits) - parts.scale
	var integerPart string
	var fractionalPart string
	switch {
	case point <= 0:
		integerPart = "0"
		fractionalPart = strings.Repeat("0", -point) + parts.digits
	default:
		integerPart = trimLeadingZeros(parts.digits[:point])
		fractionalPart = parts.digits[point:]
	}

	if !preserveTrailingZeros {
		fractionalPart = strings.TrimRight(fractionalPart, "0")
		if fractionalPart == "" {
			return sign + integerPart
		}
	}

	return sign + integerPart + "." + fractionalPart
}

func trimLeadingZeros(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}
	return value
}

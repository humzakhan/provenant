package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	auditpack "github.com/provenant/provenant/audit/pack"
)

// packResponse is written to stdout on `pack` success.
type packResponse struct {
	PackPath   string `json:"pack_path"`
	PackSHA256 string `json:"pack_sha256"`
}

// runPack implements `provenant pack --bundle <path> --out <path>`.
//
// It loads the bundle, verifies it, discovers any sidecar artifacts adjacent
// to the bundle file (report.md, report.json, mismatches.csv), and writes the
// JCS-canonical audit.pack.json to --out.
func (r Runner) runPack(_ context.Context, args []string, _ io.Reader, stdout io.Writer) error {
	flags, err := parsePackFlags(args)
	if err != nil {
		return userError(err)
	}

	bundleBytes, err := os.ReadFile(flags.bundle)
	if err != nil {
		return newPacketError(packetErrorInputNotFound, fmt.Errorf("read bundle %s: %w", flags.bundle, err))
	}

	// Discover sidecars from the directory containing the bundle file.
	opts := auditpack.Options{
		BundleDir: filepath.Dir(flags.bundle),
	}

	packBytes, err := auditpack.Build(bundleBytes, opts)
	if err != nil {
		return newPacketError(packetErrorEvidenceFailed, fmt.Errorf("build pack: %w", err))
	}

	if err := ensureOutputWritable(flags.out, flags.overwrite); err != nil {
		return err
	}
	if err := auditpack.WriteFile(flags.out, packBytes); err != nil {
		return newPacketError(packetErrorOutputWriteFailed, fmt.Errorf("write pack: %w", err))
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(packResponse{
		PackPath:   flags.out,
		PackSHA256: sha256HexBytes(packBytes),
	})
}

type packFlags struct {
	bundle    string
	out       string
	overwrite bool
}

func parsePackFlags(args []string) (packFlags, error) {
	var flags packFlags
	for i := 0; i < len(args); i++ {
		name, value, hasValue, err := parseFlag(args, i)
		if err != nil {
			return packFlags{}, err
		}
		switch name {
		case "bundle":
			if !hasValue {
				i++
				if i >= len(args) {
					return packFlags{}, fmt.Errorf("%w: --bundle requires a value", ErrUsage)
				}
				value = args[i]
			}
			if flags.bundle != "" {
				return packFlags{}, fmt.Errorf("%w: duplicate flag --bundle", ErrUsage)
			}
			flags.bundle = value
		case "out":
			if !hasValue {
				i++
				if i >= len(args) {
					return packFlags{}, fmt.Errorf("%w: --out requires a value", ErrUsage)
				}
				value = args[i]
			}
			if flags.out != "" {
				return packFlags{}, fmt.Errorf("%w: duplicate flag --out", ErrUsage)
			}
			flags.out = value
		case "overwrite":
			if hasValue {
				return packFlags{}, fmt.Errorf("%w: --overwrite does not take a value", ErrUsage)
			}
			flags.overwrite = true
		default:
			return packFlags{}, fmt.Errorf("%w: unknown flag --%s", ErrUsage, name)
		}
	}
	if flags.bundle == "" {
		return packFlags{}, fmt.Errorf("%w: --bundle is required", ErrUsage)
	}
	if flags.out == "" {
		return packFlags{}, fmt.Errorf("%w: --out is required", ErrUsage)
	}
	return flags, nil
}

// parseFlag parses a single --name or --name=value flag from args[i].
// Returns (name, value, hasValue, error). hasValue is true when the value was
// provided inline as --name=value; in that case the caller must not advance i.
func parseFlag(args []string, i int) (name, value string, hasValue bool, err error) {
	arg := args[i]
	if len(arg) < 2 || arg[:2] != "--" {
		return "", "", false, fmt.Errorf("%w: unexpected argument %q (expected --flag)", ErrUsage, arg)
	}
	rest := arg[2:]
	if rest == "" {
		return "", "", false, fmt.Errorf("%w: unknown flag %q", ErrUsage, arg)
	}
	for j, ch := range rest {
		if ch == '=' {
			return rest[:j], rest[j+1:], true, nil
		}
	}
	return rest, "", false, nil
}

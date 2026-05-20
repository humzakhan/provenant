package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	auditpack "github.com/provenant/provenant/audit/pack"
	auditreport "github.com/provenant/provenant/audit/report"
)

const commandAuditHTML = "audit-html"

// runAuditHTML implements `provenant audit-html --pack <pack.json> --out <report.html>`.
//
// Reads the audit pack at --pack, renders it via audit/report.RenderHTML, and
// writes the result to --out.  Missing --pack → exit 2 (user error).
func (r Runner) runAuditHTML(_ context.Context, args []string, _ io.Reader, _ io.Writer) error {
	flags, err := parseAuditHTMLFlags(args)
	if err != nil {
		return userError(err)
	}

	f, err := os.Open(flags.pack)
	if err != nil {
		return userError(fmt.Errorf("%w: open pack %s: %v", ErrMissingRequestPath, flags.pack, err))
	}
	defer f.Close()

	doc, err := auditpack.Decode(f)
	if err != nil {
		return newPacketError(packetErrorRequestInvalid, fmt.Errorf("decode pack: %w", err))
	}

	htmlBytes, err := auditreport.RenderHTML(&doc)
	if err != nil {
		return newPacketError(packetErrorReportFailed, fmt.Errorf("render HTML: %w", err))
	}

	if err := auditpack.WriteFile(flags.out, htmlBytes); err != nil {
		return newPacketError(packetErrorOutputWriteFailed, fmt.Errorf("write HTML: %w", err))
	}
	return nil
}

type auditHTMLFlags struct {
	pack string
	out  string
}

func parseAuditHTMLFlags(args []string) (auditHTMLFlags, error) {
	var flags auditHTMLFlags
	for i := 0; i < len(args); i++ {
		name, value, hasValue, err := parseFlag(args, i)
		if err != nil {
			return auditHTMLFlags{}, err
		}
		switch name {
		case "pack":
			if !hasValue {
				i++
				if i >= len(args) {
					return auditHTMLFlags{}, fmt.Errorf("%w: --pack requires a value", ErrUsage)
				}
				value = args[i]
			}
			if flags.pack != "" {
				return auditHTMLFlags{}, fmt.Errorf("%w: duplicate flag --pack", ErrUsage)
			}
			flags.pack = value
		case "out":
			if !hasValue {
				i++
				if i >= len(args) {
					return auditHTMLFlags{}, fmt.Errorf("%w: --out requires a value", ErrUsage)
				}
				value = args[i]
			}
			if flags.out != "" {
				return auditHTMLFlags{}, fmt.Errorf("%w: duplicate flag --out", ErrUsage)
			}
			flags.out = value
		default:
			return auditHTMLFlags{}, fmt.Errorf("%w: unknown flag --%s", ErrUsage, name)
		}
	}
	if flags.pack == "" {
		return auditHTMLFlags{}, fmt.Errorf("%w: --pack is required", ErrUsage)
	}
	if flags.out == "" {
		return auditHTMLFlags{}, fmt.Errorf("%w: --out is required", ErrUsage)
	}
	return flags, nil
}

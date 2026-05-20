// Package sources provides a named-source registry that maps logical source
// names (e.g. "ledger.get_balance") to concrete (reader, path, selector)
// tuples. Callers can use the registry as a convenience layer on top of the
// raw read-mint verb so that skill packs reference stable names rather than
// bare file paths.
package sources

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	kernel "github.com/provenant/provenant/kernel/public"
)

const schemaVersion = "provenant.sources/v1"

// ErrUnknownSource is returned when Resolve is called with a name that is not
// present in the registry.
var ErrUnknownSource = errors.New("sources: unknown source name")

// ErrInvalidRegistry is returned when the registry document fails validation.
var ErrInvalidRegistry = errors.New("sources: invalid registry")

// Entry is one named-source entry from the registry.
type Entry struct {
	// Reader is the reader name (e.g. "extract.csv_rows", "extract.json").
	Reader string
	// Path is the file path supplied to the reader.
	Path string
	// Selector is the deterministic selector applied by the reader.
	Selector kernel.SourceSelector
	// OutputPath is the JSONPath / JSON-Pointer used to extract the mint value.
	OutputPath string
	// Unit is the optional unit label attached to minted tokens.
	Unit string
}

// Registry is an immutable set of named source entries decoded from a
// sources.json config file.
type Registry struct {
	entries map[string]Entry
}

// registryDoc is the on-disk JSON shape of a sources config file.
type registryDoc struct {
	SchemaVersion string                `json:"schema_version"`
	Sources       map[string]entryDoc   `json:"sources"`
}

// entryDoc is the on-disk JSON shape of one source entry.
type entryDoc struct {
	Reader     string                 `json:"reader"`
	Path       string                 `json:"path"`
	Selector   kernel.SourceSelector  `json:"selector"`
	OutputPath string                 `json:"output_path"`
	Unit       string                 `json:"unit,omitempty"`
}

// Load decodes a registry from r. The reader must contain a valid
// provenant.sources/v1 JSON document. Unknown schema versions are rejected.
// Unknown source names or empty required fields are detected at Resolve time.
func Load(r io.Reader) (*Registry, error) {
	limited := &io.LimitedReader{R: r, N: maxRegistryBytes + 1}
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()

	var doc registryDoc
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrInvalidRegistry, err)
	}
	// Reject trailing content.
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing bytes after registry document", ErrInvalidRegistry)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("%w: registry document exceeds size limit", ErrInvalidRegistry)
	}
	if doc.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("%w: unsupported schema_version %q (want %q)", ErrInvalidRegistry, doc.SchemaVersion, schemaVersion)
	}

	entries := make(map[string]Entry, len(doc.Sources))
	for name, e := range doc.Sources {
		if name == "" {
			return nil, fmt.Errorf("%w: source name must not be empty", ErrInvalidRegistry)
		}
		if e.Reader == "" {
			return nil, fmt.Errorf("%w: source %q: reader is required", ErrInvalidRegistry, name)
		}
		if e.Path == "" {
			return nil, fmt.Errorf("%w: source %q: path is required", ErrInvalidRegistry, name)
		}
		if e.OutputPath == "" {
			return nil, fmt.Errorf("%w: source %q: output_path is required", ErrInvalidRegistry, name)
		}
		entries[name] = Entry{
			Reader:     e.Reader,
			Path:       e.Path,
			Selector:   e.Selector,
			OutputPath: e.OutputPath,
			Unit:       e.Unit,
		}
	}

	return &Registry{entries: entries}, nil
}

// Resolve looks up a named source. It returns (Entry, true) on success or
// (Entry{}, false) when the name is not present. Callers should treat a false
// result as a user error (fail-closed).
func (reg *Registry) Resolve(name string) (Entry, bool) {
	e, ok := reg.entries[name]
	return e, ok
}

// maxRegistryBytes is the upper bound on a sources.json document accepted by
// Load. 256 KiB is large enough for any realistic skill pack.
const maxRegistryBytes = 256 << 10

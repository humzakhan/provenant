package request

import (
	"fmt"
	"sort"
	"strings"
)

func validateReconcile(cfg ReconcileConfig, files fileIndex, errs *ValidationErrors) {
	if strings.TrimSpace(cfg.Mode) == "" {
		errs.add("reconcile.mode", CodeMissingRequired, "reconciliation mode is required")
		return
	}
	if !isSupportedMode(cfg.Mode) {
		errs.add("reconcile.mode", CodeUnsupportedEnum, fmt.Sprintf("unsupported mode %q", cfg.Mode))
		return
	}

	left, right, ok := validateComparedFiles(cfg, files, errs)
	if !ok {
		validateToleranceUsage(cfg, nil, nil, errs)
		validateGroupUsage(cfg, nil, nil, errs)
		return
	}

	validateToleranceUsage(cfg, left, right, errs)
	validateGroupUsage(cfg, left, right, errs)

	switch cfg.Mode {
	case ModeExactMatch:
		validateMatchOnRequired(cfg, left, right, errs)
		validateCompareFields(cfg, left, right, false, errs)
	case ModeAmountTolerance:
		validateMatchOnRequired(cfg, left, right, errs)
		validateCompareFields(cfg, left, right, true, errs)
		if cfg.Tolerance == nil {
			errs.add("reconcile.tolerance", CodeMissingRequired, "amount_tolerance requires tolerance")
		}
	case ModeExistence:
		validateMatchOnRequired(cfg, left, right, errs)
		if len(cfg.Compare) > 0 {
			errs.add("reconcile.compare", CodeInvalidModeConfig, "existence does not allow compare fields")
		}
	case ModeOneToMany:
		validateMatchOnRequired(cfg, left, right, errs)
		validateCompareFields(cfg, left, right, true, errs)
		if cfg.Group == nil {
			errs.add("reconcile.group", CodeMissingRequired, "one_to_many requires grouping config")
		}
	case ModeSchemaValidation:
		if len(cfg.MatchOn) > 0 {
			errs.add("reconcile.match_on", CodeInvalidModeConfig, "schema_validation does not allow match_on")
		}
		if len(cfg.Compare) > 0 {
			errs.add("reconcile.compare", CodeInvalidModeConfig, "schema_validation does not allow compare")
		}
	}
}

func validateComparedFiles(cfg ReconcileConfig, files fileIndex, errs *ValidationErrors) (*fileInfo, *fileInfo, bool) {
	if strings.TrimSpace(cfg.Left) == "" {
		errs.add("reconcile.left", CodeMissingRequired, "left file is required")
	}
	if strings.TrimSpace(cfg.Right) == "" {
		errs.add("reconcile.right", CodeMissingRequired, "right file is required")
	}
	leftIndex, leftOK := files.byID[cfg.Left]
	rightIndex, rightOK := files.byID[cfg.Right]
	if cfg.Left != "" && !leftOK {
		errs.add("reconcile.left", CodeInvalidReference, fmt.Sprintf("file %q does not exist", cfg.Left))
	}
	if cfg.Right != "" && !rightOK {
		errs.add("reconcile.right", CodeInvalidReference, fmt.Sprintf("file %q does not exist", cfg.Right))
	}
	if leftOK && rightOK && cfg.Left == cfg.Right {
		errs.add("reconcile.right", CodeInvalidModeConfig, "left and right must reference distinct files")
		return nil, nil, false
	}
	if !leftOK || !rightOK {
		return nil, nil, false
	}
	return &files.info[leftIndex], &files.info[rightIndex], true
}

func validateMatchOnRequired(cfg ReconcileConfig, left, right *fileInfo, errs *ValidationErrors) {
	if len(cfg.MatchOn) == 0 {
		errs.add("reconcile.match_on", CodeMissingRequired, "match_on is required")
		return
	}
	seen := make(map[string]struct{}, len(cfg.MatchOn))
	for i, name := range cfg.MatchOn {
		path := fmt.Sprintf("reconcile.match_on[%d]", i)
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			errs.add(path, CodeMissingRequired, "match_on field is required")
			continue
		}
		if _, exists := seen[trimmed]; exists {
			errs.add(path, CodeDuplicateDeclaration, fmt.Sprintf("match_on field %q is duplicated", trimmed))
			continue
		}
		seen[trimmed] = struct{}{}
		validateSharedField(path, trimmed, left, right, RoleKey, errs)
	}

	required := sortedRequiredKeys(left.requiredKeys, right.requiredKeys)
	for _, name := range required {
		if _, exists := seen[name]; !exists {
			errs.add("reconcile.match_on", CodeInvalidModeConfig, fmt.Sprintf("required key field %q must appear in match_on", name))
		}
	}
}

func validateCompareFields(cfg ReconcileConfig, left, right *fileInfo, requireAmount bool, errs *ValidationErrors) {
	if len(cfg.Compare) == 0 {
		if requireAmount {
			errs.add("reconcile.compare", CodeMissingRequired, "compare is required")
		}
		return
	}

	seen := make(map[string]struct{}, len(cfg.Compare))
	hasAmount := false
	for i, name := range cfg.Compare {
		path := fmt.Sprintf("reconcile.compare[%d]", i)
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			errs.add(path, CodeMissingRequired, "compare field is required")
			continue
		}
		if _, exists := seen[trimmed]; exists {
			errs.add(path, CodeDuplicateDeclaration, fmt.Sprintf("compare field %q is duplicated", trimmed))
			continue
		}
		seen[trimmed] = struct{}{}
		role := validateSharedField(path, trimmed, left, right, "", errs)
		if role == RoleAmount {
			hasAmount = true
		}
		if role == RoleIgnored {
			errs.add(path, CodeInvalidModeConfig, "ignored fields cannot be compared")
		}
	}

	if requireAmount && !hasAmount {
		errs.add("reconcile.compare", CodeInvalidModeConfig, "compare must include an amount field")
	}
	for fieldName, column := range left.amountFields {
		if _, exists := seen[fieldName]; !exists || column.Numeric == nil {
			continue
		}
		if _, exists := seen[column.Numeric.CurrencyField]; !exists {
			errs.add("reconcile.compare", CodeInvalidModeConfig, fmt.Sprintf("compare must include currency field %q for amount field %q", column.Numeric.CurrencyField, fieldName))
		}
	}
}

func validateToleranceUsage(cfg ReconcileConfig, left, right *fileInfo, errs *ValidationErrors) {
	validateTolerance("reconcile.tolerance", cfg.Tolerance, errs)
	if cfg.Tolerance == nil {
		return
	}
	if cfg.Mode != ModeAmountTolerance && cfg.Mode != ModeOneToMany {
		errs.add("reconcile.tolerance", CodeInvalidModeConfig, fmt.Sprintf("tolerance is not allowed for mode %q", cfg.Mode))
	}
	if left == nil || right == nil {
		return
	}
}

func validateGroupUsage(cfg ReconcileConfig, left, right *fileInfo, errs *ValidationErrors) {
	if cfg.Group == nil {
		return
	}
	if cfg.Mode != ModeOneToMany {
		errs.add("reconcile.group", CodeInvalidModeConfig, fmt.Sprintf("group is not allowed for mode %q", cfg.Mode))
		return
	}
	if len(cfg.Group.LeftKey) == 0 {
		errs.add("reconcile.group.left_key", CodeMissingRequired, "group left_key is required")
		return
	}
	if len(cfg.Group.RightKey) == 0 {
		errs.add("reconcile.group.right_key", CodeMissingRequired, "group right_key is required")
		return
	}
	if left == nil || right == nil {
		return
	}
	validateFieldList("reconcile.group.left_key", cfg.Group.LeftKey, left, RoleKey, errs)
	validateFieldList("reconcile.group.right_key", cfg.Group.RightKey, right, RoleKey, errs)
}

func validateFieldList(path string, names []string, file *fileInfo, requiredRole string, errs *ValidationErrors) {
	seen := make(map[string]struct{}, len(names))
	for i, name := range names {
		entryPath := fmt.Sprintf("%s[%d]", path, i)
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			errs.add(entryPath, CodeMissingRequired, "field reference is required")
			continue
		}
		if _, exists := seen[trimmed]; exists {
			errs.add(entryPath, CodeDuplicateDeclaration, fmt.Sprintf("field %q is duplicated", trimmed))
			continue
		}
		seen[trimmed] = struct{}{}
		role, ok := file.rolesByField[trimmed]
		if !ok {
			errs.add(entryPath, CodeInvalidReference, fmt.Sprintf("field %q does not exist", trimmed))
			continue
		}
		if requiredRole != "" && role != requiredRole {
			errs.add(entryPath, CodeInvalidReference, fmt.Sprintf("field %q must have role %q", trimmed, requiredRole))
		}
	}
}

func validateSharedField(path, name string, left, right *fileInfo, requiredRole string, errs *ValidationErrors) string {
	leftRole, leftOK := left.rolesByField[name]
	rightRole, rightOK := right.rolesByField[name]
	if !leftOK {
		errs.add(path, CodeInvalidReference, fmt.Sprintf("field %q does not exist on left file", name))
	}
	if !rightOK {
		errs.add(path, CodeInvalidReference, fmt.Sprintf("field %q does not exist on right file", name))
	}
	if !leftOK || !rightOK {
		return ""
	}
	if leftRole != rightRole {
		errs.add(path, CodeAmbiguousRole, fmt.Sprintf("field %q has role %q on left and %q on right", name, leftRole, rightRole))
		return ""
	}
	if requiredRole != "" && leftRole != requiredRole {
		errs.add(path, CodeInvalidReference, fmt.Sprintf("field %q must have role %q on both files", name, requiredRole))
	}
	return leftRole
}

func sortedRequiredKeys(left, right map[string]struct{}) []string {
	union := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		union[key] = struct{}{}
	}
	for key := range right {
		union[key] = struct{}{}
	}
	keys := make([]string, 0, len(union))
	for key := range union {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isSupportedMode(mode string) bool {
	switch mode {
	case ModeExactMatch, ModeAmountTolerance, ModeExistence, ModeOneToMany, ModeSchemaValidation:
		return true
	default:
		return false
	}
}

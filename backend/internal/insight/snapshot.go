package insight

import "sort"

// Kinds returns the resource kinds the runbook can correlate to a
// deterministic diagnosis route (M82 discovery snapshot).
func Kinds() []string {
	out := make([]string, 0, len(diagnosisByKind))
	for k := range diagnosisByKind {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Operations lists the unique remediable operation actions offered by the
// runbook catalog (sorted; M19/M44 dry-run candidates).
func Operations() []string {
	seen := map[string]bool{}
	for _, ops := range operationByKind {
		for _, op := range ops {
			seen[op.Action] = true
		}
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

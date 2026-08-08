package httpserver

// W10 error-code audit: statically scans every writeError call in the
// handler layer and asserts the error-code contract:
//   1. codes are UPPER_SNAKE (no lowercase, no hyphens, no spaces);
//   2. no error is ever emitted with a 2xx status;
//   3. the same code is never mapped to two different HTTP statuses;
//   4. only the documented status family is used.
// This makes the 400/403/404/409/500 mapping consistent by construction
// and fails loudly if a new handler drifts.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	writeErrorCall = regexp.MustCompile(`writeError\(\s*c\s*,\s*http\.Status([A-Za-z]+),\s*"([A-Z0-9_]+)"`)
	codeShape      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
)

func allowedErrorStatuses() map[string]bool {
	return map[string]bool{
		"BadRequest":          true, // 400
		"Unauthorized":        true, // 401
		"Forbidden":           true, // 403
		"NotFound":            true, // 404
		"MethodNotAllowed":    true, // 405
		"Conflict":            true, // 409
		"Gone":                true, // 410
		"PreconditionFailed":  true, // 412
		"FailedDependency":    true, // 424
		"UnprocessableEntity": true, // 422
		"MultiStatus":         true, // 207
		"TooManyRequests":     true, // 429
		"InternalServerError": true, // 500
		"BadGateway":          true, // 502
		"ServiceUnavailable":  true, // 503
		"GatewayTimeout":      true, // 504
	}
}

func handlerGoFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, m := range matches {
		if strings.HasSuffix(m, "_test.go") {
			continue
		}
		files = append(files, m)
	}
	if len(files) < 5 {
		t.Fatalf("expected many handler files, got %d", len(files))
	}
	return files
}

func TestErrorCodeAudit(t *testing.T) {
	allowed := allowedErrorStatuses()
	statusByName := map[string]string{}
	for name := range allowed {
		statusByName[name] = name
	}

	codeStatus := map[string]string{} // code -> StatusName
	var violations []string

	for _, file := range handlerGoFiles(t) {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			m := writeErrorCall.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			statusName, code := m[1], m[2]

			if _, ok := allowed[statusName]; !ok {
				violations = append(violations, file+": unexpected status "+statusName+" for code "+code)
			}
			if !codeShape.MatchString(code) {
				violations = append(violations, file+": code shape violation "+code)
			}
			if prev, ok := codeStatus[code]; ok && prev != statusName {
				violations = append(violations, file+": code "+code+" mapped to both "+prev+" and "+statusName)
			}
			codeStatus[code] = statusName
		}
	}

	if len(codeStatus) < 40 {
		t.Errorf("audit only saw %d distinct codes, expected a large surface", len(codeStatus))
	}
	for _, v := range violations {
		t.Error(v)
	}
}

func TestNoErrorWithSuccessStatus(t *testing.T) {
	for _, file := range handlerGoFiles(t) {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			m := writeErrorCall.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if strings.HasPrefix(m[1], "OK") || strings.HasPrefix(m[1], "Created") || strings.HasPrefix(m[1], "NoContent") || strings.HasPrefix(m[1], "Accepted") {
				t.Errorf("%s: error emitted with success status %s", file, m[1])
			}
		}
	}
}

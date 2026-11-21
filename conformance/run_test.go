package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/macropro"
)

// recordingTB captures Errorf calls. Embedding testing.TB satisfies the
// interface's unexported method.
type recordingTB struct {
	testing.TB
	errors []string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func runSuite() Suite[struct{}] {
	return Suite[struct{}]{
		Macros: macropro.MacroMap[struct{}]{"foo": staticMacro("F")},
		Cases:  []Case{{Query: "SELECT $__foo"}},
	}
}

func TestRun_UpdateWritesTheGoldenFileAndCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "testdata", "macros.md")
	rec := &recordingTB{TB: t}
	Run(rec, runSuite(), path, true)
	if len(rec.errors) != 0 {
		t.Fatalf("update reported errors: %v", rec.errors)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := Markdown(runSuite(), Results(runSuite())); string(got) != want {
		t.Errorf("golden file content differs from Markdown()")
	}
}

func TestRun_MatchingGoldenPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "macros.md")
	Run(t, runSuite(), path, true)
	rec := &recordingTB{TB: t}
	Run(rec, runSuite(), path, false)
	if len(rec.errors) != 0 {
		t.Errorf("matching golden reported errors: %v", rec.errors)
	}
}

func TestRun_MissingGoldenFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.md")
	rec := &recordingTB{TB: t}
	Run(rec, runSuite(), path, false)
	if len(rec.errors) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(rec.errors), rec.errors)
	}
	want := "golden file " + path + " does not exist, rerun with the update flag to create it"
	if !strings.Contains(rec.errors[0], want) {
		t.Errorf("error = %q, want it to contain %q", rec.errors[0], want)
	}
}

func TestRun_ChangedOutputFailsAndNamesTheLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "macros.md")
	Run(t, runSuite(), path, true)
	changed := runSuite()
	changed.Macros["foo"] = staticMacro("G")
	rec := &recordingTB{TB: t}
	Run(rec, changed, path, false)
	if len(rec.errors) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(rec.errors), rec.errors)
	}
	msg := rec.errors[0]
	for _, want := range []string{"output differs from " + path, "line 12", "want: | `SELECT $__foo` | `SELECT $__foo` | `SELECT F` |", "got:  | `SELECT $__foo` | `SELECT $__foo` | `SELECT G` |", "rerun with the update flag to accept the new output"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message lacks %q:\n%s", want, msg)
		}
	}
}

func TestDiffLines_LimitsOutput(t *testing.T) {
	want := strings.Repeat("a\n", 20)
	got := strings.Repeat("b\n", 20)
	if n := strings.Count(diffLines(want, got, 10), "line "); n != 10 {
		t.Errorf("diffLines reported %d lines, want 10", n)
	}
}

func TestDiffLines_ReportsADifferenceAtTheTail(t *testing.T) {
	tests := []struct{ name, want, got string }{
		{"missing final newline", "a\nb\n", "a\nb"},
		{"extra blank line", "a\n", "a\n\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if d := diffLines(tc.want, tc.got, 10); !strings.Contains(d, "line 3") {
				t.Errorf("diffLines(%q, %q) = %q, want it to name line 3", tc.want, tc.got, d)
			}
		})
	}
}

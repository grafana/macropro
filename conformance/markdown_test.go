package conformance

import (
	"strings"
	"testing"

	"github.com/grafana/macropro"
)

func TestCodeSpan(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "SELECT 1", "`SELECT 1`"},
		{"empty", "", "` `"},
		{"pipe", "a || b", "`a \\|\\| b`"},
		{"backslash", `C:\path`, "`C:\\\\path`"},
		{"newline", "a\nb", "`a\\nb`"},
		{"carriage return", "a\r\nb", "`a\\r\\nb`"},
		{"one backtick", "`col`", "`` `col` ``"},
		{"two backticks", "a `` b", "```a `` b```"},
		{"leading space", " >= 1", "`  >= 1 `"},
		{"trailing space", "x ", "` x  `"},
		{"unicode", "toDateTime('ünïcode')", "`toDateTime('ünïcode')`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := codeSpan(tc.in); got != tc.want {
				t.Errorf("codeSpan(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMarkdown_Document(t *testing.T) {
	s := Suite[struct{}]{Macros: macropro.MacroMap[struct{}]{"foo": staticMacro("F")}}
	results := []Result{
		{Name: "one", Query: "SELECT $__foo", Output: "SELECT F"},
		{Name: "bad", Query: "$__bar(x)", Error: "macro $__bar(x): boom"},
	}
	got := Markdown(s, results)
	want := "# Macro conformance\n\n" +
		"- From: 2024-01-01T00:00:00Z\n" +
		"- To: 2024-01-02T00:00:00Z\n" +
		"- Interval: 5m0s\n" +
		"- IntervalMS: 300000\n" +
		"- Table: `metrics`\n" +
		"- Column: `value`\n\n" +
		"| Case | Invocation | Output |\n" +
		"|---|---|---|\n" +
		"| `one` | `SELECT $__foo` | `SELECT F` |\n" +
		"| `bad` | `$__bar(x)` | `error: macro $__bar(x): boom` |\n"
	if got != want {
		t.Errorf("Markdown() =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdown_IsDeterministicAndKeepsDuplicates(t *testing.T) {
	s := Suite[struct{}]{Macros: macropro.DefaultMacros[struct{}]()}
	results := []Result{
		{Name: "dup", Query: "$__interval", Output: "5m"},
		{Name: "dup", Query: "$__interval", Output: "5m"},
	}
	first, second := Markdown(s, results), Markdown(s, results)
	if first != second {
		t.Fatal("Markdown rendered different bytes for the same input")
	}
	if strings.Count(first, "| `dup` |") != 2 {
		t.Errorf("duplicate rows were not both rendered:\n%s", first)
	}
	if !strings.HasSuffix(first, "\n") || strings.Contains(first, "\r") {
		t.Error("document must end with LF and contain no CR")
	}
}

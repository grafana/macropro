package conformance

import (
	"fmt"
	"strings"
	"time"
)

// Markdown renders the suite's context and results as the golden document.
// Rendering is a pure function of its inputs, so Run compares bytes and never
// parses the file.
func Markdown[T any](s Suite[T], results []Result) string {
	ctx := s.context()
	var b strings.Builder
	b.WriteString("# Macro conformance\n\n")
	fmt.Fprintf(&b, "- From: %s\n", ctx.TimeRange.From.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- To: %s\n", ctx.TimeRange.To.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Interval: %s\n", ctx.Interval)
	fmt.Fprintf(&b, "- IntervalMS: %d\n", ctx.IntervalMS)
	fmt.Fprintf(&b, "- Table: %s\n", codeSpan(ctx.Table))
	fmt.Fprintf(&b, "- Column: %s\n\n", codeSpan(ctx.Column))
	b.WriteString("| Case | Invocation | Output |\n|---|---|---|\n")
	for _, r := range results {
		out := r.Output
		if r.Error != "" {
			out = "error: " + r.Error
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", codeSpan(r.Name), codeSpan(r.Query), codeSpan(out))
	}
	return b.String()
}

var spanEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "|", `\|`)

// codeSpan renders s as a CommonMark code span that survives a table cell.
func codeSpan(s string) string {
	if s == "" {
		return "` `"
	}
	s = spanEscaper.Replace(s)
	fence := strings.Repeat("`", longestBacktickRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") ||
		strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		s = " " + s + " "
	}
	return fence + s + fence
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] != '`' {
			run = 0
			continue
		}
		run++
		if run > longest {
			longest = run
		}
	}
	return longest
}

package macropro

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestInterpolate_basicSubstitution(t *testing.T) {
	macros := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "BAR", nil
		},
	}
	got, err := Interpolate("SELECT $__foo FROM t", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "SELECT BAR FROM t"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_withArgs(t *testing.T) {
	macros := MacroMap[struct{}]{
		"add": func(_ QueryContext[struct{}], args []string) (string, error) {
			if len(args) != 2 {
				return "", nil
			}
			return args[0] + "+" + args[1], nil
		},
	}
	got, err := Interpolate("$__add(a, b)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "a+b"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedParens(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			if len(args) != 1 {
				return "", nil
			}
			return "[" + args[0] + "]", nil
		},
	}
	// Argument itself contains a function call with parens.
	got, err := Interpolate("$__wrap(COALESCE(a, b))", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[COALESCE(a, b)]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_longestMatchFirst(t *testing.T) {
	calls := map[string]int{}
	macros := MacroMap[struct{}]{
		"interval": func(_ QueryContext[struct{}], _ []string) (string, error) {
			calls["interval"]++
			return "SHORT", nil
		},
		"interval_ms": func(_ QueryContext[struct{}], _ []string) (string, error) {
			calls["interval_ms"]++
			return "LONG", nil
		},
	}
	got, err := Interpolate("$__interval_ms and $__interval", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "LONG and SHORT"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if calls["interval"] != 1 {
		t.Errorf("interval called %d times, want 1", calls["interval"])
	}
	if calls["interval_ms"] != 1 {
		t.Errorf("interval_ms called %d times, want 1", calls["interval_ms"])
	}
}

func TestInterpolate_unknownMacroUnchanged(t *testing.T) {
	macros := MacroMap[struct{}]{
		"known": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "KNOWN", nil
		},
	}
	query := "$__known and $__unknown(x, y)"
	got, err := Interpolate(query, macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "KNOWN and $__unknown(x, y)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_handlerErrorReturnsOriginal(t *testing.T) {
	macros := MacroMap[struct{}]{
		"bad": func(_ QueryContext[struct{}], args []string) (string, error) {
			if len(args) != 2 {
				return "", errorf("need 2 args, got %d", len(args))
			}
			return "ok", nil
		},
	}
	original := "SELECT $__bad(one) FROM t"
	got, err := Interpolate(original, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != original {
		t.Errorf("on error, Interpolate should return the original query; got %q", got)
	}
}

func TestInterpolate_emptyMacros(t *testing.T) {
	query := "SELECT $__timeFilter(time) FROM t"
	got, err := Interpolate(query, MacroMap[struct{}]{}, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != query {
		t.Errorf("empty MacroMap should leave query unchanged; got %q", got)
	}
}

func TestInterpolate_multipleOccurrences(t *testing.T) {
	macros := MacroMap[struct{}]{
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) { return "1", nil },
	}
	got, err := Interpolate("$__x + $__x + $__x", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "1 + 1 + 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_zeroArgWithParens(t *testing.T) {
	macros := MacroMap[struct{}]{
		"now": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "NOW()", nil
		},
	}
	got, err := Interpolate("$__now()", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "NOW()"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestInterpolate_macroInLineCommentNotExpanded is a regression test for the
// OOM vulnerability where a macro hidden behind a SQL -- comment was still
// evaluated by the macro engine, triggering fill-mode side effects that caused
// ResampleWideFrame to allocate memory for billions of rows.
func TestInterpolate_macroInLineCommentNotExpanded(t *testing.T) {
	expanded := false
	macros := MacroMap[struct{}]{
		"timeGroup": func(_ QueryContext[struct{}], _ []string) (string, error) {
			expanded = true
			return "FLOOR(UNIX_TIMESTAMP(time)/1)*1", nil
		},
	}
	// The $__timeGroup macro is intentionally placed after a -- comment, as in
	// the reported PoC: rawSql ends with "-- $__timeGroup(time, '1s', 0)"
	query := `SELECT time, val FROM metrics -- $__timeGroup(time, '1s', 0)`
	got, err := Interpolate(query, macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded {
		t.Error("macro inside -- comment was expanded; fill-mode side effects could be triggered")
	}
	// The comment region should still be present (as spaces) — the non-comment
	// part of the query must be unchanged.
	if want := "SELECT time, val FROM metrics "; got[:len(want)] != want {
		t.Errorf("query prefix mangled: got %q", got[:len(want)])
	}
}

func TestInterpolate_macroInBlockCommentNotExpanded(t *testing.T) {
	expanded := false
	macros := MacroMap[struct{}]{
		"timeGroup": func(_ QueryContext[struct{}], _ []string) (string, error) {
			expanded = true
			return "FLOOR(...)", nil
		},
	}
	query := `SELECT time, val FROM metrics /* $__timeGroup(time, '1s', 0) */`
	_, err := Interpolate(query, macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded {
		t.Error("macro inside /* */ comment was expanded")
	}
}

func TestMergeMacros_overrideWins(t *testing.T) {
	base := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) { return "base", nil },
		"bar": func(_ QueryContext[struct{}], _ []string) (string, error) { return "bar", nil },
	}
	override := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) { return "override", nil },
	}
	merged := MergeMacros(base, override)

	got, _ := merged["foo"](QueryContext[struct{}]{}, nil)
	if got != "override" {
		t.Errorf("override should win; got %q", got)
	}
	got, _ = merged["bar"](QueryContext[struct{}]{}, nil)
	if got != "bar" {
		t.Errorf("base key not preserved; got %q", got)
	}
}

func TestMergeMacros_doesNotMutateBase(t *testing.T) {
	base := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) { return "base", nil },
	}
	override := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) { return "override", nil },
	}
	_ = MergeMacros(base, override)

	got, _ := base["foo"](QueryContext[struct{}]{}, nil)
	if got != "base" {
		t.Errorf("MergeMacros mutated base; got %q", got)
	}
}

func TestParseArgs_empty(t *testing.T) {
	args, err := parseArgs("")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 0 {
		t.Errorf("expected empty slice, got %v", args)
	}
}

func TestParseArgs_single(t *testing.T) {
	args, err := parseArgs(" time ")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != "time" {
		t.Errorf("got %v", args)
	}
}

func TestParseArgs_multiple(t *testing.T) {
	args, err := parseArgs("a, b, c")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[0] != "a" || args[1] != "b" || args[2] != "c" {
		t.Errorf("got %v", args)
	}
}

func TestParseArgs_nestedParens(t *testing.T) {
	args, err := parseArgs("COALESCE(a, b), c")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "COALESCE(a, b)" || args[1] != "c" {
		t.Errorf("got %v", args)
	}
}

func TestParseArgs_quotedComma(t *testing.T) {
	args, err := parseArgs(`'a,b', c`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != `'a,b'` || args[1] != "c" {
		t.Errorf("got %v", args)
	}
}

func TestInterpolate_withPrefix_fluxStyle(t *testing.T) {
	// Flux uses "v." as its macro prefix, e.g. v.timeRangeStart, v.bucket.
	macros := MacroMap[struct{}]{
		"timeRangeStart": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "2024-01-01T00:00:00Z", nil
		},
		"bucket": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return `"grafana"`, nil
		},
	}
	query := `from(bucket: v.bucket) |> range(start: v.timeRangeStart)`
	got, err := Interpolate(query, macros, QueryContext[struct{}]{}, WithPrefix("v."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `from(bucket: "grafana") |> range(start: 2024-01-01T00:00:00Z)`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInterpolate_withPrefix_influxqlLegacy(t *testing.T) {
	// InfluxQL has legacy bare-$ macros like $timeFilter and $interval.
	macros := MacroMap[struct{}]{
		"timeFilter": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "time >= 1000ms and time <= 2000ms", nil
		},
		"interval": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "5s", nil
		},
	}
	query := `SELECT mean(value) FROM cpu WHERE $timeFilter GROUP BY time($interval)`
	got, err := Interpolate(query, macros, QueryContext[struct{}]{}, WithPrefix("$"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `SELECT mean(value) FROM cpu WHERE time >= 1000ms and time <= 2000ms GROUP BY time(5s)`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInterpolate_withPrefix_doesNotStripSQLComments(t *testing.T) {
	// When a non-SQL prefix is in use, it's common to also swap comment
	// handling. Here we verify that WithComments(0) disables auto-stripping.
	expanded := false
	macros := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) {
			expanded = true
			return "BAR", nil
		},
	}
	// -- is NOT a comment in Flux; with WithComments(0) the content after --
	// must survive and the macro inside it must expand.
	query := `from(bucket: "b") -- v.foo`
	got, err := Interpolate(query, macros, QueryContext[struct{}]{}, WithPrefix("v."), WithComments(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !expanded {
		t.Error("macro should have been expanded when comment stripping is disabled")
	}
	want := `from(bucket: "b") -- BAR`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInterpolate_withComments_slashStripping(t *testing.T) {
	// Flux uses // for line comments. A macro placed after // should not expand.
	expanded := false
	macros := MacroMap[struct{}]{
		"bucket": func(_ QueryContext[struct{}], _ []string) (string, error) {
			expanded = true
			return `"secret"`, nil
		},
	}
	query := `from(bucket: "b") // v.bucket`
	_, err := Interpolate(query, macros, QueryContext[struct{}]{}, WithPrefix("v."), WithComments(SlashComment|BlockComment))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded {
		t.Error("macro inside // comment should not have been expanded")
	}
}

func TestInterpolate_withPrefix_defaultStillWorks(t *testing.T) {
	// When no options are passed, the default "$__" prefix must still work.
	macros := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "BAR", nil
		},
	}
	got, err := Interpolate("$__foo", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "BAR" {
		t.Errorf("default prefix should still be $__; got %q", got)
	}
}

func TestStripComments_slash(t *testing.T) {
	input := "SELECT * // this is a comment\nFROM t"
	got := StripComments(input, SlashComment)
	// StripComments is length-preserving: the // comment region is replaced
	// with spaces so line/column positions remain accurate.
	want := "SELECT * " + strings.Repeat(" ", len("// this is a comment")) + "\nFROM t"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if len(got) != len(input) {
		t.Errorf("StripComments should be length-preserving; got len %d, want %d", len(got), len(input))
	}
}

// ---- helpers ----

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func errorf(format string, args ...any) error {
	return &simpleError{msg: sprintf(format, args...)}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

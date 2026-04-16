package macropro

import (
	"fmt"
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

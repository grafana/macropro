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
		"now": func(_ QueryContext[struct{}], _ []string) (string, error) {
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

func TestParseArgs_doubledSingleQuote(t *testing.T) {
	// SQL-style doubled-quote escape: 'it''s' is one string literal
	// containing "it's" and a comma inside it must NOT split the args.
	args, err := parseArgs(`'it''s, fine', x`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != `'it''s, fine'` || args[1] != "x" {
		t.Errorf("got %v", args)
	}
}

func TestParseArgs_doubledDoubleQuote(t *testing.T) {
	// Same idea for double-quoted identifiers: "he said ""hi"", then left"
	// is one identifier and commas inside do not split.
	args, err := parseArgs(`"he said ""hi"", then left", y`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != `"he said ""hi"", then left"` || args[1] != "y" {
		t.Errorf("got %v", args)
	}
}

func TestFindClosingParen_doubledQuote(t *testing.T) {
	// A doubled-quote inside a string should not prematurely end the literal,
	// so the outer ')' is correctly located.
	s := `('it''s') rest`
	end, err := findClosingParen(s, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s[end] != ')' || end != 8 {
		t.Errorf("got end=%d (char %q), want 8 (char ')')", end, s[end])
	}
}

func TestInterpolate_macroWithDoubledQuoteArg(t *testing.T) {
	// End-to-end: a macro whose argument contains a doubled-quote escape
	// must be parsed as a single argument and expanded once.
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"id": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
	}
	// $__id('it''s, still one arg') — the comma is inside the string literal.
	query := `SELECT $__id('it''s, still one arg') FROM t`
	result, err := Interpolate(query, macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotArgs) != 1 || gotArgs[0] != `'it''s, still one arg'` {
		t.Errorf("args: got %v, want single arg with doubled quote", gotArgs)
	}
	if result != `SELECT OK FROM t` {
		t.Errorf("result: got %q", result)
	}
}

func TestScanStringLiteral_unterminated(t *testing.T) {
	// An unterminated string literal should not panic; scanner returns len(s).
	s := `'unterminated`
	end := scanStringLiteral(s, 0)
	if end != len(s) {
		t.Errorf("got end=%d, want %d", end, len(s))
	}
}

func TestScanStringLiteral_trailingDoubledQuote(t *testing.T) {
	// 'a''  — opens, sees '', treats as escape, reaches EOF unterminated.
	s := `'a''`
	end := scanStringLiteral(s, 0)
	if end != len(s) {
		t.Errorf("got end=%d, want %d", end, len(s))
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

func TestStripComments_unterminatedBlock(t *testing.T) {
	// An unterminated /* comment should be blanked all the way to EOF — no
	// trailing byte should be emitted as if it were outside the comment.
	input := "SELECT /* $__foo unterminated"
	got := StripComments(input, BlockComment)
	want := "SELECT " + strings.Repeat(" ", len("/* $__foo unterminated"))
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if len(got) != len(input) {
		t.Errorf("length should be preserved; got %d want %d", len(got), len(input))
	}
}

func TestStripComments_unterminatedBlockHidesMacro(t *testing.T) {
	// Regression: the byte at len-1 of an unterminated /* must not survive.
	// Building a string where the would-leak byte is a macro character helps
	// guard against the off-by-one returning.
	input := "/* $__x"
	got := StripComments(input, BlockComment)
	want := strings.Repeat(" ", len(input))
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_lineCommentCRLF(t *testing.T) {
	// Windows line endings: scanner stops at \r, leaving \r\n intact.
	input := "SELECT * -- cmt\r\nFROM t"
	got := StripComments(input, LineComment)
	want := "SELECT * " + strings.Repeat(" ", len("-- cmt")) + "\r\nFROM t"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_lineCommentCROnly(t *testing.T) {
	// Classic-Mac line endings: \r alone terminates a line comment, so the
	// content AFTER \r (including any macro tokens) is preserved and will be
	// evaluated normally by Interpolate.
	input := "SELECT * -- cmt\r$__foo"
	got := StripComments(input, LineComment)
	want := "SELECT * " + strings.Repeat(" ", len("-- cmt")) + "\r$__foo"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_hashCommentCR(t *testing.T) {
	input := "SELECT * # cmt\r$__foo"
	got := StripComments(input, HashComment)
	want := "SELECT * " + strings.Repeat(" ", len("# cmt")) + "\r$__foo"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_slashCommentCR(t *testing.T) {
	input := `from(bucket: "b") // cmt` + "\rv.bucket"
	got := StripComments(input, SlashComment)
	want := `from(bucket: "b") ` + strings.Repeat(" ", len("// cmt")) + "\rv.bucket"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_lineCommentAtEOF(t *testing.T) {
	// Line comment running to EOF (no trailing newline): the whole thing
	// should be blanked, and the output length must match the input.
	input := "SELECT 1 -- trailing"
	got := StripComments(input, LineComment)
	want := "SELECT 1 " + strings.Repeat(" ", len("-- trailing"))
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if len(got) != len(input) {
		t.Errorf("length should be preserved; got %d want %d", len(got), len(input))
	}
}

func TestInterpolate_handlerPanic(t *testing.T) {
	// A handler that panics should not take down the caller. Interpolate
	// must return an error and the (stripped) query.
	macros := MacroMap[struct{}]{
		"boom": func(_ QueryContext[struct{}], _ []string) (string, error) {
			panic("explicit panic")
		},
	}
	_, err := Interpolate("SELECT $__boom FROM t", macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error from panicking handler, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should mention panic; got %v", err)
	}
	if !strings.Contains(err.Error(), "$__boom") {
		t.Errorf("error should include macro name; got %v", err)
	}
}

func TestInterpolate_handlerNilDeref(t *testing.T) {
	// Runtime panics (nil deref, slice out-of-range) must also be recovered,
	// not just explicit panic() calls.
	macros := MacroMap[struct{}]{
		"bad": func(_ QueryContext[struct{}], _ []string) (string, error) {
			var p *string
			return *p, nil
		},
	}
	_, err := Interpolate("$__bad", macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error from nil-deref in handler, got nil")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should mention panic; got %v", err)
	}
}

func TestInterpolate_handlerPanicStopsFurtherExpansion(t *testing.T) {
	// When one handler panics, Interpolate returns immediately; subsequent
	// macro names must not be processed (same semantics as a handler error).
	// Names are sorted longest-first, so "panicker" runs before "short".
	shortCalled := false
	macros := MacroMap[struct{}]{
		"panicker": func(_ QueryContext[struct{}], _ []string) (string, error) {
			panic("nope")
		},
		"short": func(_ QueryContext[struct{}], _ []string) (string, error) {
			shortCalled = true
			return "OK", nil
		},
	}
	_, err := Interpolate("$__panicker $__short", macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error")
	}
	if shortCalled {
		t.Error("later handler should not have been called after panic")
	}
}

func TestInterpolate_errorReturnsUnstrippedOriginal(t *testing.T) {
	// On handler error, Interpolate must return the caller's original query
	// byte-for-byte — not the comment-stripped work copy. A previous version
	// accidentally returned the stripped version, silently mutating the query
	// visible to callers that ignored the error.
	macros := MacroMap[struct{}]{
		"fail": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "", fmt.Errorf("nope")
		},
	}
	// The -- comment contains something that would otherwise be blanked.
	input := "SELECT 1 -- keep this comment visible\n$__fail"
	got, err := Interpolate(input, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got != input {
		t.Errorf("on error, must return input verbatim\ngot  %q\nwant %q", got, input)
	}
}

func TestInterpolate_errorReturnsOriginalOnPanic(t *testing.T) {
	// Same guarantee when the handler panics rather than returning an error.
	macros := MacroMap[struct{}]{
		"boom": func(_ QueryContext[struct{}], _ []string) (string, error) {
			panic("x")
		},
	}
	input := "-- hidden $__boom\n$__boom"
	got, err := Interpolate(input, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got != input {
		t.Errorf("on panic, must return input verbatim\ngot  %q\nwant %q", got, input)
	}
}

// ---- nested macro expansion ----
//
// Regression suite for the macro-as-argument break that ClickHouse dashboards
// hit when the datasource adopted macropro: a macro passed as an argument to
// another macro (e.g. $__timeInterval($__fromTime)) must be expanded before
// the outer handler is invoked. The single-pass scanner used to jump over the
// whole outer call, so the inner token reached the database verbatim.
// Semantics: arguments are expanded innermost-first; handler OUTPUT is never
// rescanned.

// nestedTestMacros returns a MacroMap mirroring the clickhouse-datasource
// shape that triggered the regression: a zero-arg time macro nested inside a
// column-wrapping macro.
func nestedTestMacros() MacroMap[struct{}] {
	return MacroMap[struct{}]{
		"fromTime": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "toDateTime(1752580800)", nil
		},
		"timeInterval": func(_ QueryContext[struct{}], args []string) (string, error) {
			if len(args) != 1 {
				return "", errorf("need 1 arg, got %d", len(args))
			}
			return "toStartOfInterval(toDateTime(" + args[0] + "), INTERVAL 60 second)", nil
		},
	}
}

func TestInterpolate_nestedMacroArg(t *testing.T) {
	got, err := Interpolate("SELECT $__timeInterval($__fromTime) AS from_time", nestedTestMacros(), QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "SELECT toStartOfInterval(toDateTime(toDateTime(1752580800)), INTERVAL 60 second) AS from_time"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInterpolate_nestedMacroWithArgs(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + strings.Join(args, "|") + "]", nil
		},
		"lower": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "lower(" + args[0] + ")", nil
		},
	}
	got, err := Interpolate("$__wrap($__lower(col))", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[lower(col)]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedMacroDeep(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"leaf": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "LEAF", nil
		},
	}
	got, err := Interpolate("$__wrap($__wrap($__wrap($__leaf)))", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[[[LEAF]]]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedMacroInExpression(t *testing.T) {
	// The nested macro sits inside a larger SQL expression within the
	// argument; the surrounding expression text must be preserved.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"from": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1000", nil
		},
	}
	got, err := Interpolate("$__wrap(toDateTime($__from) + 5)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[toDateTime(1000) + 5]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedMacroMultiplePerArg(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
	}
	got, err := Interpolate("$__wrap($__x + $__x)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[1 + 1]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedMacroSecondArg(t *testing.T) {
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"pair": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
	}
	_, err := Interpolate("$__pair(a, $__x)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "a" || gotArgs[1] != "1" {
		t.Errorf("args: got %v, want [a 1]", gotArgs)
	}
}

func TestInterpolate_nestedExpansionDoesNotChangeArity(t *testing.T) {
	// Argument boundaries are fixed BEFORE nested expansion: an expansion
	// containing a comma must not re-split the argument list. (The old
	// sqlutil engine could re-split here depending on pass order; fixed
	// boundaries are a deliberate choice, not a compatibility bug.)
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"outer": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
		"pointXY": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "3, 4", nil
		},
	}
	_, err := Interpolate("$__outer($__pointXY)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "3, 4" {
		t.Errorf("args: got %v, want single arg %q", gotArgs, "3, 4")
	}
}

func TestInterpolate_nestedUnknownMacroPassedThrough(t *testing.T) {
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"known": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
	}
	got, err := Interpolate("$__known($__unknown)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "$__unknown" {
		t.Errorf("args: got %v, want [$__unknown]", gotArgs)
	}
	if got != "OK" {
		t.Errorf("got %q, want %q", got, "OK")
	}
}

func TestInterpolate_nestedHandlerErrorReturnsOriginal(t *testing.T) {
	macros := MacroMap[struct{}]{
		"outer": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "should not be reached", nil
		},
		"bad": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "", errorf("inner failure")
		},
	}
	// Includes a comment so the unstripped-original guarantee is observable.
	input := "SELECT 1 -- keep me\n$__outer($__bad)"
	got, err := Interpolate(input, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error from nested handler, got nil")
	}
	if !strings.Contains(err.Error(), "$__bad") {
		t.Errorf("error should name the failing inner macro; got %v", err)
	}
	if got != input {
		t.Errorf("on nested error, must return input verbatim\ngot  %q\nwant %q", got, input)
	}
}

func TestInterpolate_nestedHandlerOutputNotRescanned(t *testing.T) {
	// Handler OUTPUT is spliced verbatim, never rescanned — a handler that
	// emits a macro token cannot trigger further expansion. This bounds the
	// work to the input text and closes the door on expansion loops. It is a
	// DELIBERATE divergence from the old sqlutil engine, whose sequential
	// per-name passes could expand handler output depending on pass order;
	// that order-dependent behaviour is intentionally not preserved.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"emitsMacro": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "$__wrap(x)", nil
		},
	}
	got, err := Interpolate("$__wrap($__emitsMacro)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[$__wrap(x)]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedDepthLimit(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
	}
	const depth = 200
	input := strings.Repeat("$__wrap(", depth) + "x" + strings.Repeat(")", depth)
	got, err := Interpolate(input, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected depth-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "nesting") {
		t.Errorf("error should mention nesting depth; got %v", err)
	}
	if got != input {
		t.Errorf("on depth error, must return input verbatim")
	}
}

func TestInterpolate_nestedWithCustomPrefix(t *testing.T) {
	macros := MacroMap[struct{}]{
		"windowPeriod": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "5m", nil
		},
		"aggregate": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "aggregateWindow(every: " + args[0] + ")", nil
		},
	}
	got, err := Interpolate("v.aggregate(v.windowPeriod)", macros, QueryContext[struct{}]{}, WithPrefix("v."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "aggregateWindow(every: 5m)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedZeroArgParens(t *testing.T) {
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"now": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "NOW()", nil
		},
	}
	got, err := Interpolate("$__wrap($__now())", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "[NOW()]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedInsideStringLiteralArg(t *testing.T) {
	// The top-level scanner expands macro tokens inside string literals;
	// nested expansion must behave identically for consistency.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "X", nil
		},
	}
	got, err := Interpolate("$__wrap('a $__x b')", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "['a X b']"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedCallInCommentNotExpanded(t *testing.T) {
	// Comment stripping happens before any expansion, so a whole nested call
	// hidden behind -- must not evaluate either handler.
	outerCalled, innerCalled := false, false
	macros := MacroMap[struct{}]{
		"outer": func(_ QueryContext[struct{}], _ []string) (string, error) {
			outerCalled = true
			return "OUTER", nil
		},
		"inner": func(_ QueryContext[struct{}], _ []string) (string, error) {
			innerCalled = true
			return "INNER", nil
		},
	}
	_, err := Interpolate("SELECT 1 -- $__outer($__inner)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outerCalled || innerCalled {
		t.Errorf("handlers called for commented-out nested call: outer=%v inner=%v", outerCalled, innerCalled)
	}
}

func TestInterpolate_knownMacroInsideUnknownMacroArgs(t *testing.T) {
	// The unknown-macro branch copies only prefix+name and keeps scanning, so
	// an argument list after an unknown macro is ordinary text and known
	// macros inside it still expand — in both directions of nesting.
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"known": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "K", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
	}
	got, err := Interpolate("$__unknown($__known)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "$__unknown(K)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	_, err = Interpolate("$__known($__unknown($__x))", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "$__unknown(1)" {
		t.Errorf("args: got %v, want [$__unknown(1)]", gotArgs)
	}
}

func TestInterpolate_nestedMacrosInMultipleArgs(t *testing.T) {
	// Nested macros in two DIFFERENT args of the same call — pins that the
	// per-arg expansion loop covers every argument, not just the first or
	// last one containing the prefix.
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"pair": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) { return "1", nil },
		"y": func(_ QueryContext[struct{}], _ []string) (string, error) { return "2", nil },
	}
	_, err := Interpolate("$__pair($__x, $__y)", macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "1" || gotArgs[1] != "2" {
		t.Errorf("args: got %v, want [1 2]", gotArgs)
	}
}

func TestInterpolate_nestedDepthLimitBoundary(t *testing.T) {
	// Pins the exact boundary: the innermost of 129 nested calls is processed
	// at recursion depth 128 (the deepest allowed), while 130 levels error.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
	}
	deep := func(n int) string {
		return strings.Repeat("$__wrap(", n) + "x" + strings.Repeat(")", n)
	}

	got, err := Interpolate(deep(129), macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("129 levels should be within the limit: %v", err)
	}
	if want := strings.Repeat("[", 129) + "x" + strings.Repeat("]", 129); got != want {
		t.Errorf("129-level expansion wrong: got %d bytes, want %d", len(got), len(want))
	}

	// The comment makes the return-original-unstripped guarantee observable
	// on the depth-error path.
	in := "-- boundary\n" + deep(130)
	got, err = Interpolate(in, macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("130 levels should exceed the limit")
	}
	if got != in {
		t.Errorf("on depth error, must return input verbatim")
	}
}

func TestInterpolate_wideShallowNestingUnderLimit(t *testing.T) {
	// maxNestingDepth bounds per-branch recursion depth, not a cumulative
	// expansion counter — many shallow nested calls side by side must all
	// expand, as in a realistic macro-heavy dashboard query.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
	}
	input := strings.Repeat("$__wrap($__wrap(x)) ", 300)
	got, err := Interpolate(input, macros, QueryContext[struct{}]{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := strings.Repeat("[[x]] ", 300); got != want {
		t.Errorf("wide shallow nesting mangled: got %d bytes, want %d", len(got), len(want))
	}
}

func TestInterpolate_nestedWithCommentsDisabled(t *testing.T) {
	// Recursion reuses the resolved options and never re-strips comments, so
	// with stripping disabled a nested call after comment-like text expands.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) { return "1", nil },
	}
	got, err := Interpolate("SELECT 1 -- $__wrap($__x)", macros, QueryContext[struct{}]{}, WithComments(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "SELECT 1 -- [1]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInterpolate_nestedErrorNamesArgumentPosition(t *testing.T) {
	// A nested handler failure is wrapped with the outer macro name and the
	// 1-based argument position.
	macros := MacroMap[struct{}]{
		"pair": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "OK", nil
		},
		"bad": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "", errorf("boom")
		},
	}
	_, err := Interpolate("$__pair(a, $__bad)", macros, QueryContext[struct{}]{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "argument 2") {
		t.Errorf("error should name the argument position; got %v", err)
	}
	if !strings.Contains(err.Error(), "$__pair") || !strings.Contains(err.Error(), "$__bad") {
		t.Errorf("error should name both macros; got %v", err)
	}
}

// ---- nested expansion is dialect-neutral ----
//
// macropro serves every SQL dialect (MySQL, Postgres, MSSQL, ClickHouse, …),
// so nested expansion must compose with each dialect's comment and quoting
// options rather than assume any one syntax. The recursion reuses the options
// resolved at the top level and never re-strips comments, which these tests
// pin down per dialect.

func TestInterpolate_nestedMySQLHashComment(t *testing.T) {
	// MySQL: # line comments hide a nested call; an uncommented nested call
	// on the same input still expands.
	hiddenCalled := false
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
		"hidden": func(_ QueryContext[struct{}], _ []string) (string, error) {
			hiddenCalled = true
			return "HIDDEN", nil
		},
	}
	query := "SELECT $__wrap($__x) FROM t # $__wrap($__hidden)"
	got, err := Interpolate(query, macros, QueryContext[struct{}]{},
		WithComments(LineComment|BlockComment|HashComment|BackslashEscape))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hiddenCalled {
		t.Error("nested call behind # comment was expanded")
	}
	// Pin the full output: the stripped comment region is length-preserving
	// spaces, so no macro text may leak into the tail.
	want := "SELECT [1] FROM t " + strings.Repeat(" ", len("# $__wrap($__hidden)"))
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInterpolate_nestedMSSQLBracketIdentifierArg(t *testing.T) {
	// T-SQL: a bracket-quoted identifier alongside a nested macro must reach
	// the handler verbatim while the macro expands.
	var gotArgs []string
	macros := MacroMap[struct{}]{
		"pair": func(_ QueryContext[struct{}], args []string) (string, error) {
			gotArgs = args
			return "OK", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
	}
	_, err := Interpolate("$__pair([col name], $__x)", macros, QueryContext[struct{}]{},
		WithComments(LineComment|BlockComment|BracketQuote))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "[col name]" || gotArgs[1] != "1" {
		t.Errorf("args: got %v, want [[col name] 1]", gotArgs)
	}
}

func TestInterpolate_nestedPostgresDollarQuote(t *testing.T) {
	// Postgres: a dollar-quoted body containing comment-like text must
	// survive intact while a nested macro outside it expands.
	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
	}
	query := "SELECT $func$ -- not a comment $func$, $__wrap($__x)"
	got, err := Interpolate(query, macros, QueryContext[struct{}]{},
		WithComments(LineComment|BlockComment|DollarQuote))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "SELECT $func$ -- not a comment $func$, [1]"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
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

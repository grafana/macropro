package macropro

import (
	"fmt"
	"maps"
	"strings"
)

// Interpolate scans query for all $__<name>(…) tokens whose names appear in
// macros, calls the corresponding MacroFunc, and splices the returned string
// back in place.
//
// Macro names are matched longest-first so that $__interval_ms is never
// incorrectly matched as $__interval.
//
// Standard SQL line comments (--) and block comments (/* */) are stripped
// before macro expansion so that a macro token placed inside a comment is
// never evaluated. Callers that also need MySQL-style hash comment stripping
// (#) should call [StripComments] with [HashComment] before calling Interpolate.
//
// The default "$__" prefix and SQL comment styles can be overridden via
// [WithPrefix] and [WithComments] — useful for non-SQL query languages like
// InfluxDB Flux or for datasources with legacy macro syntaxes.
//
// Tokens whose names do not appear in macros are left unchanged. The first
// handler error is returned immediately; the original query string is
// returned alongside the error.
func Interpolate[T any](query string, macros MacroMap[T], ctx QueryContext[T], opts ...Option) (string, error) {
	if len(macros) == 0 {
		return query, nil
	}

	// Resolve options. Defaults: "$__" prefix, strip -- and /* */ comments.
	o := options{prefix: "$__", comments: LineComment | BlockComment}
	for _, opt := range opts {
		opt(&o)
	}

	// Strip comments before macro expansion so that tokens inside comment
	// regions are never evaluated. This closes the attack vector where a macro
	// hidden behind a -- or /* */ comment still triggers side effects (e.g.
	// fill-mode activation leading to OOM via ResampleWideFrame). The stripped
	// copy is held in `work`; `query` retains the caller's original bytes so
	// they can be returned untouched on error.
	work := query
	if o.comments != 0 {
		// If the prefix never appears in the raw input, no amount of comment
		// stripping can make it appear — StripComments only blanks regions,
		// it never introduces prefix bytes. Skip stripping AND scanning.
		if !strings.Contains(query, o.prefix) {
			return query, nil
		}
		work = StripComments(work, o.comments)
	}

	// Second fast path: the prefix may have only appeared inside a comment
	// that StripComments just blanked out. If no prefix remains, there is
	// nothing to expand — return work without allocating a Builder.
	if !strings.Contains(work, o.prefix) {
		return work, nil
	}

	// Single forward scan: find each prefix occurrence, read the macro name
	// greedily at the natural word boundary, and dispatch via map lookup.
	// Greedy name-reading inherently prevents $__interval from matching inside
	// $__interval_ms — the scanner always consumes the longest valid name
	// before consulting the map, so no longest-first sort is required.
	prefix := o.prefix
	var b strings.Builder
	b.Grow(len(work))

	i := 0
	for i < len(work) {
		rel := strings.Index(work[i:], prefix)
		if rel < 0 {
			b.WriteString(work[i:])
			break
		}
		if rel > 0 {
			b.WriteString(work[i : i+rel])
			i += rel
		}

		// Read the macro name (identifier chars only).
		nameStart := i + len(prefix)
		nameEnd := nameStart
		for nameEnd < len(work) && isNameChar(work[nameEnd]) {
			nameEnd++
		}

		if nameStart == nameEnd {
			// Bare prefix with nothing that looks like a name after it —
			// emit the prefix verbatim and continue scanning.
			b.WriteString(prefix)
			i = nameStart
			continue
		}

		name := work[nameStart:nameEnd]
		fn, ok := macros[name]
		if !ok {
			// Unknown macro — copy prefix+name through and advance.
			b.WriteString(work[i:nameEnd])
			i = nameEnd
			continue
		}

		// Consume an optional argument list.
		raw := ""
		after := nameEnd
		if after < len(work) && work[after] == '(' {
			end, err := findClosingParen(work, after)
			if err == nil {
				raw = work[after+1 : end]
				after = end + 1
			}
			// If parens are unbalanced, fall through as a zero-arg call.
		}

		args, err := parseArgs(raw)
		if err != nil {
			return query, fmt.Errorf("macro %s%s: %w", prefix, name, err)
		}
		replacement, err := callHandler(fn, ctx, args)
		if err != nil {
			return query, fmt.Errorf("macro %s%s(%s): %w", prefix, name, strings.Join(args, ", "), err)
		}
		b.WriteString(replacement)
		i = after
	}

	return b.String(), nil
}

// MergeMacros returns a new MacroMap with every entry from base, with entries
// from overrides taking precedence for identical names.
func MergeMacros[T any](base, overrides MacroMap[T]) MacroMap[T] {
	merged := make(MacroMap[T], len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)
	return merged
}

// isNameChar reports whether b is a valid macro name character ([_a-zA-Z0-9]).
func isNameChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// findClosingParen returns the index of the ')' that closes the '(' at
// position open in s, tracking bracket depth and respecting single- and
// double-quoted strings.
func findClosingParen(s string, open int) (int, error) {
	depth := 0
	for i := open; i < len(s); {
		c := s[i]
		switch c {
		case '\'', '"':
			i = scanStringLiteral(s, i)
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
		i++
	}
	return -1, fmt.Errorf("unbalanced parentheses")
}

// parseArgs splits the raw argument string (the content between the outer
// parentheses) by comma, trimming whitespace from each argument. Commas
// inside nested parentheses or string literals are not treated as separators.
// Returns an empty slice for a blank raw string (zero-arg macro call).
func parseArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}

	var args []string
	depth := 0
	start := 0

	for i := 0; i < len(raw); {
		c := raw[i]
		switch c {
		case '\'', '"':
			i = scanStringLiteral(raw, i)
			continue
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, fmt.Errorf("unexpected ')' at position %d", i)
			}
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(raw[start:i]))
				start = i + 1
			}
		}
		i++
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced parentheses in arguments")
	}
	args = append(args, strings.TrimSpace(raw[start:]))
	return args, nil
}

// scanStringLiteral advances past a SQL-style quoted string literal starting
// at position pos in s, where s[pos] is either a single or double quote.
// Returns the index immediately after the closing quote. Recognises doubled-
// quote escapes ('' within a single-quoted string, "" within a double-quoted
// string). If the literal is unterminated, returns len(s).
//
// This helper is the single source of truth for string-literal boundaries
// across the parser and [StripComments]. It does NOT recognise backslash
// escapes — MySQL and a handful of other dialects accept \' and \", but the
// SQL standard does not. Callers relying on backslash escapes should
// pre-sanitise or disable comment stripping via [WithComments](0).
// callHandler invokes fn(ctx, args) with a deferred recover so that a panic
// inside a MacroFunc is converted to an error rather than propagating out of
// [Interpolate]. A handler is arbitrary user code and the library treats it
// as a trust boundary — a single buggy or malicious handler should not be
// able to crash the caller.
func callHandler[T any](fn MacroFunc[T], ctx QueryContext[T], args []string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panicked: %v", r)
		}
	}()
	return fn(ctx, args)
}

// scanToLineEnd returns the index of the first line terminator at or after
// start in s, or len(s) if none is found. Both \n and \r are recognised, so
// classic-Mac \r-only line endings terminate line comments correctly without
// over-stripping content that follows. The returned index points AT the
// terminator; the caller is responsible for emitting or preserving it.
func scanToLineEnd(s string, start int) int {
	j := start
	for j < len(s) && s[j] != '\n' && s[j] != '\r' {
		j++
	}
	return j
}

func scanStringLiteral(s string, pos int) int {
	quote := s[pos]
	j := pos + 1
	for j < len(s) {
		if s[j] == quote {
			if j+1 < len(s) && s[j+1] == quote {
				// Doubled-quote escape — keep going.
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return j // unterminated — treat rest of input as string content
}

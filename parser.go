package macropro

import (
	"fmt"
	"maps"
	"sort"
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
	// fill-mode activation leading to OOM via ResampleWideFrame).
	if o.comments != 0 {
		query = StripComments(query, o.comments)
	}

	// Build a sorted list of names, longest first, to prevent prefix collisions.
	names := make([]string, 0, len(macros))
	for name := range macros {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] < names[j]
	})

	result := query
	for _, name := range names {
		fn := macros[name]
		token := o.prefix + name

		var replaceErr error
		result = replaceAllMacro(result, token, func(raw string) string {
			if replaceErr != nil {
				return raw // already errored — leave remaining tokens alone
			}
			args, err := parseArgs(raw)
			if err != nil {
				replaceErr = fmt.Errorf("macro %s: %w", token, err)
				return raw
			}
			replacement, err := fn(ctx, args)
			if err != nil {
				replaceErr = fmt.Errorf("macro %s(%s): %w", token, strings.Join(args, ", "), err)
				return raw
			}
			return replacement
		})

		if replaceErr != nil {
			return query, replaceErr
		}
	}
	return result, nil
}

// MergeMacros returns a new MacroMap with every entry from base, with entries
// from overrides taking precedence for identical names.
func MergeMacros[T any](base, overrides MacroMap[T]) MacroMap[T] {
	merged := make(MacroMap[T], len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)
	return merged
}

// replaceAllMacro finds all occurrences of token (possibly followed by a
// parenthesised argument list) in s and calls fn with the raw argument string
// (empty string if no parens found). fn returns the replacement text.
func replaceAllMacro(s, token string, fn func(raw string) string) string {
	var b strings.Builder
	for {
		idx := strings.Index(s, token)
		if idx == -1 {
			b.WriteString(s)
			break
		}

		// Ensure this is a complete name match and not the prefix of a longer macro.
		after := idx + len(token)
		if after < len(s) && isNameChar(s[after]) {
			b.WriteString(s[:after])
			s = s[after:]
			continue
		}

		b.WriteString(s[:idx])
		s = s[after:]

		// Consume an optional argument list.
		raw := ""
		if len(s) > 0 && s[0] == '(' {
			end, err := findClosingParen(s, 0)
			if err == nil {
				raw = s[1:end] // content between the parens
				s = s[end+1:] // advance past the closing paren
			}
			// If parens are unbalanced, treat as zero-arg and leave s alone.
		}
		b.WriteString(fn(raw))
	}
	return b.String()
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

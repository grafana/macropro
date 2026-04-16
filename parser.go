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
	inSingle := false
	inDouble := false

	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case inSingle || inDouble:
			// Inside a string literal — skip everything.
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
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
	inSingle := false
	inDouble := false
	start := 0

	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case inSingle || inDouble:
			// Inside a string literal — skip.
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return nil, fmt.Errorf("unexpected ')' at position %d", i)
			}
			depth--
		case c == ',' && depth == 0:
			args = append(args, strings.TrimSpace(raw[start:i]))
			start = i + 1
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced parentheses in arguments")
	}
	args = append(args, strings.TrimSpace(raw[start:]))
	return args, nil
}

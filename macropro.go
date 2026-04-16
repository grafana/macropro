// Package macropro provides a generic, language-agnostic engine for parsing
// and expanding Grafana macros in query strings.
//
// Macros have the form $__<name> or $__<name>(arg1, arg2, …). Any datasource
// backend can register its own handlers and call [Interpolate] to rewrite
// queries at runtime.
//
// # Quick start
//
//	type MyExtra struct{ Database string }
//
//	overrides := macropro.MacroMap[MyExtra]{
//	    "database": func(ctx macropro.QueryContext[MyExtra], _ []string) (string, error) {
//	        return ctx.Extra.Database, nil
//	    },
//	}
//
//	macros := macropro.MergeMacros(macropro.DefaultMacros[MyExtra](), overrides)
//
//	result, err := macropro.Interpolate(query, macros, ctx)
//
// # Comment stripping
//
// [Interpolate] automatically strips standard SQL line comments (--) and block
// comments (/* */) before expanding macros, so macro tokens hidden inside
// comments are never evaluated. Callers using MySQL, which also supports #
// as a line-comment delimiter, should pre-strip hash comments:
//
//	clean := macropro.StripComments(query, macropro.HashComment)
//	result, err := macropro.Interpolate(clean, macros, ctx)
package macropro

import "strings"

// StripComments removes SQL comment regions from query according to the
// requested CommentStyle bitmask, while preserving single-quoted and
// double-quoted string literals (and optionally PostgreSQL dollar-quoted
// strings).
//
// The returned string has the same byte length in comment regions replaced by
// spaces, so that line/column positions in error messages remain accurate.
func StripComments(query string, style CommentStyle) string {
	var b strings.Builder
	b.Grow(len(query))

	i := 0
	for i < len(query) {
		// Respect single-quoted strings.
		if query[i] == '\'' {
			j := i + 1
			for j < len(query) {
				if query[j] == '\'' {
					j++
					if j < len(query) && query[j] == '\'' {
						// Escaped quote '' — keep going.
						j++
						continue
					}
					break
				}
				j++
			}
			b.WriteString(query[i:j])
			i = j
			continue
		}

		// Respect double-quoted identifiers.
		if query[i] == '"' {
			j := i + 1
			for j < len(query) {
				if query[j] == '"' {
					j++
					if j < len(query) && query[j] == '"' {
						j++
						continue
					}
					break
				}
				j++
			}
			b.WriteString(query[i:j])
			i = j
			continue
		}

		// PostgreSQL dollar-quoted strings: $tag$…$tag$
		if style&DollarQuote != 0 && query[i] == '$' {
			if tag, end := parseDollarQuote(query, i); end >= 0 {
				b.WriteString(query[i:end])
				i = end
				_ = tag
				continue
			}
		}

		// Line comments: -- …\n
		if style&LineComment != 0 && i+1 < len(query) && query[i] == '-' && query[i+1] == '-' {
			j := i
			for j < len(query) && query[j] != '\n' {
				j++
			}
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Slash line comments: // …\n  (Flux-style)
		if style&SlashComment != 0 && i+1 < len(query) && query[i] == '/' && query[i+1] == '/' {
			j := i
			for j < len(query) && query[j] != '\n' {
				j++
			}
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Hash comments: # …\n  (MySQL-style)
		if style&HashComment != 0 && query[i] == '#' {
			j := i
			for j < len(query) && query[j] != '\n' {
				j++
			}
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Block comments: /* … */
		if style&BlockComment != 0 && i+1 < len(query) && query[i] == '/' && query[i+1] == '*' {
			j := i + 2
			for j+1 < len(query) {
				if query[j] == '*' && query[j+1] == '/' {
					j += 2
					break
				}
				j++
			}
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		b.WriteByte(query[i])
		i++
	}
	return b.String()
}

// parseDollarQuote detects a PostgreSQL dollar-quoted string starting at pos
// in s. Returns the tag and the end index (exclusive) after the closing tag,
// or ("", -1) if pos is not the start of a dollar-quoted string.
func parseDollarQuote(s string, pos int) (tag string, end int) {
	if s[pos] != '$' {
		return "", -1
	}
	// Find the closing $ of the opening tag.
	j := pos + 1
	for j < len(s) && s[j] != '$' {
		c := s[j]
		if !isDollarTagChar(c) {
			return "", -1
		}
		j++
	}
	if j >= len(s) {
		return "", -1
	}
	openTag := s[pos : j+1] // includes both $
	// Search for the matching closing tag.
	rest := s[j+1:]
	idx := strings.Index(rest, openTag)
	if idx < 0 {
		return "", -1
	}
	return openTag, j + 1 + idx + len(openTag)
}

func isDollarTagChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func writeSpaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

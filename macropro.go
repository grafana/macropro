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
//
// When the input contains nothing that could start a comment or string
// literal, the original string is returned unchanged with no allocation.
func StripComments(query string, style CommentStyle) string {
	// Fast path: if no comment-stripping bit is set, there is nothing to do.
	// DollarQuote alone is a no-op because it only preserves regions that are
	// already copied verbatim.
	const anyCommentStyle = LineComment | BlockComment | HashComment | SlashComment
	if style&anyCommentStyle == 0 {
		return query
	}

	// Fast path: most real-world queries contain no comments and no string
	// literals. Scan for any byte that could trigger action under the current
	// style; if none is present, return the input unchanged. strings.IndexAny
	// is SIMD-accelerated on common platforms, so this is much cheaper than a
	// per-byte Go loop.
	if !strings.ContainsAny(query, stripNeedles(style)) {
		return query
	}

	var b strings.Builder
	b.Grow(len(query))

	i := 0
	for i < len(query) {
		// Respect single- and double-quoted strings (and PostgreSQL identifiers).
		// scanStringLiteral handles SQL-style doubled-quote escapes ('' and "").
		if query[i] == '\'' || query[i] == '"' {
			j := scanStringLiteral(query, i)
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

		// Line comments: -- …  (SQL-style)
		if style&LineComment != 0 && i+1 < len(query) && query[i] == '-' && query[i+1] == '-' {
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Slash line comments: // …  (Flux-style)
		if style&SlashComment != 0 && i+1 < len(query) && query[i] == '/' && query[i+1] == '/' {
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Hash comments: # …  (MySQL-style)
		if style&HashComment != 0 && query[i] == '#' {
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j
			continue
		}

		// Block comments: /* … */
		// If the closing */ is missing, the remainder of the input is blanked
		// through EOF so that a macro token hidden in an unterminated comment
		// cannot escape the stripper.
		if style&BlockComment != 0 && i+1 < len(query) && query[i] == '/' && query[i+1] == '*' {
			j := i + 2
			for j < len(query) {
				if j+1 < len(query) && query[j] == '*' && query[j+1] == '/' {
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

// stripNeedles returns the set of bytes that could start a comment under the
// given style. Quote characters are deliberately NOT included: if a query
// contains no comment starter, there is nothing to strip regardless of what
// lies inside string literals, so we can skip the full scan.
//
// The caller has already verified that at least one comment-stripping bit
// is set. DollarQuote is included because we must enter the scanner to
// preserve $tag$…$tag$ regions when other comment styles are active.
//
// Common style values return a constant so the hot path is allocation-free;
// unusual combinations fall through to a small builder.
func stripNeedles(style CommentStyle) string {
	switch style {
	case LineComment | BlockComment:
		return stripNeedlesSQL
	case LineComment | BlockComment | HashComment:
		return stripNeedlesMySQL
	case LineComment | BlockComment | DollarQuote:
		return stripNeedlesPostgres
	case LineComment | BlockComment | HashComment | DollarQuote:
		return stripNeedlesMySQLPG
	case SlashComment | BlockComment:
		return stripNeedlesFlux
	}

	var b strings.Builder
	b.Grow(4)
	if style&LineComment != 0 {
		b.WriteByte('-')
	}
	if style&(BlockComment|SlashComment) != 0 {
		b.WriteByte('/')
	}
	if style&HashComment != 0 {
		b.WriteByte('#')
	}
	if style&DollarQuote != 0 {
		b.WriteByte('$')
	}
	return b.String()
}

const (
	stripNeedlesSQL      = "-/"
	stripNeedlesMySQL    = "-/#"
	stripNeedlesPostgres = "-/$"
	stripNeedlesMySQLPG  = "-/#$"
	stripNeedlesFlux     = "/"
)

func isDollarTagChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func writeSpaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

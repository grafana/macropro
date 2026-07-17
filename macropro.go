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
//
// A trailing SQLCommenter (https://google.github.io/sqlcommenter/) attribution
// tag such as /*application='grafana',source='bi'*/ is preserved rather than
// stripped: [Interpolate] splits it off with [SplitTrailingSQLCommenter]
// before expansion and re-appends it verbatim, so query-tagging metadata
// reaches the database while macros inside the tag are still never evaluated.
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

	needles := scanNeedles(style)
	var b strings.Builder
	b.Grow(len(query))

	i := 0
	for i < len(query) {
		// Jump directly to the next byte that could trigger an action.
		// strings.IndexAny is SIMD-accelerated, so long runs of ordinary
		// bytes are copied as a single WriteString rather than one
		// WriteByte per character.
		rel := strings.IndexAny(query[i:], needles)
		if rel < 0 {
			b.WriteString(query[i:])
			break
		}
		if rel > 0 {
			b.WriteString(query[i : i+rel])
			i += rel
		}

		c := query[i]
		switch {
		case c == '\'' || c == '"':
			// Single- or double-quoted literal (handles '' and "" escapes,
			// plus backslash escapes when BackslashEscape is set).
			var j int
			if style&BackslashEscape != 0 {
				j = scanStringLiteralBackslash(query, i)
			} else {
				j = scanStringLiteral(query, i)
			}
			b.WriteString(query[i:j])
			i = j

		case style&BacktickQuote != 0 && c == '`':
			// MySQL backtick-quoted identifier. The doubled-backtick escape
			// has the same shape as '' / "", so scanStringLiteral handles it.
			j := scanStringLiteral(query, i)
			b.WriteString(query[i:j])
			i = j

		case style&BracketQuote != 0 && c == '[':
			// T-SQL bracket-quoted identifier, with ']]' as the closing escape.
			j := scanBracketIdentifier(query, i)
			b.WriteString(query[i:j])
			i = j

		case style&DollarQuote != 0 && c == '$':
			// PostgreSQL dollar-quoted string: $tag$…$tag$
			if _, end := parseDollarQuote(query, i); end >= 0 {
				b.WriteString(query[i:end])
				i = end
			} else {
				// Lone $ that is not a dollar-quote start — copy as-is.
				b.WriteByte(c)
				i++
			}

		case style&LineComment != 0 && c == '-' && i+1 < len(query) && query[i+1] == '-':
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j

		case style&SlashComment != 0 && c == '/' && i+1 < len(query) && query[i+1] == '/':
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j

		case style&BlockComment != 0 && c == '/' && i+1 < len(query) && query[i+1] == '*':
			// If the closing */ is missing, the remainder of the input is
			// blanked through EOF so that a macro token hidden in an
			// unterminated comment cannot escape the stripper.
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

		case style&HashComment != 0 && c == '#':
			j := scanToLineEnd(query, i)
			writeSpaces(&b, j-i)
			i = j

		default:
			// Byte matched the needle set but did not start any enabled
			// comment (e.g. a lone '-' with LineComment enabled but no
			// second '-', or a '/' that is neither /* nor //). Copy it.
			b.WriteByte(c)
			i++
		}
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
		return stripNeedlesLineBlock
	case LineComment | BlockComment | HashComment:
		return stripNeedlesLineBlockHash
	case LineComment | BlockComment | DollarQuote:
		return stripNeedlesLineBlockDollar
	case LineComment | BlockComment | HashComment | DollarQuote:
		return stripNeedlesLineBlockHashDollar
	case SlashComment | BlockComment:
		return stripNeedlesSlashBlock
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
	stripNeedlesLineBlock           = "-/"
	stripNeedlesLineBlockHash       = "-/#"
	stripNeedlesLineBlockDollar     = "-/$"
	stripNeedlesLineBlockHashDollar = "-/#$"
	stripNeedlesSlashBlock          = "/"
)

// scanNeedles returns the set of bytes that the full scanner must stop on:
// every byte from [stripNeedles] plus the two quote characters, since the
// scanner has to enter string-literal tracking mode when it sees a quote.
// This is the needle set passed to strings.IndexAny inside the main loop,
// allowing long runs of ordinary bytes to be copied as a single WriteString.
//
// Common style values return a constant so the hot path is allocation-free;
// unusual combinations fall through to a small builder.
func scanNeedles(style CommentStyle) string {
	switch style {
	case LineComment | BlockComment:
		return scanNeedlesLineBlock
	case LineComment | BlockComment | HashComment:
		return scanNeedlesLineBlockHash
	case LineComment | BlockComment | DollarQuote:
		return scanNeedlesLineBlockDollar
	case LineComment | BlockComment | HashComment | DollarQuote:
		return scanNeedlesLineBlockHashDollar
	case SlashComment | BlockComment:
		return scanNeedlesSlashBlock
	}

	var b strings.Builder
	b.Grow(8)
	b.WriteByte('\'')
	b.WriteByte('"')
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
	if style&BacktickQuote != 0 {
		b.WriteByte('`')
	}
	if style&BracketQuote != 0 {
		b.WriteByte('[')
	}
	return b.String()
}

const (
	scanNeedlesLineBlock           = "'\"-/"
	scanNeedlesLineBlockHash       = "'\"-/#"
	scanNeedlesLineBlockDollar     = "'\"-/$"
	scanNeedlesLineBlockHashDollar = "'\"-/#$"
	scanNeedlesSlashBlock          = "'\"/"
)

func isDollarTagChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func writeSpaces(b *strings.Builder, n int) {
	for range n {
		b.WriteByte(' ')
	}
}

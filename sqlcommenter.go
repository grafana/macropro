package macropro

import (
	"regexp"
	"strings"
)

// sqlCommenterRegExp validates a SQLCommenter attribution comment made of one
// or more key='value' pairs, e.g. /*application='grafana',source='bi'*/. Keys
// allow '%' for the URL-encoded serialisation; values allow escaped quotes
// (\') and any other byte. The tag is re-appended to the query verbatim and
// never interpolated, so the value charset does not need to defend against
// macros. The caller has already ensured the comment contains no internal */.
var sqlCommenterRegExp = regexp.MustCompile(`^/\*\s*[a-zA-Z0-9%_.-]+='(?:\\.|[^'\\])*'(\s*,\s*[a-zA-Z0-9%_.-]+='(?:\\.|[^'\\])*')*\s*\*/$`)

// SplitTrailingSQLCommenter splits a trailing SQLCommenter
// (https://google.github.io/sqlcommenter/) attribution tag off the end of
// query. It returns the query without the tag and the tag itself (including
// any trailing ';'), or the original query and an empty string when there is
// none.
//
// [Interpolate] calls this automatically whenever [BlockComment] stripping is
// enabled and re-appends the tag verbatim after expansion, so the tag reaches
// the database unchanged and no macro can complete across the comment
// boundary in either direction. It is exported for callers that pre-process
// queries with [StripComments] directly and need the same protection.
//
// style selects which line-comment markers the target dialect recognises:
// [LineComment] "--", [HashComment] "#", [SlashComment] "//". A tag-shaped
// block inside a trailing line comment is not executable SQL and must not be
// revived, so if the text before the tag on its own line contains an enabled
// marker the query is returned untouched. Only set styles the engine actually
// treats as comments: '#' is ordinary syntax in T-SQL (#temp tables) and
// PostgreSQL (#> JSON operators), and enabling [HashComment] there would drop
// valid tags.
func SplitTrailingSQLCommenter(query string, style CommentStyle) (string, string) {
	trimmed := strings.TrimRight(query, " \t\r\n")
	for strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimRight(strings.TrimSuffix(trimmed, ";"), " \t\r\n")
	}
	if !strings.HasSuffix(trimmed, "*/") {
		return query, ""
	}
	open := strings.LastIndex(trimmed, "/*")
	if open < 0 {
		return query, ""
	}
	comment := trimmed[open:]
	if len(comment) < len("/**/") {
		return query, ""
	}
	// The block comment must be self-contained: an internal */ means the
	// database would close the comment early, leaving executable text that
	// must not be moved.
	if strings.Contains(comment[2:len(comment)-2], "*/") {
		return query, ""
	}
	if !sqlCommenterRegExp.MatchString(comment) {
		return query, ""
	}
	// Check the text before the tag on its own line for an enabled
	// line-comment marker. This is conservative: a marker inside a string
	// literal on that line also prevents splitting, which just falls back to
	// the tag being stripped like any other comment.
	line := trimmed[strings.LastIndexByte(trimmed[:open], '\n')+1 : open]
	if style&LineComment != 0 && strings.Contains(line, "--") {
		return query, ""
	}
	if style&HashComment != 0 && strings.Contains(line, "#") {
		return query, ""
	}
	if style&SlashComment != 0 && strings.Contains(line, "//") {
		return query, ""
	}
	return query[:open], query[open:]
}

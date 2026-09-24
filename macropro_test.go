package macropro

import (
	"strings"
	"testing"
)

// blankRange returns q with the byte range [lo,hi) replaced by spaces.
// Used to build expected output for length-preserving StripComments tests.
func blankRange(q string, lo, hi int) string {
	return q[:lo] + strings.Repeat(" ", hi-lo) + q[hi:]
}

func TestStripComments_lineComment(t *testing.T) {
	q := "SELECT 1 -- this is a comment\nFROM t"
	got := StripComments(q, LineComment)
	// Length must be preserved; comment region replaced with spaces.
	if len(got) != len(q) {
		t.Fatalf("length changed: got %d, want %d", len(got), len(q))
	}
	if got[:9] != "SELECT 1 " {
		t.Errorf("prefix wrong: %q", got[:9])
	}
	suffix := got[9+20:] // skip "SELECT 1 " + 20 replaced chars
	if suffix != "\nFROM t" {
		t.Errorf("suffix wrong: %q", suffix)
	}
}

func TestStripComments_blockComment(t *testing.T) {
	q := "SELECT /* comment */ 1"
	got := StripComments(q, BlockComment)
	// Length must be preserved; /* comment */ (13 chars) replaced with spaces.
	if len(got) != len(q) {
		t.Fatalf("length changed: got %d, want %d", len(got), len(q))
	}
	if got[:7] != "SELECT " {
		t.Errorf("prefix wrong: %q", got[:7])
	}
	if got[7+13:] != " 1" {
		t.Errorf("suffix wrong: %q", got[7+13:])
	}
}

func TestStripComments_blockCommentDoesNotNestByDefault(t *testing.T) {
	// Without NestedBlockComment the first */ closes the comment, so " c */ 1"
	// survives as code. This is the MySQL and Oracle reading.
	q := "SELECT /* a /* b */ c */ 1"
	got := StripComments(q, BlockComment)
	want := blankRange(q, 7, 19)
	if got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

func TestStripComments_nestedBlockCommentImpliesBlockComment(t *testing.T) {
	q := "SELECT /* comment */ 1"
	got := StripComments(q, NestedBlockComment)
	want := blankRange(q, 7, 20)
	if got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

func TestStripComments_nestedBlockCommentDepth(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			"three levels",
			"SELECT /* a /* b /* c */ d */ e */ 1",
			blankRange("SELECT /* a /* b /* c */ d */ e */ 1", 7, 34),
		},
		{
			// Quotes carry no meaning inside a comment, so this /* opens a
			// level that is never closed and the rest is blanked through EOF.
			"quoted /* inside a comment still opens a level",
			"SELECT /* '/*' */ 1",
			blankRange("SELECT /* '/*' */ 1", 7, 19),
		},
		{
			"adjacent comments tracked independently",
			"SELECT /* a /* b */ */ 1 /* c */ 2",
			blankRange(blankRange("SELECT /* a /* b */ */ 1 /* c */ 2", 7, 22), 25, 32),
		},
		{
			// The markers overlap by one byte, so the scanner must not reuse
			// the '*' of an opener as the '*' of a closer.
			"lone /*/ stays open",
			"SELECT /*/ 1",
			blankRange("SELECT /*/ 1", 7, 12),
		},
		{
			"/*/*/ opens twice and closes none",
			"SELECT /*/*/ 1",
			blankRange("SELECT /*/*/ 1", 7, 14),
		},
		{
			"stray closer after a complete comment is left alone",
			"SELECT /**/*/ 1",
			blankRange("SELECT /**/*/ 1", 7, 11),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripComments(tc.input, LineComment|NestedBlockComment)
			if got != tc.want {
				t.Errorf("\ninput %q\ngot   %q\nwant  %q", tc.input, got, tc.want)
			}
		})
	}
}

// stripNeedles answers every StripComments call, so a style that misses its
// constant fast path allocates a builder on each query. The dialect recipes in
// the README must stay on the constant path.
func TestStripComments_recommendedStylesAreAllocationFree(t *testing.T) {
	q := "SELECT id, name FROM users WHERE tenant = 42"
	styles := []struct {
		name  string
		style CommentStyle
	}{
		{"generic", LineComment | BlockComment},
		{"generic nested", LineComment | NestedBlockComment},
		{"postgresql", LineComment | NestedBlockComment | DollarQuote},
		{"flux", SlashComment | BlockComment},
	}
	for _, s := range styles {
		t.Run(s.name, func(t *testing.T) {
			var sink string
			got := testing.AllocsPerRun(100, func() { sink = StripComments(q, s.style) })
			if got != 0 {
				t.Errorf("StripComments allocated %v times, want 0 (sink %q)", got, sink)
			}
		})
	}
}

func TestStripComments_hashComment(t *testing.T) {
	q := "SELECT 1 # comment\nFROM t"
	got := StripComments(q, HashComment)
	want := "SELECT 1          \nFROM t"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestStripComments_preservesSingleQuotedString(t *testing.T) {
	// The -- inside a string literal must NOT be stripped.
	q := `SELECT '--not a comment' AS s`
	got := StripComments(q, LineComment|BlockComment)
	if got != q {
		t.Errorf("string literal was modified: got %q", got)
	}
}

func TestStripComments_preservesDoubleQuotedIdentifier(t *testing.T) {
	q := `SELECT "col--name" FROM t`
	got := StripComments(q, LineComment)
	if got != q {
		t.Errorf("double-quoted identifier was modified: got %q", got)
	}
}

func TestStripComments_dollarQuote(t *testing.T) {
	// $$ ... $$ is a PostgreSQL dollar-quoted string — its content is NOT a comment.
	q := "SELECT $$ -- not a comment $$ AS s"
	got := StripComments(q, LineComment|DollarQuote)
	// The dollar-quoted region should be preserved, so the -- inside stays.
	if got != q {
		t.Errorf("dollar-quoted string was modified: got %q", got)
	}
}

func TestStripComments_lineCommentNotStrippedWhenStyleOmitted(t *testing.T) {
	q := "SELECT 1 -- comment\nFROM t"
	got := StripComments(q, BlockComment) // LineComment not set
	if got != q {
		t.Errorf("line comment stripped when not requested: got %q", got)
	}
}

func TestStripComments_preservesLength(t *testing.T) {
	q := "SELECT /* hello */ 1"
	got := StripComments(q, BlockComment)
	if len(got) != len(q) {
		t.Errorf("StripComments changed string length: got %d, want %d", len(got), len(q))
	}
}

func TestStripComments_thenInterpolate(t *testing.T) {
	// Macros inside comments should not be expanded after stripping.
	q := "SELECT 1 -- $__interval\nFROM t"
	macros := DefaultMacros[struct{}]()
	ctx := QueryContext[struct{}]{Interval: 5e9} // 5 seconds

	clean := StripComments(q, LineComment)
	got, err := Interpolate(clean, macros, ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The $__interval inside the comment region becomes spaces, so it won't match.
	if containsMacro(got) {
		t.Errorf("macro inside comment was unexpectedly expanded: %q", got)
	}
}

func containsMacro(s string) bool {
	for i := 0; i+3 < len(s); i++ {
		if s[i] == '$' && s[i+1] == '_' && s[i+2] == '_' {
			return true
		}
	}
	return false
}

// TestStripComments_dialectCases covers the quote-aware comment-stripping
// scenarios from grafana/grafana PR #121535 (MySQL) and PR #121772
// (PostgreSQL, MSSQL). Each case asserts that StripComments either strips
// the intended comment region OR leaves quoted regions verbatim.
//
// macropro.StripComments is length-preserving: stripped regions become runs
// of spaces of equal byte length, so `want` is built from `input` by blanking
// a specific byte range.
func TestStripComments_dialectCases(t *testing.T) {
	// Standard comment-stripping (PostgreSQL- and MSSQL-compatible subset).
	t.Run("standard", func(t *testing.T) {
		style := LineComment | BlockComment
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{
				"line comment stripped",
				"SELECT 1 -- a comment",
				blankRange("SELECT 1 -- a comment", 9, 21),
			},
			{
				"block comment stripped",
				"SELECT /* a comment */ 1",
				blankRange("SELECT /* a comment */ 1", 7, 22),
			},
			{
				"multiline block comment stripped",
				"SELECT /*\n  multiline\n  comment\n*/ 1",
				blankRange("SELECT /*\n  multiline\n  comment\n*/ 1", 7, 34),
			},
			{
				"line comment inside single-quoted string preserved",
				"SELECT '-- not a comment' AS label",
				"SELECT '-- not a comment' AS label",
			},
			{
				"block comment inside single-quoted string preserved",
				"SELECT '/* not a comment */' AS label",
				"SELECT '/* not a comment */' AS label",
			},
			{
				"line comment inside double-quoted identifier preserved",
				`SELECT "col -- name" FROM t`,
				`SELECT "col -- name" FROM t`,
			},
			{
				"block comment inside double-quoted identifier preserved",
				`SELECT "col /* name */" FROM t`,
				`SELECT "col /* name */" FROM t`,
			},
			{
				"doubled-quote escape inside single-quoted string",
				"SELECT 'it''s fine -- not a comment' AS v",
				"SELECT 'it''s fine -- not a comment' AS v",
			},
			{
				"doubled-quote escape inside double-quoted identifier",
				`SELECT "col ""-- name""" FROM t`,
				`SELECT "col ""-- name""" FROM t`,
			},
			{
				"mixed: -- inside string then real -- comment",
				"SELECT '-- in string' AS a -- real comment",
				blankRange("SELECT '-- in string' AS a -- real comment", 27, 42),
			},
			{
				"mixed: block comment inside string then real block comment",
				"SELECT '/* in string */' AS a /* real comment */",
				blankRange("SELECT '/* in string */' AS a /* real comment */", 30, 48),
			},
			{
				"no-op: query with no comments",
				"SELECT col FROM t WHERE col > 1",
				"SELECT col FROM t WHERE col > 1",
			},
			{
				"newline after line comment preserved",
				"SELECT 1 -- comment\nFROM t",
				blankRange("SELECT 1 -- comment\nFROM t", 9, 19),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := StripComments(tc.input, style)
				if got != tc.want {
					t.Errorf("\ninput %q\ngot   %q\nwant  %q", tc.input, got, tc.want)
				}
			})
		}
	})

	// PostgreSQL: adds dollar-quoted strings to the standard set.
	t.Run("postgresql", func(t *testing.T) {
		style := LineComment | NestedBlockComment | DollarQuote
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{
				"nested block comment closes at the outer */",
				"SELECT /* a /* b */ c */ 1",
				blankRange("SELECT /* a /* b */ c */ 1", 7, 24),
			},
			{
				"line comment inside empty dollar-quoted string preserved",
				"SELECT $$ -- not a comment $$",
				"SELECT $$ -- not a comment $$",
			},
			{
				"block comment inside empty dollar-quoted string preserved",
				"SELECT $$ /* not a comment */ $$",
				"SELECT $$ /* not a comment */ $$",
			},
			{
				"line comment inside tagged dollar-quoted string preserved",
				"SELECT $body$ -- not a comment $body$",
				"SELECT $body$ -- not a comment $body$",
			},
			{
				"grafana macro not confused with dollar-quote",
				"SELECT $__timeFrom() -- comment",
				blankRange("SELECT $__timeFrom() -- comment", 21, 31),
			},
			{
				"positional parameter not confused with dollar-quote",
				"SELECT $1 -- comment",
				blankRange("SELECT $1 -- comment", 10, 20),
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := StripComments(tc.input, style)
				if got != tc.want {
					t.Errorf("\ninput %q\ngot   %q\nwant  %q", tc.input, got, tc.want)
				}
			})
		}
	})

	// MySQL: adds hash-comment, backtick identifiers, and backslash escapes.
	t.Run("mysql", func(t *testing.T) {
		style := LineComment | BlockComment | HashComment | BacktickQuote | BackslashEscape
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{
				"strips hash comments",
				"SELECT 1 # hash comment\nFROM t",
				blankRange("SELECT 1 # hash comment\nFROM t", 9, 23),
			},
			{
				// MySQL does not nest block comments, so the first */ closes
				// the comment and " c */ 1" is code the server would execute.
				"block comment ends at the first */",
				"SELECT /* a /* b */ c */ 1",
				blankRange("SELECT /* a /* b */ c */ 1", 7, 19),
			},
			{
				"preserves # inside single-quoted string",
				`SELECT JSON_UNQUOTE(JSON_EXTRACT(t.properties, '$."Claim #"')) AS claim`,
				`SELECT JSON_UNQUOTE(JSON_EXTRACT(t.properties, '$."Claim #"')) AS claim`,
			},
			{
				"preserves # inside double-quoted string",
				`SELECT "col#name" FROM t`,
				`SELECT "col#name" FROM t`,
			},
			{
				"preserves # inside backtick-quoted identifier",
				"SELECT `Claim #` FROM t",
				"SELECT `Claim #` FROM t",
			},
			{
				"handles backslash-escaped quote inside string",
				`SELECT 'it\'s a #test' FROM t`,
				`SELECT 'it\'s a #test' FROM t`,
			},
			{
				"handles doubled quotes inside strings",
				"SELECT 'it''s a #test' FROM t",
				"SELECT 'it''s a #test' FROM t",
			},
			{
				"real-world JSON path with hash regression",
				"SELECT\n  JSON_UNQUOTE(JSON_EXTRACT(t.properties, '$.\"Claim #\"')) AS `Claim Number`\nFROM repairshopr.tickets t\nWHERE t.status = 'Resolved'",
				"SELECT\n  JSON_UNQUOTE(JSON_EXTRACT(t.properties, '$.\"Claim #\"')) AS `Claim Number`\nFROM repairshopr.tickets t\nWHERE t.status = 'Resolved'",
			},
			{
				"hash comment after quoted string with hash",
				"SELECT 'Claim #' # this is a comment\nFROM t",
				blankRange("SELECT 'Claim #' # this is a comment\nFROM t", 17, 36),
			},
			{
				"doubled-backtick escape inside backtick identifier",
				"SELECT `col``name` FROM t",
				"SELECT `col``name` FROM t",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := StripComments(tc.input, style)
				if got != tc.want {
					t.Errorf("\ninput %q\ngot   %q\nwant  %q", tc.input, got, tc.want)
				}
			})
		}
	})

	// MSSQL: adds T-SQL bracket-quoted identifiers to the standard set.
	t.Run("mssql", func(t *testing.T) {
		style := LineComment | NestedBlockComment | BracketQuote
		cases := []struct {
			name  string
			input string
			want  string
		}{
			{
				"nested block comment closes at the outer */",
				"SELECT /* a /* b */ c */ 1",
				blankRange("SELECT /* a /* b */ c */ 1", 7, 24),
			},
			{
				"unterminated nested block comment blanked to EOF",
				"SELECT 1 /* a /* b */ FROM t",
				blankRange("SELECT 1 /* a /* b */ FROM t", 9, 28),
			},
			{
				"nested block comment inside bracket-quoted identifier preserved",
				"SELECT [col /* a /* b */ name] FROM t",
				"SELECT [col /* a /* b */ name] FROM t",
			},
			{
				"line comment inside bracket-quoted identifier preserved",
				"SELECT [col -- name] FROM t",
				"SELECT [col -- name] FROM t",
			},
			{
				"block comment inside bracket-quoted identifier preserved",
				"SELECT [col /* name */] FROM t",
				"SELECT [col /* name */] FROM t",
			},
			{
				"doubled-bracket escape inside bracket identifier",
				"SELECT [col]]name] FROM t",
				"SELECT [col]]name] FROM t",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := StripComments(tc.input, style)
				if got != tc.want {
					t.Errorf("\ninput %q\ngot   %q\nwant  %q", tc.input, got, tc.want)
				}
			})
		}
	})
}

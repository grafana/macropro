package macropro

import "testing"

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

package macropro

import (
	"strings"
	"testing"
)

// FuzzInterpolate exercises the recursive nested-expansion path with hostile
// inputs. Two invariants must hold for every input: Interpolate never panics
// (handler panics are recovered and surfaced as errors), and whenever an
// error is returned the caller's input comes back verbatim.
func FuzzInterpolate(f *testing.F) {
	seeds := []string{
		"$__wrap($__wrap(x))",
		strings.Repeat("$__wrap(", 130) + "x" + strings.Repeat(")", 130),
		"$__wrap('a $__x b')",
		"$__known($__unknown($__x))",
		"$__pair('a,b', $__x(",
		"SELECT 1 -- $__wrap($__x)",
		"$__wrap(COALESCE(a, b), 'it''s)('",
		"$__pair(a, $__bad)",
		"$__wrap($__bad)",
		"$__",
		"$__wrap(",
		"'unterminated $__x",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	macros := MacroMap[struct{}]{
		"wrap": func(_ QueryContext[struct{}], args []string) (string, error) {
			return "[" + args[0] + "]", nil // panics on zero args, which must be recovered
		},
		"x": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "1", nil
		},
		"known": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "K", nil
		},
		"pair": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "OK", nil
		},
		"bad": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "", errorf("always fails")
		},
	}

	f.Fuzz(func(t *testing.T, query string) {
		got, err := Interpolate(query, macros, QueryContext[struct{}]{})
		if err != nil && got != query {
			t.Errorf("on error, Interpolate must return input verbatim\ninput %q\ngot   %q\nerr   %v", query, got, err)
		}
	})
}

// FuzzStripComments exercises the byte scanner across every style combination.
// Two invariants must hold for every input: the output is the same byte length
// as the input, and each byte is either unchanged or blanked to a space. Both
// fail loudly on the index-arithmetic mistakes that comment and quote nesting
// invite.
func FuzzStripComments(f *testing.F) {
	seeds := []struct {
		query string
		style uint
	}{
		{"SELECT /* a /* b */ c */ 1", uint(LineComment | NestedBlockComment)},
		{"SELECT /* a /* b */ c */ 1", uint(LineComment | BlockComment)},
		{"/*", uint(NestedBlockComment)},
		{"/*/", uint(NestedBlockComment)},
		{"/*/*", uint(NestedBlockComment)},
		{"*/", uint(NestedBlockComment)},
		{"/* '/*' */", uint(NestedBlockComment)},
		{"SELECT [a /* b] -- c", uint(LineComment | NestedBlockComment | BracketQuote)},
		{"SELECT $$ /* $$ --", uint(LineComment | NestedBlockComment | DollarQuote)},
		{"SELECT `a /*` # b", uint(HashComment | NestedBlockComment | BacktickQuote)},
	}
	for _, s := range seeds {
		f.Add(s.query, s.style)
	}

	f.Fuzz(func(t *testing.T, query string, style uint) {
		got := StripComments(query, CommentStyle(style))
		if len(got) != len(query) {
			t.Fatalf("length not preserved: got %d, want %d\nstyle %d input %q", len(got), len(query), style, query)
		}
		for i := range len(got) {
			if got[i] != query[i] && got[i] != ' ' {
				t.Fatalf("byte %d became %q, want unchanged or blanked\nstyle %d input %q", i, got[i], style, query)
			}
		}
	})
}

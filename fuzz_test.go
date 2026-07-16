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
			return "[" + args[0] + "]", nil // panics on zero args; must be recovered
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

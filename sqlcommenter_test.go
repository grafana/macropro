package macropro

import (
	"strings"
	"testing"
)

func TestSplitTrailingSQLCommenter(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		style    CommentStyle
		wantBody string
		wantTag  string
	}{
		{
			name:     "trailing tag before terminator",
			sql:      "SELECT 1 AS value\n/*application='grafana',source='bi'*/;",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 AS value\n",
			wantTag:  "/*application='grafana',source='bi'*/;",
		},
		{
			name:     "trailing tag without terminator",
			sql:      "SELECT 1 /*application='grafana'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 ",
			wantTag:  "/*application='grafana'*/",
		},
		{
			name:     "url-encoded key and value",
			sql:      "SELECT 1 /*a%20b='%2Fparam*d'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 ",
			wantTag:  "/*a%20b='%2Fparam*d'*/",
		},
		{
			name:     "escaped quote in value",
			sql:      `SELECT 1 /*controller='it\'s'*/`,
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 ",
			wantTag:  `/*controller='it\'s'*/`,
		},
		{
			name:     "paren in value does not let a macro complete across the boundary",
			sql:      "SELECT $__timeFilter(t /*k='a)b'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT $__timeFilter(t ",
			wantTag:  "/*k='a)b'*/",
		},
		{
			name:     "value containing */ is not a self-contained tag",
			sql:      "SELECT 1 /*k='*/ DROP TABLE t --'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 /*k='*/ DROP TABLE t --'*/",
			wantTag:  "",
		},
		{
			name:     "overlapping opener and closer is not a tag",
			sql:      "SELECT 1 /*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 /*/",
			wantTag:  "",
		},
		{
			name:     "repeated trailing semicolons",
			sql:      "SELECT 1 /*app='grafana'*/;;",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 ",
			wantTag:  "/*app='grafana'*/;;",
		},
		{
			name:     "tag inside a trailing line comment is not revived",
			sql:      "SELECT 1\n-- note /*k='v'*/",
			style:    LineComment | BlockComment | HashComment,
			wantBody: "SELECT 1\n-- note /*k='v'*/",
			wantTag:  "",
		},
		{
			name:     "tag inside a trailing hash comment is not revived",
			sql:      "SELECT 1 # /*k='v'*/",
			style:    LineComment | BlockComment | HashComment,
			wantBody: "SELECT 1 # /*k='v'*/",
			wantTag:  "",
		},
		{
			name:     "tag inside a trailing slash comment is not revived",
			sql:      "from(bucket) // /*k='v'*/",
			style:    SlashComment | BlockComment,
			wantBody: "from(bucket) // /*k='v'*/",
			wantTag:  "",
		},
		{
			name:     "tag on its own line after a line comment is still split",
			sql:      "SELECT 1 -- note\n/*app='grafana'*/",
			style:    LineComment | BlockComment | HashComment,
			wantBody: "SELECT 1 -- note\n",
			wantTag:  "/*app='grafana'*/",
		},
		{
			name:     "hash is ordinary syntax when the style does not include it",
			sql:      "SELECT * FROM #tmp /*app='grafana'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT * FROM #tmp ",
			wantTag:  "/*app='grafana'*/",
		},
		{
			name:     "postgres json operator does not block the tag",
			sql:      "SELECT data#>'{a}' FROM t /*app='grafana'*/",
			style:    LineComment | BlockComment,
			wantBody: "SELECT data#>'{a}' FROM t ",
			wantTag:  "/*app='grafana'*/",
		},
		{
			name:     "inline (non-trailing) tag is left in place",
			sql:      "SELECT 1 /*k='v'*/ WHERE x = 1",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 /*k='v'*/ WHERE x = 1",
			wantTag:  "",
		},
		{
			name:     "plain comment is not a tag",
			sql:      "SELECT 1 /* just a note */",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 /* just a note */",
			wantTag:  "",
		},
		{
			name:     "no comment",
			sql:      "SELECT 1 FROM t",
			style:    LineComment | BlockComment,
			wantBody: "SELECT 1 FROM t",
			wantTag:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, tag := SplitTrailingSQLCommenter(tc.sql, tc.style)
			if body != tc.wantBody {
				t.Errorf("body: got %q, want %q", body, tc.wantBody)
			}
			if tag != tc.wantTag {
				t.Errorf("tag: got %q, want %q", tag, tc.wantTag)
			}
		})
	}
}

func TestInterpolate_preservesTrailingSQLCommenterTag(t *testing.T) {
	macros := MacroMap[struct{}]{
		"foo": func(_ QueryContext[struct{}], _ []string) (string, error) {
			return "BAR", nil
		},
	}

	tests := []struct {
		name  string
		query string
		opts  []Option
		want  string
	}{
		{
			name:  "trailing tag survives macro expansion",
			query: "SELECT $__foo FROM t\n/*application='grafana',source='bi'*/;",
			want:  "SELECT BAR FROM t\n/*application='grafana',source='bi'*/;",
		},
		{
			name:  "trailing tag survives when no macro is present",
			query: "SELECT 1 /*application='grafana'*/",
			want:  "SELECT 1 /*application='grafana'*/",
		},
		{
			name:  "macro-shaped text inside the tag is not expanded",
			query: "SELECT 1 /*k='$__foo'*/",
			want:  "SELECT 1 /*k='$__foo'*/",
		},
		{
			// StripComments blanks the comment to spaces (length-preserving),
			// then $__foo shrinks to BAR.
			name:  "plain block comment is still stripped",
			query: "SELECT $__foo /* note */",
			want:  "SELECT BAR " + strings.Repeat(" ", len("/* note */")),
		},
		{
			name:  "tag inside a trailing hash comment is stripped, not revived",
			query: "SELECT $__foo # /*k='v'*/",
			opts:  []Option{WithComments(LineComment | BlockComment | HashComment)},
			want:  "SELECT BAR " + strings.Repeat(" ", len("# /*k='v'*/")),
		},
		{
			name:  "tag is not split when block comment stripping is disabled",
			query: "SELECT 1 /*k='$__foo'*/",
			opts:  []Option{WithComments(0)},
			want:  "SELECT 1 /*k='BAR'*/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Interpolate(tc.query, macros, QueryContext[struct{}]{}, tc.opts...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

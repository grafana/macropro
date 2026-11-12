// Package conformance pins the output of every macro a plugin ships. A plugin
// lists its invocations in a Suite, Run interpolates them under a fixed
// context and compares the rendered Markdown table with a golden file, so a
// macropro upgrade that changes an inherited default fails the plugin's own
// tests instead of changing a query.
package conformance

import (
	"sort"
	"time"

	"github.com/grafana/macropro"
)

// DefaultPrefix is the macro prefix a Suite uses when Prefix is empty.
const DefaultPrefix = "$__"

// Suite is one plugin's macro set under test.
type Suite[T any] struct {
	Macros  macropro.MacroMap[T]
	Options []macropro.Option
	// Context is the query context for every case. The zero value means
	// DefaultContext.
	Context macropro.QueryContext[T]
	// Prefix is the WithPrefix value when the suite passes that option.
	// Empty means DefaultPrefix.
	Prefix string
	Cases  []Case
}

// Case is one invocation. Name defaults to Query.
type Case struct {
	Name  string
	Query string
}

// Result is the outcome of one case. Error holds the error text and Output is
// empty when interpolation failed.
type Result struct {
	Name   string
	Query  string
	Output string
	Error  string
}

// DefaultContext returns the fixed context every suite shares unless it sets
// its own. Its values never change, because a change would alter every golden
// file in the fleet.
func DefaultContext[T any]() macropro.QueryContext[T] {
	return macropro.QueryContext[T]{
		TimeRange: macropro.TimeRange{
			From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		Interval:   5 * time.Minute,
		IntervalMS: 300_000,
		Table:      "metrics",
		Column:     "value",
	}
}

// Results interpolates every case and then the engine probes, in order.
func Results[T any](s Suite[T]) []Result {
	ctx := s.context()
	cases := append(append([]Case(nil), s.Cases...), probes(s)...)
	results := make([]Result, 0, len(cases))
	for _, c := range cases {
		name := c.Name
		if name == "" {
			name = c.Query
		}
		r := Result{Name: name, Query: c.Query}
		out, err := macropro.Interpolate(c.Query, s.Macros, ctx, s.Options...)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Output = out
		}
		results = append(results, r)
	}
	return results
}

func (s Suite[T]) context() macropro.QueryContext[T] {
	c := s.Context
	if c.TimeRange.From.IsZero() && c.TimeRange.To.IsZero() &&
		c.Interval == 0 && c.IntervalMS == 0 && c.Table == "" && c.Column == "" {
		d := DefaultContext[T]()
		d.Extra = c.Extra
		return d
	}
	return c
}

func (s Suite[T]) prefix() string {
	if s.Prefix == "" {
		return DefaultPrefix
	}
	return s.Prefix
}

func probes[T any](s Suite[T]) []Case {
	p := s.prefix()
	out := []Case{{Name: "engine: unknown macro passes through", Query: "SELECT " + p + "conformanceUnknown(a, b)"}}
	if len(s.Macros) == 0 {
		return out
	}
	names := make([]string, 0, len(s.Macros))
	for name := range s.Macros {
		names = append(names, name)
	}
	sort.Strings(names)
	m := p + names[0]
	return append(out,
		Case{Name: "engine: macro inside a line comment", Query: "SELECT 1 -- " + m},
		Case{Name: "engine: macro inside a block comment", Query: "SELECT /* " + m + " */ 1"},
		Case{Name: "engine: trailing SQLCommenter tag", Query: "SELECT " + m + " /*traceparent='00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'*/"},
	)
}

// Package conformance pins the output of every macro a plugin ships. A plugin
// lists its invocations in a Suite, Run interpolates them under a fixed
// context and compares the rendered Markdown table with a golden file, so a
// macropro upgrade that changes an inherited default fails the plugin's own
// tests instead of changing a query.
package conformance

import (
	"time"

	"github.com/grafana/macropro"
)

// DefaultPrefix is the macro prefix a Suite uses when Prefix is empty.
const DefaultPrefix = "$__"

// Suite is one plugin's macro set under test.
type Suite[T any] struct {
	Macros  macropro.MacroMap[T]
	Options []macropro.Option
	// Context is the query context for every case. If only Extra is set,
	// Results uses DefaultContext with that Extra. Unset fields of any other
	// Context stay zero.
	Context macropro.QueryContext[T]
	// Prefix is the macro prefix for every case and probe. Empty means
	// DefaultPrefix. It overrides any WithPrefix in Options.
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
	p := s.prefix()
	macros := macropro.MergeMacros(s.Macros, macropro.MacroMap[T]{probeMacro: probe[T]})
	opts := append(append([]macropro.Option(nil), s.Options...), macropro.WithPrefix(p))
	cases := append(append([]Case(nil), s.Cases...), probes(p)...)
	results := make([]Result, 0, len(cases))
	for _, c := range cases {
		name := c.Name
		if name == "" {
			name = c.Query
		}
		r := Result{Name: name, Query: c.Query}
		out, err := macropro.Interpolate(c.Query, macros, ctx, opts...)
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

const probeMacro = "conformanceProbe"

func probe[T any](macropro.QueryContext[T], []string) (string, error) { return "PROBE", nil }

func probes(prefix string) []Case {
	m := prefix + probeMacro
	return []Case{
		{Name: "engine: unknown macro passes through", Query: "SELECT " + prefix + "conformanceUnknown(a, b)"},
		{Name: "engine: macro inside a line comment", Query: "SELECT 1 -- " + m},
		{Name: "engine: macro inside a block comment", Query: "SELECT /* " + m + " */ 1"},
		{Name: "engine: trailing SQLCommenter tag", Query: "SELECT " + m + " /*traceparent='00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'*/"},
	}
}

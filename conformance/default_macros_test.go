package conformance_test

import (
	"flag"
	"testing"

	"github.com/grafana/macropro"
	"github.com/grafana/macropro/conformance"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestDefaultMacrosConformance(t *testing.T) {
	conformance.Run(t, conformance.Suite[struct{}]{
		Macros: macropro.DefaultMacros[struct{}](),
		Cases: []conformance.Case{
			{Query: "$__interval"},
			{Query: "$__interval()"},
			{Query: "$__interval(x)"},
			{Query: "$__interval_ms"},
			{Query: "$__interval_ms()"},
			{Query: "$__timeFrom"},
			{Query: "$__timeFrom()"},
			{Query: "$__timeFrom(created_at)"},
			{Query: "$__timeFrom(a, b)"},
			{Query: "$__timeTo"},
			{Query: "$__timeTo()"},
			{Query: "$__timeTo(created_at)"},
			{Query: "$__timeTo(a, b)"},
			{Query: "$__timeFilter"},
			{Query: "$__timeFilter()"},
			{Query: "$__timeFilter(created_at)"},
			{Query: "$__timeFilter(COALESCE(a, b))"},
			{Query: "$__timeFilter(a, b)"},
			{Query: "$__timeGroup"},
			{Query: "$__timeGroup()"},
			{Query: "$__timeGroup(created_at)"},
			{Query: "$__timeGroup(created_at, 5m)"},
			{Query: "$__timeGroup(created_at, 1d)"},
			{Query: "$__timeGroup(created_at, $__interval)"},
			{Query: "$__timeGroup(created_at, 5m, 0)"},
			{Query: "$__table"},
			{Query: "$__table()"},
			{Query: "$__column"},
			{Query: "$__column()"},
			{Name: "full query", Query: "SELECT $__timeGroup(created_at, $__interval) AS t, avg($__column) FROM $__table WHERE $__timeFilter(created_at) GROUP BY t"},
		},
	}, "testdata/default_macros.md", *update)
}

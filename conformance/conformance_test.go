package conformance

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/macropro"
)

func TestDefaultContext(t *testing.T) {
	got := DefaultContext[struct{}]()
	want := macropro.QueryContext[struct{}]{
		TimeRange: macropro.TimeRange{
			From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		Interval:   5 * time.Minute,
		IntervalMS: 300_000,
		Table:      "metrics",
		Column:     "value",
	}
	if got != want {
		t.Fatalf("DefaultContext() = %+v, want %+v", got, want)
	}
}

func staticMacro(out string) macropro.MacroFunc[struct{}] {
	return func(macropro.QueryContext[struct{}], []string) (string, error) { return out, nil }
}

func TestResults_RunsCasesInOrder(t *testing.T) {
	s := Suite[struct{}]{
		Macros: macropro.MacroMap[struct{}]{"foo": staticMacro("FOO")},
		Cases: []Case{
			{Name: "named", Query: "SELECT $__foo"},
			{Query: "SELECT $__foo, 1"},
		},
	}
	got := Results(s)
	if len(got) < 2 {
		t.Fatalf("got %d results, want at least the 2 cases", len(got))
	}
	want := []Result{
		{Name: "named", Query: "SELECT $__foo", Output: "SELECT FOO"},
		{Name: "SELECT $__foo, 1", Query: "SELECT $__foo, 1", Output: "SELECT FOO, 1"},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("result %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestResults_UsesDefaultContextWhenUnset(t *testing.T) {
	s := Suite[struct{}]{
		Macros: macropro.DefaultMacros[struct{}](),
		Cases:  []Case{{Query: "$__timeFrom()"}, {Query: "$__table"}},
	}
	got := Results(s)
	if got[0].Output != "'2024-01-01T00:00:00Z'" {
		t.Errorf("timeFrom under the default context = %q", got[0].Output)
	}
	if got[1].Output != "metrics" {
		t.Errorf("table under the default context = %q", got[1].Output)
	}
}

func TestResults_KeepsAnExplicitContext(t *testing.T) {
	s := Suite[struct{}]{
		Macros:  macropro.DefaultMacros[struct{}](),
		Context: macropro.QueryContext[struct{}]{Table: "events"},
		Cases:   []Case{{Query: "$__table"}},
	}
	if got := Results(s)[0].Output; got != "events" {
		t.Errorf("table under an explicit context = %q, want %q", got, "events")
	}
}

func TestResults_CarriesExtraThroughTheDefaultContext(t *testing.T) {
	type extra struct{ Database string }
	s := Suite[extra]{
		Macros: macropro.MergeMacros(macropro.DefaultMacros[extra](), macropro.MacroMap[extra]{
			"db": func(ctx macropro.QueryContext[extra], _ []string) (string, error) { return ctx.Extra.Database, nil },
		}),
		Context: macropro.QueryContext[extra]{Extra: extra{Database: "prod"}},
		Cases:   []Case{{Query: "SELECT $__db"}, {Query: "$__table"}},
	}
	got := Results(s)
	if got[0].Output != "SELECT prod" {
		t.Errorf("Extra under the default context = %q, want %q", got[0].Output, "SELECT prod")
	}
	if got[1].Output != "metrics" {
		t.Errorf("table with only Extra set = %q, want the default %q", got[1].Output, "metrics")
	}
}

func TestResults_RecordsErrors(t *testing.T) {
	s := Suite[struct{}]{
		Macros: macropro.MacroMap[struct{}]{
			"fails": func(macropro.QueryContext[struct{}], []string) (string, error) {
				return "", errors.New("boom")
			},
			"panics": func(macropro.QueryContext[struct{}], []string) (string, error) {
				panic("kaboom")
			},
		},
		Cases: []Case{{Query: "$__fails(x)"}, {Query: "$__panics"}},
	}
	got := Results(s)
	if got[0].Output != "" || got[0].Error != "macro $__fails(x): boom" {
		t.Errorf("error case = %+v", got[0])
	}
	if got[1].Error == "" || got[1].Output != "" {
		t.Errorf("panic case = %+v, want a recorded error", got[1])
	}
}

func TestResults_ExpandsInsideStringLiterals(t *testing.T) {
	s := Suite[struct{}]{
		Macros: macropro.DefaultMacros[struct{}](),
		Cases:  []Case{{Query: "SELECT '$__interval'"}},
	}
	got := Results(s)[0]
	if got.Error != "" || got.Output != "SELECT '5m'" {
		t.Errorf("string literal case = %+v, want SELECT '5m' with no error", got)
	}
}

const traceTag = "/*traceparent='00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'*/"

func TestResults_AppendsEngineProbes(t *testing.T) {
	needsArg := func(_ macropro.QueryContext[struct{}], args []string) (string, error) {
		if len(args) != 1 {
			return "", errors.New("needs one argument")
		}
		return args[0], nil
	}
	s := Suite[struct{}]{
		Macros: macropro.MacroMap[struct{}]{"zeta": staticMacro("Z"), "alpha": needsArg},
		Cases:  []Case{{Query: "$__zeta"}},
	}
	got := Results(s)
	want := []Result{
		{Name: "$__zeta", Query: "$__zeta", Output: "Z"},
		{Name: "engine: unknown macro passes through", Query: "SELECT $__conformanceUnknown(a, b)", Output: "SELECT $__conformanceUnknown(a, b)"},
		{Name: "engine: macro inside a line comment", Query: "SELECT 1 -- $__conformanceProbe", Output: "SELECT 1 " + strings.Repeat(" ", len("-- $__conformanceProbe"))},
		{Name: "engine: macro inside a block comment", Query: "SELECT /* $__conformanceProbe */ 1", Output: "SELECT " + strings.Repeat(" ", len("/* $__conformanceProbe */")) + " 1"},
		{Name: "engine: trailing SQLCommenter tag", Query: "SELECT $__conformanceProbe " + traceTag, Output: "SELECT PROBE " + traceTag},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestResults_PrefixAppliesToCasesAndProbes(t *testing.T) {
	s := Suite[struct{}]{
		Macros: macropro.MacroMap[struct{}]{"foo": staticMacro("F")},
		Prefix: "v.",
		Cases:  []Case{{Query: "SELECT v.foo"}},
	}
	got := Results(s)
	if got[0].Output != "SELECT F" {
		t.Errorf("case output = %q, want SELECT F", got[0].Output)
	}
	if got[1].Query != "SELECT v.conformanceUnknown(a, b)" {
		t.Errorf("unknown probe = %q", got[1].Query)
	}
	if got[4].Output != "SELECT PROBE "+traceTag {
		t.Errorf("trailing tag probe output = %q, want the probe macro expanded", got[4].Output)
	}
}

func TestResults_PrefixOverridesAWithPrefixOption(t *testing.T) {
	s := Suite[struct{}]{
		Macros:  macropro.MacroMap[struct{}]{"foo": staticMacro("F")},
		Options: []macropro.Option{macropro.WithPrefix("v.")},
		Cases:   []Case{{Query: "SELECT $__foo"}},
	}
	if got := Results(s)[0].Output; got != "SELECT F" {
		t.Errorf("case output = %q, want SELECT F", got)
	}
}

func TestResults_ProbesRecordCommentExpansionWhenStrippingIsOff(t *testing.T) {
	s := Suite[struct{}]{
		Macros:  macropro.MacroMap[struct{}]{"foo": staticMacro("F")},
		Options: []macropro.Option{macropro.WithComments(0)},
	}
	got := Results(s)
	if got[1].Output != "SELECT 1 -- PROBE" {
		t.Errorf("line comment probe with stripping off = %q, want the macro expanded", got[1].Output)
	}
}

func TestResults_EmptyMacroMapStillRunsEveryProbe(t *testing.T) {
	got := Results(Suite[struct{}]{})
	if len(got) != 4 {
		t.Fatalf("got %d results, want the 4 engine probes", len(got))
	}
	if got[3].Output != "SELECT PROBE "+traceTag {
		t.Errorf("trailing tag probe output = %q, want the probe macro expanded", got[3].Output)
	}
}

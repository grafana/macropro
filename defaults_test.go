package macropro

import (
	"testing"
	"time"
)

var testCtx = QueryContext[struct{}]{
	TimeRange: TimeRange{
		From: mustTime("2024-01-01T00:00:00Z"),
		To:   mustTime("2024-01-02T00:00:00Z"),
	},
	Interval:   5 * time.Minute,
	IntervalMS: 300_000,
	Table:      "metrics",
	Column:     "value",
}

func TestDefaultMacros_interval(t *testing.T) {
	got, err := Interpolate("$__interval", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "5m"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_interval_ms(t *testing.T) {
	got, err := Interpolate("$__interval_ms", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "300000"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeFrom(t *testing.T) {
	got, err := Interpolate("$__timeFrom()", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2024-01-01T00:00:00Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeTo(t *testing.T) {
	got, err := Interpolate("$__timeTo()", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2024-01-02T00:00:00Z"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeFilter(t *testing.T) {
	got, err := Interpolate(
		"WHERE $__timeFilter(created_at)",
		DefaultMacros[struct{}](),
		testCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE created_at >= '2024-01-01T00:00:00Z' AND created_at <= '2024-01-02T00:00:00Z'"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestDefaultMacros_timeFilter_missingArg(t *testing.T) {
	_, err := Interpolate("$__timeFilter()", DefaultMacros[struct{}](), testCtx)
	if err == nil {
		t.Fatal("expected error for missing column arg")
	}
}

func TestDefaultMacros_timeGroup(t *testing.T) {
	got, err := Interpolate(
		"$__timeGroup(ts, 5m)",
		DefaultMacros[struct{}](),
		testCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "FLOOR(UNIX_TIMESTAMP(ts)/300)*300"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeGroup_quotedInterval(t *testing.T) {
	got, err := Interpolate(
		"$__timeGroup(ts, '1h')",
		DefaultMacros[struct{}](),
		testCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "FLOOR(UNIX_TIMESTAMP(ts)/3600)*3600"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeGroup_badInterval(t *testing.T) {
	_, err := Interpolate("$__timeGroup(ts, notaduration)", DefaultMacros[struct{}](), testCtx)
	if err == nil {
		t.Fatal("expected error for invalid interval")
	}
}

func TestDefaultMacros_table(t *testing.T) {
	got, err := Interpolate("SELECT * FROM $__table", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT * FROM metrics"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_column(t *testing.T) {
	got, err := Interpolate("ORDER BY $__column", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ORDER BY value"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_intervalLongestMatchFirst(t *testing.T) {
	// Both $__interval and $__interval_ms are in the default set.
	// $__interval_ms must not be partially matched as $__interval.
	got, err := Interpolate(
		"$__interval_ms ms, $__interval s",
		DefaultMacros[struct{}](),
		testCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "300000 ms, 5m s"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{30 * time.Second, "30s"},
		{500 * time.Millisecond, "500ms"},
		{0, "0s"},
	}
	for _, tc := range cases {
		got := FormatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestDefaultMacros_genericExtra(t *testing.T) {
	type MyExtra struct{ Schema string }

	overrides := MacroMap[MyExtra]{
		"schema": func(ctx QueryContext[MyExtra], _ []string) (string, error) {
			return ctx.Extra.Schema, nil
		},
	}
	macros := MergeMacros(DefaultMacros[MyExtra](), overrides)
	ctx := QueryContext[MyExtra]{
		TimeRange:  testCtx.TimeRange,
		Interval:   testCtx.Interval,
		IntervalMS: testCtx.IntervalMS,
		Extra:      MyExtra{Schema: "public"},
	}

	got, err := Interpolate("FROM $__schema.table WHERE $__timeFrom()", macros, ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := "FROM public.table WHERE 2024-01-01T00:00:00Z"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

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
	if want := "'2024-01-01T00:00:00Z'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeFrom_bareToken(t *testing.T) {
	got, err := Interpolate("$__timeFrom", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "'2024-01-01T00:00:00Z'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeFrom_filterForm(t *testing.T) {
	got, err := Interpolate("WHERE $__timeFrom(created_at)", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "WHERE created_at >= '2024-01-01T00:00:00Z'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeFrom_tooManyArgs(t *testing.T) {
	_, err := Interpolate("$__timeFrom(a, b)", DefaultMacros[struct{}](), testCtx)
	if err == nil {
		t.Fatal("expected error for more than one argument")
	}
}

func TestDefaultMacros_timeTo(t *testing.T) {
	got, err := Interpolate("$__timeTo()", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "'2024-01-02T00:00:00Z'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeTo_filterForm(t *testing.T) {
	got, err := Interpolate("WHERE $__timeTo(created_at)", DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "WHERE created_at <= '2024-01-02T00:00:00Z'"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMacros_timeBoundaries_rangeQuery(t *testing.T) {
	got, err := Interpolate(
		"WHERE ts BETWEEN $__timeFrom() AND $__timeTo()",
		DefaultMacros[struct{}](),
		testCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE ts BETWEEN '2024-01-01T00:00:00Z' AND '2024-01-02T00:00:00Z'"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
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

// TestFormatDuration pins FormatDuration to byte parity with
// gtime.FormatInterval, the $__interval contract every sqlutil-based plugin
// ships. The want values are golden outputs of gtime.FormatInterval
// (grafana-plugin-sdk-go v0.293.0, backend/gtime/gtime.go) — do not adjust
// them without cross-checking against the SDK.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		// Sub-day snapped intervals: identical before and after parity.
		{"5m", 5 * time.Minute, "5m"},
		{"2h", 2 * time.Hour, "2h"},
		{"30s", 30 * time.Second, "30s"},
		{"500ms", 500 * time.Millisecond, "500ms"},
		{"999ms", 999 * time.Millisecond, "999ms"},
		// Day and year rungs (no week rung: gtime formats 7d as "7d").
		{"24h is 1d", 24 * time.Hour, "1d"},
		{"7d stays 7d", 7 * 24 * time.Hour, "7d"},
		{"365d is 1y", 365 * 24 * time.Hour, "1y"},
		{"729d truncates to 1y", 729 * 24 * time.Hour, "1y"},
		// Threshold ladder truncates non-whole units, matching gtime.
		{"90m truncates to 1h", 90 * time.Minute, "1h"},
		{"36h truncates to 1d", 36 * time.Hour, "1d"},
		{"1m30s truncates to 1m", 90 * time.Second, "1m"},
		{"1500ms truncates to 1s", 1500 * time.Millisecond, "1s"},
		// Degenerate inputs floor to "1ms", matching gtime.
		{"zero", 0, "1ms"},
		{"negative", -5 * time.Minute, "1ms"},
		{"sub-millisecond", 800 * time.Microsecond, "1ms"},
	}
	for _, tc := range cases {
		got := FormatDuration(tc.d)
		if got != tc.want {
			t.Errorf("%s: FormatDuration(%v) = %q, want %q", tc.name, tc.d, got, tc.want)
		}
	}
}

// TestDefaultMacros_timeGroup_nestedInterval pins the README quick-start
// composition GROUP BY $__timeGroup(col, $__interval). Since the gtime-parity
// change, day-scale intervals format as "1d"/"1y", so timeGroup's interval
// parsing must accept Grafana's calendar units — this test fails if
// FormatDuration's output alphabet and ParseDuration's input alphabet ever
// drift apart again.
func TestDefaultMacros_timeGroup_nestedInterval(t *testing.T) {
	ctx := testCtx
	ctx.Interval = 24 * time.Hour
	got, err := Interpolate("$__timeGroup(ts, $__interval)", DefaultMacros[struct{}](), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "FLOOR(UNIX_TIMESTAMP(ts)/86400)*86400"; got != want {
		t.Errorf("got %q, want %q", got, want)
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
	want := "FROM public.table WHERE '2024-01-01T00:00:00Z'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestParseDuration pins ParseDuration to behavioural parity with the SDK's
// gtime.ParseDuration: everything stdlib time.ParseDuration accepts, plus
// Grafana's calendar units with fixed Julian constants. Golden values are
// cross-checked against grafana-plugin-sdk-go v0.293.0 (backend/gtime).
func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"30s", 30 * time.Second},
		{"1d", 24 * time.Hour},
		{"2d", 48 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"2w", 14 * 24 * time.Hour},
		{"1M", 2629800 * time.Second},  // Julian year / 12
		{"1y", 31557600 * time.Second}, // 365.25 days
		{"2y", 2 * 31557600 * time.Second},
	}
	for _, tc := range cases {
		got, err := ParseDuration(tc.in)
		if err != nil {
			t.Errorf("ParseDuration(%q) returned error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseDuration_invalid(t *testing.T) {
	// Signs, decimals, and bare numbers must not take the calendar-unit path;
	// they fall through to stdlib parsing, which rejects them, matching gtime.
	for _, in := range []string{"", "d", "-1d", "1.5d", "10", "notaduration", "1q"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) expected error, got nil", in)
		}
	}
}

func TestDefaultMacros_timeGroup_calendarUnits(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"$__timeGroup(ts, 1d)", "FLOOR(UNIX_TIMESTAMP(ts)/86400)*86400"},
		{"$__timeGroup(ts, 1w)", "FLOOR(UNIX_TIMESTAMP(ts)/604800)*604800"},
		{"$__timeGroup(ts, '1M')", "FLOOR(UNIX_TIMESTAMP(ts)/2629800)*2629800"},
		{"$__timeGroup(ts, 1y)", "FLOOR(UNIX_TIMESTAMP(ts)/31557600)*31557600"},
	}
	for _, tc := range cases {
		got, err := Interpolate(tc.in, DefaultMacros[struct{}](), testCtx)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTimeGroupExtractEpoch(t *testing.T) {
	macros := MergeMacros(DefaultMacros[struct{}](), MacroMap[struct{}]{
		"timeGroup": TimeGroupExtractEpoch[struct{}],
	})
	cases := []struct {
		in   string
		want string
	}{
		{"$__timeGroup(ts, 5m)", "floor(extract(epoch from ts)/300)*300"},
		{"$__timeGroup(ts, 1d)", "floor(extract(epoch from ts)/86400)*86400"},
	}
	for _, tc := range cases {
		got, err := Interpolate(tc.in, macros, testCtx)
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTimeGroupExtractEpoch_errors(t *testing.T) {
	macros := MergeMacros(DefaultMacros[struct{}](), MacroMap[struct{}]{
		"timeGroup": TimeGroupExtractEpoch[struct{}],
	})
	for _, in := range []string{
		"$__timeGroup(ts)",               // missing interval
		"$__timeGroup(ts, notaduration)", // unparseable interval
		"$__timeGroup(ts, 0s)",           // non-positive interval
		"$__timeGroup(ts, -5m)",          // negative interval
		"$__timeGroup(, 5m)",             // empty column
		"$__timeGroup(ts, 9999999999d)",  // calendar-unit overflow
	} {
		if _, err := Interpolate(in, macros, testCtx); err == nil {
			t.Errorf("%s: expected error, got nil", in)
		}
	}
}

func TestTimeGroupUnixTimestamp_matchesDefault(t *testing.T) {
	// The default timeGroup IS the MySQL-family recipe; registering the
	// recipe explicitly must be a no-op.
	explicit := MergeMacros(DefaultMacros[struct{}](), MacroMap[struct{}]{
		"timeGroup": TimeGroupUnixTimestamp[struct{}],
	})
	const q = "$__timeGroup(ts, 5m)"
	def, err := Interpolate(q, DefaultMacros[struct{}](), testCtx)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := Interpolate(q, explicit, testCtx)
	if err != nil {
		t.Fatal(err)
	}
	if def != rec {
		t.Errorf("default %q != recipe %q", def, rec)
	}
	if want := "FLOOR(UNIX_TIMESTAMP(ts)/300)*300"; def != want {
		t.Errorf("got %q, want %q", def, want)
	}
}

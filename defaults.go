package macropro

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DefaultMacros returns a MacroMap containing the standard Grafana macros with
// SQL-flavoured starting-point implementations. All timestamps are formatted
// as RFC 3339. The defaults are not valid in every dialect: $__timeGroup in
// particular emits the MySQL-family epoch idiom and must be overridden — or
// replaced with a recipe such as [TimeGroupExtractEpoch] — on PostgreSQL,
// CockroachDB, Redshift, MSSQL, BigQuery, and ClickHouse.
//
// Datasources should call MergeMacros(DefaultMacros[MyCtx](), myOverrides) to
// layer their own implementations on top of these defaults.
func DefaultMacros[T any]() MacroMap[T] {
	return MacroMap[T]{
		"interval":    macroInterval[T],
		"interval_ms": macroIntervalMS[T],
		"timeFrom":    macroTimeFrom[T],
		"timeTo":      macroTimeTo[T],
		"timeFilter":  macroTimeFilter[T],
		"timeGroup":   TimeGroupUnixTimestamp[T],
		"table":       macroTable[T],
		"column":      macroColumn[T],
	}
}

// macroInterval returns the query interval as a duration string in Grafana's
// interval notation (e.g. "5m", "1d") via [FormatDuration]. Zero-arg macro —
// any arguments are ignored.
func macroInterval[T any](ctx QueryContext[T], _ []string) (string, error) {
	return FormatDuration(ctx.Interval), nil
}

// macroIntervalMS returns the query interval in milliseconds as a plain integer
// string (e.g. "300000"). Zero-arg macro — any arguments are ignored.
func macroIntervalMS[T any](ctx QueryContext[T], _ []string) (string, error) {
	return strconv.FormatInt(ctx.IntervalMS, 10), nil
}

// macroTimeFrom renders the start of the time range. With no arguments it
// returns the timestamp as a quoted RFC 3339 string literal; with a column
// argument it returns a lower-bound filter expression matching the output of
// sqlutil.DefaultMacros, so sqlds migrations keep filter-style queries
// working unchanged.
//
//	$__timeFrom()     → '2024-01-01T00:00:00Z'
//	$__timeFrom(time) → time >= '2024-01-01T00:00:00Z'
func macroTimeFrom[T any](ctx QueryContext[T], args []string) (string, error) {
	return timeBoundary("$__timeFrom", ">=", ctx.TimeRange.From, args)
}

// macroTimeTo is the upper-bound counterpart to macroTimeFrom.
//
//	$__timeTo()     → '2024-01-02T00:00:00Z'
//	$__timeTo(time) → time <= '2024-01-02T00:00:00Z'
func macroTimeTo[T any](ctx QueryContext[T], args []string) (string, error) {
	return timeBoundary("$__timeTo", "<=", ctx.TimeRange.To, args)
}

// timeBoundary implements the dual-mode behaviour shared by macroTimeFrom and
// macroTimeTo. A single empty argument is treated like no argument at all, so
// engines that parse "()" as one empty string (as sqlutil does) still get the
// value form rather than a filter expression with a missing column.
func timeBoundary(name, op string, t time.Time, args []string) (string, error) {
	ts := formatTime(t)
	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == ""):
		return "'" + ts + "'", nil
	case len(args) == 1:
		return fmt.Sprintf("%s %s '%s'", args[0], op, ts), nil
	default:
		return "", fmt.Errorf("%s accepts at most 1 argument (column name), got %d", name, len(args))
	}
}

// macroTimeFilter expects exactly one argument: the column name. It expands to
// a BETWEEN filter covering the dashboard time range.
//
//	$__timeFilter(time) → time >= '2024-01-01T00:00:00Z' AND time <= '2024-01-02T00:00:00Z'
func macroTimeFilter[T any](ctx QueryContext[T], args []string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("$__timeFilter requires exactly 1 argument (column name), got %d", len(args))
	}
	col := args[0]
	from := formatTime(ctx.TimeRange.From)
	to := formatTime(ctx.TimeRange.To)
	return fmt.Sprintf("%s >= '%s' AND %s <= '%s'", col, from, col, to), nil
}

// timeGroupArgs validates and parses the (column, interval) argument pair
// shared by the $__timeGroup recipes, returning the trimmed column expression
// and the interval rounded to whole seconds. The interval accepts Grafana's
// calendar units via [ParseDuration] and may be quoted.
func timeGroupArgs(args []string) (string, int64, error) {
	// Errors carry no macro name: Interpolate wraps every handler error with
	// the invoking macro's name, and the recipes can be registered under any
	// name or prefix.
	if len(args) != 2 {
		return "", 0, fmt.Errorf("requires 2 arguments (column, interval), got %d", len(args))
	}
	col := strings.TrimSpace(args[0])
	if col == "" {
		return "", 0, fmt.Errorf("requires a column expression, got an empty first argument")
	}
	intervalStr := strings.Trim(strings.TrimSpace(args[1]), "'\"")

	d, err := ParseDuration(intervalStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid interval %q: %w", intervalStr, err)
	}
	secs := int64(math.Round(d.Seconds()))
	if secs <= 0 {
		return "", 0, fmt.Errorf("interval must be positive, got %q", intervalStr)
	}
	return col, secs, nil
}

// TimeGroupUnixTimestamp is the MySQL-family $__timeGroup recipe and the
// default implementation registered by [DefaultMacros]. It buckets the column
// into interval-sized groups using the epoch idiom valid on MySQL, MariaDB,
// and Spark-derived dialects such as Databricks. The result is a numeric
// epoch bucket, so alias it as your time column.
//
//	$__timeGroup(time, 5m) → FLOOR(UNIX_TIMESTAMP(time)/300)*300
func TimeGroupUnixTimestamp[T any](_ QueryContext[T], args []string) (string, error) {
	col, secs, err := timeGroupArgs(args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("FLOOR(UNIX_TIMESTAMP(%s)/%d)*%d", col, secs, secs), nil
}

// TimeGroupExtractEpoch is the PostgreSQL-family $__timeGroup recipe, matching
// the expression Grafana's PostgreSQL datasource emits for the two-argument
// form. Valid on PostgreSQL, CockroachDB, and Redshift. The fill-mode third
// argument accepted by Grafana's core SQL datasources is not supported: fill
// is a frame post-processing concern, not SQL. Register it as an override:
//
//	MergeMacros(DefaultMacros[T](), MacroMap[T]{"timeGroup": TimeGroupExtractEpoch[T]})
//
//	$__timeGroup(time, 5m) → floor(extract(epoch from time)/300)*300
func TimeGroupExtractEpoch[T any](_ QueryContext[T], args []string) (string, error) {
	col, secs, err := timeGroupArgs(args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("floor(extract(epoch from %s)/%d)*%d", col, secs, secs), nil
}

// macroTable returns the table name from the query context. Zero-arg macro.
func macroTable[T any](ctx QueryContext[T], _ []string) (string, error) {
	if ctx.Table == "" {
		return "", fmt.Errorf("$__table: no table name in query context")
	}
	return ctx.Table, nil
}

// macroColumn returns the column name from the query context. Zero-arg macro.
func macroColumn[T any](ctx QueryContext[T], _ []string) (string, error) {
	if ctx.Column == "" {
		return "", fmt.Errorf("$__column: no column name in query context")
	}
	return ctx.Column, nil
}

// formatTime returns t as an RFC 3339 UTC string.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatDuration renders d in the largest unit that fits, truncating any
// remainder (90m renders as "1h"). It is byte-compatible with the SDK's
// gtime.FormatInterval — the $__interval contract shipped by every
// sqlutil-based plugin — so a datasource migrating from sqlds keeps its exact
// interval strings. The unit ladder is y (365d), d, h, m, s, ms; Grafana has
// no week unit on the formatting side, so seven days renders as "7d". Zero,
// negative, and sub-millisecond durations all render as "1ms", matching the
// SDK.
func FormatDuration(d time.Duration) string {
	const (
		day  = 24 * time.Hour
		year = 365 * day
	)
	switch {
	case d >= year:
		return strconv.FormatInt(int64(d/year), 10) + "y"
	case d >= day:
		return strconv.FormatInt(int64(d/day), 10) + "d"
	case d >= time.Hour:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d >= time.Minute:
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	case d >= time.Second:
		return strconv.FormatInt(int64(d/time.Second), 10) + "s"
	case d >= time.Millisecond:
		return strconv.FormatInt(int64(d/time.Millisecond), 10) + "ms"
	default:
		return "1ms"
	}
}

// ParseDuration parses a duration in Grafana's notation: everything stdlib
// time.ParseDuration accepts, plus the calendar units "d" (24h), "w" (7d),
// "M", and "y" (Julian: a year is 365.25 days and a month is a twelfth of
// that). It matches the SDK's gtime.ParseDuration, which uses these fixed
// constants — unlike gtime.ParseInterval, whose calendar units are relative
// to the wall clock — so output is deterministic. A calendar unit must be a
// bare run of digits plus the unit letter; signs, decimals, and compound
// values ("1h30m") take the stdlib path.
func ParseDuration(s string) (time.Duration, error) {
	const (
		day  = 24 * time.Hour
		week = 7 * day
		// Julian year, matching gtime's daysInAYear constant.
		year  = time.Duration(365.25 * 24 * float64(time.Hour))
		month = year / 12
	)
	if n := len(s) - 1; n >= 1 && isDigits(s[:n]) {
		num, err := strconv.Atoi(s[:n])
		if err == nil {
			var unit time.Duration
			switch s[n] {
			case 'd':
				unit = day
			case 'w':
				unit = week
			case 'M':
				unit = month
			case 'y':
				unit = year
			}
			if unit != 0 {
				d := time.Duration(num) * unit
				// One deliberate divergence from gtime, which wraps silently
				// here and hands the caller a garbage duration.
				if time.Duration(num) != 0 && d/unit != time.Duration(num) {
					return 0, fmt.Errorf("duration %q overflows", s)
				}
				return d, nil
			}
		}
	}
	return time.ParseDuration(s)
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

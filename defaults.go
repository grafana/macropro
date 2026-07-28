package macropro

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// DefaultMacros returns a MacroMap containing the standard Grafana macros with
// dialect-neutral SQL implementations. All timestamps are formatted as RFC 3339.
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
		"timeGroup":   macroTimeGroup[T],
		"table":       macroTable[T],
		"column":      macroColumn[T],
	}
}

// macroInterval returns the query interval as a human-readable duration string
// (e.g. "5m", "30s"). Zero-arg macro — any arguments are ignored.
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

// macroTimeGroup expects two arguments: the column name and the interval string
// (e.g. "5m", "1h"). It expands to a floor-division grouping expression.
//
//	$__timeGroup(time, 5m) → FLOOR(UNIX_TIMESTAMP(time)/300)*300
func macroTimeGroup[T any](_ QueryContext[T], args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("$__timeGroup requires 2 arguments (column, interval), got %d", len(args))
	}
	col := strings.TrimSpace(args[0])
	intervalStr := strings.Trim(strings.TrimSpace(args[1]), "'\"")

	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		return "", fmt.Errorf("$__timeGroup: invalid interval %q: %w", intervalStr, err)
	}
	secs := int64(math.Round(d.Seconds()))
	if secs <= 0 {
		return "", fmt.Errorf("$__timeGroup: interval must be positive, got %q", intervalStr)
	}
	return fmt.Sprintf("FLOOR(UNIX_TIMESTAMP(%s)/%d)*%d", col, secs, secs), nil
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

// FormatDuration returns d as a compact human-readable string using the
// largest whole unit (e.g. 5m, 2h, 300s). Falls back to milliseconds for
// sub-second intervals.
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < 0 {
		d = -d
	}
	switch {
	case d%time.Hour == 0:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d%time.Minute == 0:
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	case d%time.Second == 0:
		return strconv.FormatInt(int64(d/time.Second), 10) + "s"
	default:
		return strconv.FormatInt(int64(d/time.Millisecond), 10) + "ms"
	}
}

# macropro

A generic, language-agnostic Go library for parsing and expanding [Grafana macros](https://grafana.com/docs/grafana/latest/datasources/mysql/query-editor/#macros) in query strings.

Grafana macros take the form `$__name` or `$__name(arg1, arg2, …)` and are expanded at query time to inject dynamic values — time ranges, grouping intervals, table names, and so on. `macropro` provides the parsing engine and a set of dialect-neutral defaults; datasource backends register their own handlers on top.

## Installation

```sh
go get github.com/grafana/macropro
```

Requires Go 1.21 or later (uses generics and `maps.Copy`).

## Quick start

```go
package main

import (
    "fmt"
    "time"

    "github.com/grafana/macropro"
)

func main() {
    ctx := macropro.QueryContext[struct{}]{
        TimeRange: macropro.TimeRange{
            From: time.Now().Add(-time.Hour),
            To:   time.Now(),
        },
        Interval:   5 * time.Minute,
        IntervalMS: 300_000,
        Table:      "metrics",
    }

    result, err := macropro.Interpolate(
        "SELECT * FROM $__table WHERE $__timeFilter(created_at) GROUP BY $__timeGroup(created_at, $__interval)",
        macropro.DefaultMacros[struct{}](),
        ctx,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(result)
    // SELECT * FROM metrics WHERE created_at >= '2024-01-01T23:00:00Z' AND created_at <= '2024-01-02T00:00:00Z' GROUP BY FLOOR(UNIX_TIMESTAMP(created_at)/300)*300
}
```

## Datasource-specific macros

Provide datasource-specific handlers by merging them on top of the defaults. The generic type parameter `T` carries any extra context fields your handlers need.

```go
type ClickHouseCtx struct {
    Database string
}

overrides := macropro.MacroMap[ClickHouseCtx]{
    // Override $__timeFilter with ClickHouse-native syntax.
    "timeFilter": func(ctx macropro.QueryContext[ClickHouseCtx], args []string) (string, error) {
        if len(args) != 1 {
            return "", fmt.Errorf("timeFilter requires 1 argument")
        }
        col := args[0]
        from := ctx.TimeRange.From.Unix()
        to := ctx.TimeRange.To.Unix()
        return fmt.Sprintf("%s >= toDateTime(%d) AND %s <= toDateTime(%d)", col, from, col, to), nil
    },
    // Add a new datasource-specific macro.
    "database": func(ctx macropro.QueryContext[ClickHouseCtx], _ []string) (string, error) {
        return ctx.Extra.Database, nil
    },
}

macros := macropro.MergeMacros(macropro.DefaultMacros[ClickHouseCtx](), overrides)

ctx := macropro.QueryContext[ClickHouseCtx]{
    // ... populate common fields ...
    Extra: ClickHouseCtx{Database: "default"},
}

result, err := macropro.Interpolate(query, macros, ctx)
```

## Migrating from sqlds

Many Grafana SQL datasources use [`grafana/sqlds`](https://github.com/grafana/sqlds) for query execution and macro interpolation. `sqlds.MacroFunc` has a different signature to `macropro.MacroFunc[T]`:

```go
// sqlds signature
type MacroFunc func(query *sqlutil.Query, args []string) (string, error)

// macropro signature
type MacroFunc[T any] func(ctx QueryContext[T], args []string) (string, error)
```

The standard migration pattern keeps sqlds for query execution while replacing its interpolation engine with macropro. It requires two small pieces of glue in the datasource's macros package.

### Step 1 — Bridge `*sqlutil.Query` to `QueryContext`

Write a `contextFrom` function that maps the sqlds query struct to a macropro context:

```go
func contextFrom(q *sqlutil.Query) macropro.QueryContext[struct{}] {
    return macropro.QueryContext[struct{}]{
        TimeRange: macropro.TimeRange{
            From: q.TimeRange.From,
            To:   q.TimeRange.To,
        },
        Interval:   q.Interval,
        IntervalMS: q.Interval.Milliseconds(),
        Table:      q.Table,
        Column:     q.Column,
    }
}
```

If your datasource needs extra context (e.g. a database name, cluster ID), define an `Extra` struct and populate it here.

### Step 2 — Define handlers and expose an `Interpolate` function

Write your macro handlers as `macropro.MacroFunc[T]`, then expose a standalone `Interpolate` helper so call sites don't need to know about sqlds:

```go
var MyMacros = macropro.MacroMap[struct{}]{
    "timeFilter": func(ctx macropro.QueryContext[struct{}], args []string) (string, error) {
        // ... datasource-specific SQL ...
    },
    // ...
}

func Interpolate(rawSQL string, q *sqlutil.Query) (string, error) {
    return macropro.Interpolate(rawSQL, MyMacros, contextFrom(q))
}
```

### Step 3 — Keep the sqlds bridge for query execution

If the datasource uses `sqlds.Driver`, its `Macros()` method must return a `sqlds.Macros` map. Rather than duplicating logic, bridge each macropro handler with a one-line adapter:

```go
func adapt(fn macropro.MacroFunc[struct{}]) sqlds.MacroFunc {
    return func(q *sqlutil.Query, args []string) (string, error) {
        return fn(contextFrom(q), args)
    }
}

// Macros satisfies the sqlds.Driver interface. Each entry delegates to MyMacros.
var Macros = sqlds.Macros{
    "timeFilter": adapt(MyMacros["timeFilter"]),
    // ...
}
```

With this in place, `driver.Macros()` returns `Macros` unchanged and sqlds handles query execution as before. New call sites (tests, custom query paths) use `Interpolate` directly — no sqlds dependency required.

## Default macros

These are provided by `DefaultMacros[T]()`. The parsing engine is language-agnostic and works on any string (SQL, KQL, ES|QL, PromQL, etc.). `DefaultMacros` provides SQL-flavoured implementations as a starting point — non-SQL datasources should define their own `MacroMap` with only the macros they need, and SQL datasources should override any macro that requires dialect-specific syntax.

| Macro | Arguments | Default output |
|---|---|---|
| `$__interval` | — | Interval as a duration string, e.g. `5m` |
| `$__interval_ms` | — | Interval in milliseconds, e.g. `300000` |
| `$__timeFrom()` | — | Start of time range as RFC 3339, e.g. `2024-01-01T00:00:00Z` |
| `$__timeTo()` | — | End of time range as RFC 3339 |
| `$__timeFilter(col)` | column name | `col >= 'from' AND col <= 'to'` |
| `$__timeGroup(col, interval)` | column name, duration | `FLOOR(UNIX_TIMESTAMP(col)/N)*N` |
| `$__table` | — | Table name from `QueryContext.Table` |
| `$__column` | — | Column name from `QueryContext.Column` |

## Comment stripping

To prevent macros inside SQL comments from being expanded, strip comments before calling `Interpolate`. `StripComments` is length-preserving — comment regions are replaced with spaces so that line/column positions in error messages remain accurate.

```go
clean := macropro.StripComments(query, macropro.LineComment|macropro.BlockComment)
result, err := macropro.Interpolate(clean, macros, ctx)
```

Available `CommentStyle` flags:

| Flag | Strips |
|---|---|
| `LineComment` | `-- …` until end of line |
| `BlockComment` | `/* … */` |
| `HashComment` | `# …` until end of line (MySQL-style) |
| `DollarQuote` | Preserves PostgreSQL `$tag$…$tag$` dollar-quoted strings |

Flags can be combined with `\|`. Pass `0` to strip nothing.

## API reference

```go
// Interpolate replaces all recognised $__ macros in query.
// Unknown macros are left unchanged.
// Returns the first handler error encountered, with the macro name included.
func Interpolate[T any](query string, macros MacroMap[T], ctx QueryContext[T]) (string, error)

// MergeMacros returns a new MacroMap with overrides merged on top of base.
// For identical names, the override wins. The base map is not mutated.
func MergeMacros[T any](base, overrides MacroMap[T]) MacroMap[T]

// DefaultMacros returns the standard set of Grafana macros with RFC 3339 /
// generic SQL implementations.
func DefaultMacros[T any]() MacroMap[T]

// StripComments removes SQL comment regions from query, replacing them with
// spaces to preserve byte positions.
func StripComments(query string, style CommentStyle) string
```

### Types

```go
type QueryContext[T any] struct {
    TimeRange  TimeRange
    Interval   time.Duration
    IntervalMS int64
    Table      string
    Column     string
    Extra      T // datasource-specific fields
}

type TimeRange struct {
    From time.Time
    To   time.Time
}

type MacroFunc[T any] func(ctx QueryContext[T], args []string) (string, error)
type MacroMap[T any] map[string]MacroFunc[T]
```

## Implementation notes

- **Longest-match first**: macro names are sorted by descending length before scanning, so `$__interval_ms` is never partially consumed by `$__interval`.
- **Bracket-depth argument parsing**: arguments are split by `,` while tracking bracket depth and respecting single- and double-quoted strings, so `$__wrap(COALESCE(a, b), c)` correctly yields two arguments.
- **Error safety**: if a handler returns an error, `Interpolate` returns the original unmodified query alongside the error.
- **No SQL assumptions**: the parser works on any string; only the default macro implementations produce SQL.

## License

Apache 2.0 — see [LICENSE](LICENSE).

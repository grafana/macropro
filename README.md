# macropro

[![CI](https://github.com/grafana/macropro/actions/workflows/ci.yml/badge.svg)](https://github.com/grafana/macropro/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/grafana/macropro.svg)](https://pkg.go.dev/github.com/grafana/macropro)
[![Go Report Card](https://goreportcard.com/badge/github.com/grafana/macropro)](https://goreportcard.com/report/github.com/grafana/macropro)

A generic, language-agnostic Go library for parsing and expanding [Grafana macros](https://grafana.com/docs/grafana/latest/datasources/mysql/query-editor/#macros) in query strings.

Grafana macros take the form `$__name` or `$__name(arg1, arg2, …)` and are expanded at query time to inject dynamic values — time ranges, grouping intervals, table names, and so on. `macropro` provides the parsing engine and a set of SQL-flavoured default macros; datasource backends override any default whose SQL is not valid in their dialect and register their own handlers on top.

## Installation

```sh
go get github.com/grafana/macropro
```

Requires Go 1.23 or later (uses generics and `maps.Copy`).

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

Since [sqlds v5.2.0](https://github.com/grafana/sqlds/pull/269), `SQLDatasource` exposes a pluggable `Interpolator` func field that owns the full SQL rewrite for each query:

```go
type Interpolator func(ctx context.Context, query *sqlutil.Query, rawJSON json.RawMessage) (string, error)
```

The recommended pattern keeps `sqlds.Driver` for query execution and framing but assigns a macropro-backed `Interpolator`, replacing sqlds's built-in macro scan entirely. Errors returned from the interpolator propagate through the normal query path as first-class query errors.

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

If your datasource needs extra context (e.g. a database name, cluster ID), define an `Extra` struct — Step 3 shows how to populate it from the raw query JSON.

### Step 2 — Define handlers and expose an `Interpolate` function

Write your macro handlers as `macropro.MacroFunc[T]`, then expose a standalone `Interpolate` helper so call sites don't need to know about sqlds:

```go
var MyMacros = macropro.MergeMacros(
    macropro.DefaultMacros[struct{}](),
    macropro.MacroMap[struct{}]{
        "timeFilter": func(ctx macropro.QueryContext[struct{}], args []string) (string, error) {
            // ... datasource-specific SQL ...
        },
        // ...
    },
)

func Interpolate(rawSQL string, q *sqlutil.Query) (string, error) {
    return macropro.Interpolate(rawSQL, MyMacros, contextFrom(q))
}
```

### Step 3 — Assign the `Interpolator`

In your datasource factory, assign the macropro-backed func after constructing the `SQLDatasource`. sqlds calls it once per query and passes the expanded SQL straight to the driver:

```go
// Macros satisfies sqlds.Driver but is only consulted by sqlds's default
// interpolator, which the assignment below replaces.
func (d *Driver) Macros() sqlds.Macros { return sqlds.Macros{} }

func newDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
    ds := sqlds.NewDatasource(&Driver{})
    ds.Interpolator = func(_ context.Context, q *sqlutil.Query, _ json.RawMessage) (string, error) {
        return Interpolate(q.RawSQL, q)
    }
    return ds.NewDatasource(ctx, settings)
}
```

The `rawJSON` argument carries the unparsed query JSON from the request — `sqlutil.Query` only parses its fixed fields and drops the rest — so it is the channel for plugin-defined macro context. Unmarshal it into your `Extra` struct:

```go
ds.Interpolator = func(_ context.Context, q *sqlutil.Query, rawJSON json.RawMessage) (string, error) {
    mctx := contextFrom(q)
    if err := json.Unmarshal(rawJSON, &mctx.Extra); err != nil {
        return "", fmt.Errorf("parsing query JSON: %w", err)
    }
    return macropro.Interpolate(q.RawSQL, MyMacros, mctx)
}
```

Why not keep sqlds's built-in macro handling? sqlds delegates to `sqlutil`'s defaults, several of which emit SQL that is not valid in every dialect — `$__timeGroup` produces SQL-Server-style `datepart()` calls, `$__timeFrom`/`$__timeTo` emit RFC 3339 string literals that rely on implicit string→DateTime coercion. Replacing the pipeline with your own `MacroMap` lets you override those defaults with dialect-correct output and drop the `sqlds`-side macro wiring entirely. The same caveat applies to macropro's own defaults — `$__timeGroup` emits the MySQL-family idiom unless overridden (see [dialect recipes](#__timegroup-dialect-recipes)) — the difference is that the override mechanism is the primary API rather than a second interpolation pass.

### Older sqlds versions

On sqlds versions before v5.2.0 there is no `Interpolator` field. The closest equivalent is to expand macros upstream in the driver's `MutateQueryData` hook — round-tripping each query's JSON to rewrite `rawSql` — and return an empty `sqlds.Macros{}` so sqlds's own scan is a no-op on already-expanded SQL. That hook cannot return errors, so expansion failures can only be logged and passed through. Prefer upgrading sqlds and using the `Interpolator` field.

## Default macros

These are provided by `DefaultMacros[T]()`. The parsing engine is language-agnostic and works on any string (SQL, KQL, ES|QL, PromQL, etc.). `DefaultMacros` provides SQL-flavoured implementations as a starting point — non-SQL datasources should define their own `MacroMap` with only the macros they need, and SQL datasources should override any macro that requires dialect-specific syntax. In particular `$__timeGroup` defaults to the MySQL-family idiom, which is invalid on PostgreSQL, CockroachDB, Redshift, MSSQL, BigQuery, and ClickHouse — pick a [dialect recipe](#__timegroup-dialect-recipes) or write your own handler.

| Macro | Arguments | Default output |
|---|---|---|
| `$__interval` | — | Interval as a duration string in Grafana's notation, e.g. `5m`, `1d` (byte-compatible with the SDK's `gtime.FormatInterval`) |
| `$__interval_ms` | — | Interval in milliseconds, e.g. `300000` |
| `$__timeFrom([col])` | optional column name | Value form `'2024-01-01T00:00:00Z'`, or filter form `col >= '2024-01-01T00:00:00Z'` with a column |
| `$__timeTo([col])` | optional column name | Value form `'2024-01-02T00:00:00Z'`, or filter form `col <= '2024-01-02T00:00:00Z'` with a column |
| `$__timeFilter(col)` | column name | `col >= 'from' AND col <= 'to'` |
| `$__timeGroup(col, interval)` | column name, duration | `FLOOR(UNIX_TIMESTAMP(col)/N)*N` (MySQL-family — see [dialect recipes](#__timegroup-dialect-recipes)) |
| `$__table` | — | Table name from `QueryContext.Table` |
| `$__column` | — | Column name from `QueryContext.Column` |

`$__interval` renders through the same truncating ladder as `gtime.FormatInterval`: values that do not fit a whole unit lose precision, so a hand-typed min interval of `90m` renders as `1h` and `36h` renders as `1d`. Intervals calculated by Grafana are snapped (`gtime.RoundInterval`) to values that always render exactly, so truncation only affects manual overrides and API callers. Where the query needs the exact interval, use `$__interval_ms`, which is always the precise integer millisecond value.

The `interval` argument of `$__timeGroup` accepts Grafana's duration notation via `ParseDuration`: stdlib units plus `d`, `w`, `M`, and `y` (fixed Julian constants, e.g. `$__timeGroup(ts, 1d)` buckets by 86400 seconds).

### $__timeGroup dialect recipes

There is no dialect-neutral time-bucketing expression in SQL — every dialect spells it differently. macropro ships the two most common idioms as named, generic handlers:

| Recipe | Dialects | Output for `$__timeGroup(ts, 5m)` |
|---|---|---|
| `TimeGroupUnixTimestamp[T]` (default) | MySQL, MariaDB, Databricks/Spark SQL | `FLOOR(UNIX_TIMESTAMP(ts)/300)*300` |
| `TimeGroupExtractEpoch[T]` | PostgreSQL, CockroachDB, Redshift | `floor(extract(epoch from ts)/300)*300` |

Register a recipe as an override:

```go
macros := macropro.MergeMacros(macropro.DefaultMacros[struct{}](), macropro.MacroMap[struct{}]{
    "timeGroup": macropro.TimeGroupExtractEpoch[struct{}],
})
```

Both recipes take exactly two arguments and emit a numeric epoch bucket, so alias the expression as your time column (`$__timeGroup(ts, 5m) AS time`). The fill-mode third argument accepted by Grafana's core SQL datasources is not supported: fill belongs to frame post-processing, not SQL. For other dialects, write a handler that mirrors what the shipped Grafana datasource for that dialect emits — MSSQL uses `FLOOR(DATEDIFF(second, '1970-01-01', ts)/N)*N`, BigQuery uses `TIMESTAMP_MILLIS(DIV(UNIX_MILLIS(ts), Nms) * Nms)`, ClickHouse uses `toStartOfInterval(toDateTime(ts), INTERVAL N second)`, and Snowflake uses `TIME_SLICE` — reusing `macropro.ParseDuration` for interval parsing so calendar units keep working.

## Comment stripping

`Interpolate` automatically strips standard SQL line comments (`--`) and block comments (`/* */`) before expanding macros, so macro tokens hidden inside comments are never evaluated. For finer control — disabling stripping, stripping MySQL-style hash comments, or Flux-style `//` — pass [`WithComments`](#options) (see below).

You can also call `StripComments` directly. It is length-preserving: comment regions are replaced with spaces so line/column positions in error messages remain accurate.

```go
clean := macropro.StripComments(query, macropro.HashComment)
result, err := macropro.Interpolate(clean, macros, ctx)
```

### SQLCommenter tags

A trailing [SQLCommenter](https://google.github.io/sqlcommenter/) attribution tag is the one comment `Interpolate` preserves:

```sql
SELECT 1 AS value
/*application='grafana',source='bi'*/;
```

Whenever `BlockComment` stripping is enabled, the tag is split off with `SplitTrailingSQLCommenter` before stripping and macro expansion, then re-appended verbatim, so query-tagging metadata (for example PlanetScale Insights or other observability backends) reaches the database unchanged. Because the tag never passes through expansion, a macro token inside a tag value is never evaluated, and no macro can complete across the comment boundary in either direction.

Only trailing tags in strict `key='value'` form are preserved. Inline tags, plain block comments, and tag-shaped text inside a trailing line comment are still stripped like any other comment. Callers that pre-process queries with `StripComments` directly can call `SplitTrailingSQLCommenter` themselves for the same protection.

Available `CommentStyle` flags:

| Flag | Strips / preserves |
|---|---|
| `LineComment` | Strips `-- …` until end of line |
| `BlockComment` | Strips `/* … */` |
| `HashComment` | Strips `# …` until end of line (MySQL-style) |
| `SlashComment` | Strips `// …` until end of line (Flux-style) |
| `DollarQuote` | Preserves PostgreSQL `$tag$…$tag$` dollar-quoted strings |
| `BacktickQuote` | Preserves MySQL `` `col name` `` backtick identifiers (with `` `` `` escape) |
| `BracketQuote` | Preserves T-SQL `[col name]` bracket identifiers (with `]]` escape) |
| `BackslashEscape` | Treats `\x` as a two-byte escape inside `'…'` and `"…"` — required for MySQL with `NO_BACKSLASH_ESCAPES=OFF` (the default) |

Flags can be combined with `\|`. Pass `0` to strip nothing.

Dialect recipes:

| Dialect | Style |
|---|---|
| Generic SQL (default) | `LineComment \| BlockComment` |
| PostgreSQL | `LineComment \| BlockComment \| DollarQuote` |
| MySQL | `LineComment \| BlockComment \| HashComment \| BacktickQuote \| BackslashEscape` |
| MSSQL / T-SQL | `LineComment \| BlockComment \| BracketQuote` |
| InfluxDB Flux | `SlashComment \| BlockComment` |

## Options

Both the default `$__` prefix and the default comment-stripping set can be overridden per-call via functional options. This is essential for query languages that don't follow Grafana SQL conventions.

```go
// Flux uses "v." as its variable prefix and "//" for line comments.
result, err := macropro.Interpolate(query, fluxMacros, ctx,
    macropro.WithPrefix("v."),
    macropro.WithComments(macropro.SlashComment|macropro.BlockComment),
)

// Legacy InfluxQL bare "$" macros (e.g. $timeFilter, $interval):
result, err := macropro.Interpolate(query, legacyMacros, ctx,
    macropro.WithPrefix("$"),
)
```

Available options:

| Option | Purpose | Default |
|---|---|---|
| `WithPrefix(string)` | Macro prefix to scan for | `"$__"` |
| `WithComments(CommentStyle)` | Comment styles auto-stripped before expansion; `0` disables | `LineComment\|BlockComment` |
| `WithZeroArgMacros(names...)` | Declared macros never consume a following `(...)` — the parens stay in the output as ordinary text | all macros accept argument lists |

`WithZeroArgMacros` matters when token-style macros are embedded in non-SQL bodies (JSON, painless scripts, ES\|QL). There, a `(` after the macro token belongs to the surrounding language, and consuming it as an argument list would silently corrupt the query:

```go
// {"script":"x * $__interval_ms(doc['y'].value)"}
result, err := macropro.Interpolate(body, macros, ctx,
    macropro.WithComments(0),
    macropro.WithZeroArgMacros("interval", "interval_ms"),
)
// {"script":"x * 15000(doc['y'].value)"} — the parens survive
```

If your query language uses more than one prefix family in the same query (as InfluxDB Flux does, with both `$__interval` and `v.timeRangeStart`), call `Interpolate` multiple times with different prefixes. Each call operates on the output of the previous one.

## API reference

```go
// Interpolate replaces all recognised <prefix><name> macros in query.
// Unknown macros are left unchanged. By default, the prefix is "$__" and
// standard SQL comments are stripped. Use WithPrefix / WithComments to
// override. Returns the first handler error encountered, with the macro
// name included.
func Interpolate[T any](query string, macros MacroMap[T], ctx QueryContext[T], opts ...Option) (string, error)

// WithPrefix overrides the macro prefix used when scanning. Default "$__".
func WithPrefix(prefix string) Option

// WithComments overrides the set of comment styles auto-stripped before
// expansion. Default LineComment|BlockComment. Pass 0 to disable stripping.
func WithComments(style CommentStyle) Option

// MergeMacros returns a new MacroMap with overrides merged on top of base.
// For identical names, the override wins. The base map is not mutated.
func MergeMacros[T any](base, overrides MacroMap[T]) MacroMap[T]

// DefaultMacros returns the standard set of Grafana macros with RFC 3339
// timestamps and SQL-flavoured starting-point implementations.
func DefaultMacros[T any]() MacroMap[T]

// StripComments removes comment regions from query, replacing them with
// spaces to preserve byte positions.
func StripComments(query string, style CommentStyle) string

// SplitTrailingSQLCommenter splits a trailing SQLCommenter attribution tag
// off the end of query, returning the body and the tag (or query and "" when
// there is none). style selects which line-comment markers the dialect
// recognises. Interpolate calls this automatically when BlockComment is set.
func SplitTrailingSQLCommenter(query string, style CommentStyle) (string, string)
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

- **Single-pass greedy name reading**: the parser makes one forward pass over the input. At each prefix occurrence it consumes the longest run of identifier characters and then consults the `MacroMap`, so `$__interval_ms` is always matched in full before `$__interval` is ever considered — no longest-first sort required.
- **No-prefix fast paths**: if the prefix does not occur in the input, `Interpolate` returns the original string without allocating or stripping comments. If the prefix only occurs inside a comment, the post-strip check short-circuits before allocating a `strings.Builder`.
- **Bracket-depth argument parsing**: arguments are split by `,` while tracking bracket depth and respecting single- and double-quoted strings, so `$__wrap(COALESCE(a, b), c)` correctly yields two arguments.
- **Panic isolation**: a panic inside a `MacroFunc` is caught and returned as an error. Handlers are a trust boundary; one buggy handler will not crash the caller.
- **Error safety**: if a handler returns an error, `Interpolate` returns the original unmodified query alongside the error.
- **No SQL assumptions**: the parser works on any string; only the default macro implementations produce SQL.

## Security considerations

`macropro` is a string-templating engine. It has no notion of SQL syntax, no notion of identifiers vs. literals, and it does not escape anything. Treat it accordingly.

### Macro arguments are spliced, not escaped

The built-in handlers interpolate arguments directly into their output:

```go
// $__timeFilter(col) expands to:
//   col >= 'from' AND col <= 'to'
return fmt.Sprintf("%s >= '%s' AND %s <= '%s'", col, from, col, to), nil
```

There is no quoting, no identifier validation, and no type checking. If the string passed as `col` contains SQL syntax, that syntax ends up in the final query. The same applies to every custom handler you write — the output string is a raw fragment spliced into the query.

### Trust model

`Interpolate` is safe only if **the query template and macro arguments originate from a trusted author** (typically the dashboard editor). A concrete way to think about it: the strings you pass to `Interpolate` are code, not data.

The library is **not** safe if you:

- Concatenate template-variable values, HTTP parameters, or any other untrusted input into the query string before calling `Interpolate`. The attacker can trivially close a macro argument and inject arbitrary SQL — this is plain SQL injection, not a macropro bug, but the macro layer does nothing to mitigate it.
- Populate `QueryContext.Table`, `QueryContext.Column`, or any `Extra` field used by a handler from unsanitised input. These values are spliced into output with no escaping.
- Accept arbitrary macro definitions from untrusted code. A malicious `MacroFunc` has full control over the rewritten query string.

### Responsibilities by layer

| Layer | Must do |
|---|---|
| Caller of `Interpolate` | Sanitise any untrusted input **before** it reaches the query string or `QueryContext`. Use parameterised queries at the driver level where possible. |
| Handler author | Quote, escape, or validate arguments as required by the target dialect. If an argument is expected to be an identifier, reject or quote non-identifier input. |
| Library | Parse `$__name(args)` tokens correctly, strip comments so hidden macros don't execute, consume the longest valid name at each prefix occurrence, contain handler panics, terminate. Nothing else. |

### Known parser limitations

These are **not** security boundaries, but are worth understanding when reasoning about what the parser treats as a macro vs. what it leaves alone:

- Quote tracking in `StripComments` recognises SQL-standard doubled-quote escapes (`''`, `""`) by default. Backslash escapes (`\'`, `\"`) are only honoured when `BackslashEscape` is set — callers targeting MySQL's default `NO_BACKSLASH_ESCAPES=OFF` mode must include the flag, or a macro token appearing after a backslash-escaped quote can be misparsed.
- Error messages returned by `Interpolate` may include raw argument text from the query. If those errors are logged, treat them as query fragments (potentially sensitive) and scrub accordingly.

## License

Apache 2.0 — see [LICENSE](LICENSE).

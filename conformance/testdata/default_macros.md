# Macro conformance

- From: 2024-01-01T00:00:00Z
- To: 2024-01-02T00:00:00Z
- Interval: 5m0s
- IntervalMS: 300000
- Table: `metrics`
- Column: `value`

| Case | Invocation | Output |
|---|---|---|
| `$__interval` | `$__interval` | `5m` |
| `$__interval()` | `$__interval()` | `5m` |
| `$__interval(x)` | `$__interval(x)` | `5m` |
| `$__interval_ms` | `$__interval_ms` | `300000` |
| `$__interval_ms()` | `$__interval_ms()` | `300000` |
| `$__timeFrom` | `$__timeFrom` | `'2024-01-01T00:00:00Z'` |
| `$__timeFrom()` | `$__timeFrom()` | `'2024-01-01T00:00:00Z'` |
| `$__timeFrom(created_at)` | `$__timeFrom(created_at)` | `created_at >= '2024-01-01T00:00:00Z'` |
| `$__timeFrom(a, b)` | `$__timeFrom(a, b)` | `error: macro $__timeFrom(a, b): $__timeFrom accepts at most 1 argument (column name), got 2` |
| `$__timeTo` | `$__timeTo` | `'2024-01-02T00:00:00Z'` |
| `$__timeTo()` | `$__timeTo()` | `'2024-01-02T00:00:00Z'` |
| `$__timeTo(created_at)` | `$__timeTo(created_at)` | `created_at <= '2024-01-02T00:00:00Z'` |
| `$__timeTo(a, b)` | `$__timeTo(a, b)` | `error: macro $__timeTo(a, b): $__timeTo accepts at most 1 argument (column name), got 2` |
| `$__timeFilter` | `$__timeFilter` | `error: macro $__timeFilter(): $__timeFilter requires exactly 1 argument (column name), got 0` |
| `$__timeFilter()` | `$__timeFilter()` | `error: macro $__timeFilter(): $__timeFilter requires exactly 1 argument (column name), got 0` |
| `$__timeFilter(created_at)` | `$__timeFilter(created_at)` | `created_at >= '2024-01-01T00:00:00Z' AND created_at <= '2024-01-02T00:00:00Z'` |
| `$__timeFilter(COALESCE(a, b))` | `$__timeFilter(COALESCE(a, b))` | `COALESCE(a, b) >= '2024-01-01T00:00:00Z' AND COALESCE(a, b) <= '2024-01-02T00:00:00Z'` |
| `$__timeFilter(a, b)` | `$__timeFilter(a, b)` | `error: macro $__timeFilter(a, b): $__timeFilter requires exactly 1 argument (column name), got 2` |
| `$__timeGroup` | `$__timeGroup` | `error: macro $__timeGroup(): requires 2 arguments (column, interval), got 0` |
| `$__timeGroup()` | `$__timeGroup()` | `error: macro $__timeGroup(): requires 2 arguments (column, interval), got 0` |
| `$__timeGroup(created_at)` | `$__timeGroup(created_at)` | `error: macro $__timeGroup(created_at): requires 2 arguments (column, interval), got 1` |
| `$__timeGroup(created_at, 5m)` | `$__timeGroup(created_at, 5m)` | `FLOOR(UNIX_TIMESTAMP(created_at)/300)*300` |
| `$__timeGroup(created_at, 1d)` | `$__timeGroup(created_at, 1d)` | `FLOOR(UNIX_TIMESTAMP(created_at)/86400)*86400` |
| `$__timeGroup(created_at, $__interval)` | `$__timeGroup(created_at, $__interval)` | `FLOOR(UNIX_TIMESTAMP(created_at)/300)*300` |
| `$__timeGroup(created_at, 5m, 0)` | `$__timeGroup(created_at, 5m, 0)` | `error: macro $__timeGroup(created_at, 5m, 0): requires 2 arguments (column, interval), got 3` |
| `$__table` | `$__table` | `metrics` |
| `$__table()` | `$__table()` | `metrics` |
| `$__column` | `$__column` | `value` |
| `$__column()` | `$__column()` | `value` |
| `full query` | `SELECT $__timeGroup(created_at, $__interval) AS t, avg($__column) FROM $__table WHERE $__timeFilter(created_at) GROUP BY t` | `SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 AS t, avg(value) FROM metrics WHERE created_at >= '2024-01-01T00:00:00Z' AND created_at <= '2024-01-02T00:00:00Z' GROUP BY t` |
| `engine: unknown macro passes through` | `SELECT $__conformanceUnknown(a, b)` | `SELECT $__conformanceUnknown(a, b)` |
| `engine: macro inside a line comment` | `SELECT 1 -- $__column` | ` SELECT 1              ` |
| `engine: macro inside a block comment` | `SELECT /* $__column */ 1` | `SELECT                 1` |
| `engine: trailing SQLCommenter tag` | `SELECT $__column /*traceparent='00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'*/` | `SELECT value /*traceparent='00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01'*/` |

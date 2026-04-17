package macropro

import (
	"strings"
	"testing"
	"time"
)

// Shared fixtures held at package scope so setup cost is not counted in
// benchmark loops and so the compiler cannot eliminate the call.
var (
	benchCtx = QueryContext[struct{}]{
		TimeRange: TimeRange{
			From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		Interval:   5 * time.Minute,
		IntervalMS: 300_000,
		Table:      "metrics",
		Column:     "value",
	}

	benchMacros = DefaultMacros[struct{}]()
)

// Sinks to prevent dead-code elimination.
var (
	sinkString string
	sinkErr    error
)

// BenchmarkInterpolate_shortNoComments is the hot path for typical queries:
// a short query with no comments. Guards the H1 fast path once it lands —
// this benchmark should drop noticeably.
func BenchmarkInterpolate_shortNoComments(b *testing.B) {
	query := "SELECT * FROM $__table WHERE $__timeFilter(created_at)"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkInterpolate_longNoComments tests the H1 fast path at scale.
// ~12 KB query with zero comments — currently allocates a full copy; once
// the no-comment fast path lands, the copy and walk should disappear.
func BenchmarkInterpolate_longNoComments(b *testing.B) {
	query := strings.Repeat(
		"SELECT a, b, c, d FROM table_name WHERE x = 1 AND y = 2 AND z = 3 UNION ALL ",
		160,
	) + "SELECT $__table"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkInterpolate_commentsHeavy exercises the StripComments hot loop.
// Guards H2 (chunked WriteString vs. per-byte WriteByte).
func BenchmarkInterpolate_commentsHeavy(b *testing.B) {
	var sb strings.Builder
	for range 50 {
		sb.WriteString("SELECT 1 -- this is a line comment explaining the query\n")
		sb.WriteString("/* a block comment with some content */ FROM t UNION ALL\n")
	}
	sb.WriteString("SELECT $__timeFilter(ts)")
	query := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkInterpolate_manyMacros stresses the per-name pass structure
// in replaceAllMacro. Guards M1 (single-pass multi-needle scan) — if we
// collapse the N passes into one, this benchmark should drop.
func BenchmarkInterpolate_manyMacros(b *testing.B) {
	macros := MergeMacros(DefaultMacros[struct{}](), MacroMap[struct{}]{
		"syn1": func(_ QueryContext[struct{}], _ []string) (string, error) { return "1", nil },
		"syn2": func(_ QueryContext[struct{}], _ []string) (string, error) { return "2", nil },
		"syn3": func(_ QueryContext[struct{}], _ []string) (string, error) { return "3", nil },
		"syn4": func(_ QueryContext[struct{}], _ []string) (string, error) { return "4", nil },
		"syn5": func(_ QueryContext[struct{}], _ []string) (string, error) { return "5", nil },
		"syn6": func(_ QueryContext[struct{}], _ []string) (string, error) { return "6", nil },
		"syn7": func(_ QueryContext[struct{}], _ []string) (string, error) { return "7", nil },
		"syn8": func(_ QueryContext[struct{}], _ []string) (string, error) { return "8", nil },
	})
	query := "SELECT $__syn1, $__syn5 FROM $__table WHERE $__timeFilter(ts) GROUP BY $__timeGroup(ts, 5m)"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, macros, benchCtx)
	}
}

// BenchmarkInterpolate_hotLoop simulates a server handling many queries with
// the same MacroMap. Guards M2 — if we introduce a Compile step, this
// benchmark should drop because per-call sort+names rebuild goes away.
func BenchmarkInterpolate_hotLoop(b *testing.B) {
	query := "SELECT $__table FROM x WHERE $__timeFilter(ts) GROUP BY $__timeGroup(ts, 5m)"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkInterpolate_realistic mixes realistic input: a medium-size query
// with a handful of comments and multiple macro expansions. This is the
// closest approximation to production load.
func BenchmarkInterpolate_realistic(b *testing.B) {
	query := `-- dashboard: cpu-by-host
SELECT
    $__timeGroup(ts, 5m) AS time,
    host,
    avg(cpu) AS cpu
FROM $__table
WHERE $__timeFilter(ts)
  /* exclude synthetic agents */
  AND host NOT LIKE 'synth-%'
GROUP BY time, host
ORDER BY time`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkStripComments_noComments isolates the H1 fast path from
// Interpolate overhead. Should drop to near zero once the fast path lands.
func BenchmarkStripComments_noComments(b *testing.B) {
	query := strings.Repeat("SELECT a, b, c FROM t WHERE x = 1 AND y = 2 ", 200)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString = StripComments(query, LineComment|BlockComment)
	}
}

// BenchmarkStripComments_heavy isolates the H2 byte-walk cost.
func BenchmarkStripComments_heavy(b *testing.B) {
	var sb strings.Builder
	for range 100 {
		sb.WriteString("SELECT 1 -- long-ish comment to exercise the line scanner\n")
	}
	query := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString = StripComments(query, LineComment|BlockComment)
	}
}

// BenchmarkInterpolate_noPrefix is the dominant real-world case for
// datasources that also accept raw SQL without Grafana variables: a query
// that contains no $__ anywhere. Guards M2 (no-prefix fast path) — should
// short-circuit before StripComments and before allocating a Builder.
func BenchmarkInterpolate_noPrefix(b *testing.B) {
	query := strings.Repeat(
		"SELECT column_a, column_b FROM some_table WHERE key = 'value' ",
		20,
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkInterpolate_prefixInCommentOnly exercises the second M2 fast
// path: the only $__ occurrence is hidden inside a comment, so after
// stripping there is nothing to expand and the Builder allocation can be
// skipped.
func BenchmarkInterpolate_prefixInCommentOnly(b *testing.B) {
	query := "-- dashboard uses $__timeFilter(ts) for the window\n" +
		"SELECT a, b FROM t WHERE k = 'v' ORDER BY a LIMIT 10"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString, sinkErr = Interpolate(query, benchMacros, benchCtx)
	}
}

// BenchmarkStripComments_longNoComments is the no-comment scenario at scale.
// Currently walks every byte; H1 should short-circuit to a no-op.
func BenchmarkStripComments_longNoComments(b *testing.B) {
	query := strings.Repeat(
		"SELECT column_a, column_b FROM some_table WHERE key = 'value' AND n = 42 ",
		200,
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkString = StripComments(query, LineComment|BlockComment)
	}
}

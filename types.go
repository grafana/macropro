package macropro

import "time"

// TimeRange represents the time boundaries of a Grafana dashboard query.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// QueryContext holds the runtime values available to every macro handler.
//
// T is a datasource-specific extension type. Use struct{} if the datasource
// does not need additional context fields.
type QueryContext[T any] struct {
	TimeRange  TimeRange
	Interval   time.Duration
	IntervalMS int64
	Table      string
	Column     string
	// Extra holds any datasource-specific context. It is passed verbatim to
	// every MacroFunc, providing type-safe access to fields that are not part
	// of the common set above.
	Extra T
}

// MacroFunc is the function signature for a single macro handler.
//
// ctx is the query context. args contains the already-parsed,
// whitespace-trimmed arguments from inside the macro's parentheses (empty
// slice for zero-arg macros). The function returns the string that will
// replace the entire $__macroName(…) token in the query, or an error.
//
// Errors are returned immediately by Interpolate with the macro name and raw
// argument list included in the error message.
type MacroFunc[T any] func(ctx QueryContext[T], args []string) (string, error)

// MacroMap maps macro names (without the $__ prefix) to their handler.
// For example, to handle $__timeFilter, use key "timeFilter".
type MacroMap[T any] map[string]MacroFunc[T]

// CommentStyle is a bitmask that controls which SQL comment styles are
// recognised by StripComments.
type CommentStyle uint

const (
	// LineComment strips -- line comments.
	LineComment CommentStyle = 1 << iota
	// BlockComment strips /* … */ block comments.
	BlockComment
	// HashComment strips # line comments (MySQL-style).
	HashComment
	// DollarQuote preserves PostgreSQL $tag$…$tag$ dollar-quoted string
	// literals so they are not mistaken for macro tokens.
	DollarQuote
	// SlashComment strips // line comments (Flux-style).
	SlashComment
)

// Option configures the behaviour of [Interpolate]. Options are applied in
// order and the last one wins for conflicting settings.
type Option func(*options)

// options is the resolved configuration for a single Interpolate call.
type options struct {
	prefix   string
	comments CommentStyle
}

// WithPrefix overrides the macro prefix used when scanning the query. The
// default is "$__". Non-SQL query languages may need a different prefix — for
// example, InfluxDB Flux uses "v." (as in v.timeRangeStart) and legacy
// InfluxQL uses bare "$" (as in $timeFilter).
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithComments overrides the set of comment styles that [Interpolate]
// automatically strips from the query before macro expansion. The default is
// LineComment|BlockComment, matching standard SQL. Pass 0 to disable
// auto-stripping, or a different bitmask (e.g. SlashComment|BlockComment for
// Flux) to customise it.
func WithComments(style CommentStyle) Option {
	return func(o *options) { o.comments = style }
}

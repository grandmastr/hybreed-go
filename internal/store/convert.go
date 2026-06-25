package store

// Hand-written helpers (NOT generated) for converting between Go values and the
// pgtype wrappers that sqlc emits for nullable / numeric / temporal columns.

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Ptr returns a pointer to v — handy for sqlc's nullable (narg) parameters.
func Ptr[T any](v T) *T { return &v }

// Num builds a pgtype.Numeric from a float64 (2-decimal precision is plenty for
// body weight / macros).
func Num(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(f, 'f', 2, 64))
	return n
}

// Float reads a pgtype.Numeric back into a float64 (0 when NULL/invalid).
func Float(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	v, err := n.Float64Value()
	if err != nil {
		return 0
	}
	return v.Float64
}

// TS wraps a time.Time as a valid pgtype.Timestamptz.
func TS(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// TimeOf reads a pgtype.Timestamptz (zero time when invalid).
func TimeOf(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

// DateOf wraps a time.Time as a valid pgtype.Date (date portion only).
func DateOf(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// DateValue reads a pgtype.Date back into a time.Time.
func DateValue(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

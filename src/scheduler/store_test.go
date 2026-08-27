package scheduler

import (
	"database/sql"
	"time"
)

// sqlNullTime is a small test helper for building a valid taskRow
// timestamp field without importing database/sql into every test.
func sqlNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}

package store

import "database/sql"

// NullInt64 把 0 当作 NULL，用于可空外键。
func NullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

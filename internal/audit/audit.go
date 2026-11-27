// Package audit 维护 append-only 的审计哈希链（design.md 第 16.3 节）。
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// Entry 一条待追加的审计记录。
type Entry struct {
	TS     int64
	Actor  string // admin:1 / agent:3 / open:key1
	Action string
	Target string
	Before string // JSON 或空
	After  string
	IP     string
}

// Append 在 tx 内取链尾、算哈希并插入。必须与业务写在同一事务。
func Append(ctx context.Context, tx *sql.Tx, e Entry) error {
	var prev string
	err := tx.QueryRowContext(ctx,
		`SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prev)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("读审计链尾: %w", err)
	}
	h := digest(e, prev)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_log(ts, actor, action, target, before, after, ip, prev_hash, hash)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TS, e.Actor, e.Action, e.Target, e.Before, e.After, e.IP, prev, h)
	return err
}

// digest 用 NUL 分隔各字段，避免拼接歧义。
func digest(e Entry, prev string) string {
	w := sha256.New()
	for _, f := range []string{
		prev, fmt.Sprint(e.TS), e.Actor, e.Action, e.Target, e.Before, e.After, e.IP,
	} {
		w.Write([]byte(f))
		w.Write([]byte{0})
	}
	return hex.EncodeToString(w.Sum(nil))
}

// Verify 从创世重算整链，返回检查的条数。断链时返回断点 id。
func Verify(ctx context.Context, db *sql.DB) (int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, ts, actor, action, COALESCE(target,''), COALESCE(before,''),
		        COALESCE(after,''), COALESCE(ip,''), prev_hash, hash
		   FROM audit_log ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var (
		n    int64
		prev string
	)
	for rows.Next() {
		var (
			id           int64
			e            Entry
			gotPrev, got string
		)
		if err := rows.Scan(&id, &e.TS, &e.Actor, &e.Action, &e.Target,
			&e.Before, &e.After, &e.IP, &gotPrev, &got); err != nil {
			return n, err
		}
		if gotPrev != prev {
			return n, fmt.Errorf("审计链在 id=%d 断开：prev_hash 不匹配", id)
		}
		if want := digest(e, prev); want != got {
			return n, fmt.Errorf("审计链在 id=%d 断开：hash 不匹配", id)
		}
		prev = got
		n++
	}
	return n, rows.Err()
}

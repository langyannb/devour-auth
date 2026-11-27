package store

import "context"

type AuditRow struct {
	ID     int64
	TS     int64
	Actor  string
	Action string
	Target string
	Before string
	After  string
	IP     string
}

// ListAudit 倒序分页读取审计，供后台「审计」页与校验前浏览。
func (s *Store) ListAudit(ctx context.Context, limit, offset int) ([]AuditRow, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, actor, action, COALESCE(target,''), COALESCE(before,''), COALESCE(after,''), COALESCE(ip,'')
		   FROM audit_log ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AuditRow
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.TS, &a.Actor, &a.Action, &a.Target, &a.Before, &a.After, &a.IP); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

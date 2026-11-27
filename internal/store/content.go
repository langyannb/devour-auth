package store

import (
	"context"
	"database/sql"
	"errors"
)

// ---------- 版本 ----------

type Release struct {
	ID        int64
	AppID     int64
	Version   string
	Info      string
	URL       string
	Must      int64
	RolloutBP int64
	Active    int64
	CreatedAt int64
}

func (s *Store) CreateRelease(ctx context.Context, r Release) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO releases(app_id, version, info, url, must, rollout_bp, active, created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		r.AppID, r.Version, r.Info, r.URL, r.Must, r.RolloutBP, r.Active, r.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListReleases(ctx context.Context, appID int64) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, version, COALESCE(info,''), COALESCE(url,''), must, rollout_bp, active, created_at
		   FROM releases WHERE app_id = ? ORDER BY id DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Release
	for rows.Next() {
		var r Release
		if err := rows.Scan(&r.ID, &r.AppID, &r.Version, &r.Info, &r.URL, &r.Must, &r.RolloutBP, &r.Active, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestRelease 取最新启用的版本；rollout_bp < 10000 时由调用方按设备灰度。
func (s *Store) LatestRelease(ctx context.Context, appID int64) (Release, error) {
	var r Release
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, version, COALESCE(info,''), COALESCE(url,''), must, rollout_bp, active, created_at
		   FROM releases WHERE app_id = ? AND active = 1 ORDER BY id DESC LIMIT 1`, appID).
		Scan(&r.ID, &r.AppID, &r.Version, &r.Info, &r.URL, &r.Must, &r.RolloutBP, &r.Active, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	return r, err
}

// UpdateRelease 改版本内容（不改 app_id 和发布时间）；appID 限定只能改本应用的记录。
func (s *Store) UpdateRelease(ctx context.Context, appID, id int64, r Release) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE releases SET version = ?, info = ?, url = ?, must = ?, rollout_bp = ? WHERE id = ? AND app_id = ?`,
		r.Version, r.Info, r.URL, r.Must, r.RolloutBP, id, appID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteRelease 删掉一条版本记录（同样限定应用）。
func (s *Store) DeleteRelease(ctx context.Context, appID, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM releases WHERE id = ? AND app_id = ?`, id, appID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) SetReleaseActive(ctx context.Context, id int64, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE releases SET active = ? WHERE id = ?`, v, id)
	return err
}

// ---------- 公告 ----------

type Notice struct {
	ID        int64
	AppID     int64
	Kind      string // list | popup | immediate
	Title     string
	Body      string
	Active    int64
	CreatedAt int64
}

func (s *Store) CreateNotice(ctx context.Context, n Notice) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notices(app_id, kind, title, body, active, created_at) VALUES(?,?,?,?,?,?)`,
		n.AppID, n.Kind, n.Title, n.Body, n.Active, n.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListNotices(ctx context.Context, appID int64, onlyActive bool) ([]Notice, error) {
	q := `SELECT id, app_id, kind, COALESCE(title,''), body, active, created_at FROM notices WHERE app_id = ?`
	if onlyActive {
		q += ` AND active = 1`
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.QueryContext(ctx, q, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notice
	for rows.Next() {
		var n Notice
		if err := rows.Scan(&n.ID, &n.AppID, &n.Kind, &n.Title, &n.Body, &n.Active, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateNotice 改公告内容；appID 限定只能改本应用的记录。
func (s *Store) UpdateNotice(ctx context.Context, appID, id int64, n Notice) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notices SET kind = ?, title = ?, body = ? WHERE id = ? AND app_id = ?`,
		n.Kind, n.Title, n.Body, id, appID)
	if err != nil {
		return false, err
	}
	c, err := res.RowsAffected()
	return c > 0, err
}

// DeleteNotice 删掉一条公告（同样限定应用）。
func (s *Store) DeleteNotice(ctx context.Context, appID, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notices WHERE id = ? AND app_id = ?`, id, appID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) SetNoticeActive(ctx context.Context, id int64, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notices SET active = ? WHERE id = ?`, v, id)
	return err
}

// ---------- 远程变量 ----------

func (s *Store) ListFlags(ctx context.Context, appID int64) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM flags WHERE app_id = ? ORDER BY key`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetFlag(ctx context.Context, appID int64, key, value string, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO flags(app_id, key, value, updated_at) VALUES(?,?,?,?)
		 ON CONFLICT(app_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		appID, key, value, now)
	return err
}

// ListFlagsTx 事务内读应用开关，避免和单连接事务互等。
func (s *Store) ListFlagsTx(ctx context.Context, tx *sql.Tx, appID int64) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT key, value FROM flags WHERE app_id = ? ORDER BY key`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

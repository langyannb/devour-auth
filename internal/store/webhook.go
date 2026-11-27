package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Webhook struct {
	ID        int64
	AppID     int64
	URL       string
	Secret    string
	Events    string
	Active    int64
	CreatedAt int64
}

func (s *Store) ListWebhooks(ctx context.Context, appID int64) ([]Webhook, error) {
	q := `SELECT id, app_id, url, secret, events, active, created_at FROM webhooks`
	var args []any
	if appID > 0 {
		q += ` WHERE app_id = ?`
		args = append(args, appID)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.AppID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) CreateWebhook(ctx context.Context, w Webhook) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO webhooks(app_id, url, secret, events, active, created_at) VALUES(?,?,?,?,?,?)`,
		w.AppID, w.URL, w.Secret, w.Events, w.Active, w.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) SetWebhookActive(ctx context.Context, id int64, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE webhooks SET active = ? WHERE id = ?`, v, id)
	return err
}

func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	return err
}

// WebhooksFor 取该应用订阅了某事件的启用回调。
func (s *Store) WebhooksFor(ctx context.Context, appID int64, event string) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, url, secret, events, active, created_at
		   FROM webhooks WHERE app_id = ? AND active = 1`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.AppID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt); err != nil {
			return nil, err
		}
		for _, e := range strings.Split(w.Events, ",") {
			if strings.TrimSpace(e) == event {
				out = append(out, w)
				break
			}
		}
	}
	return out, rows.Err()
}

var _ = errors.Is
var _ = sql.ErrNoRows

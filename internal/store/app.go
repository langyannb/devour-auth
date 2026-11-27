package store

import (
	"context"
	"database/sql"
	"errors"
)

type App struct {
	ID        int64
	Code      string
	Name      string
	Active    int64
	CreatedAt int64
}

func (s *Store) CreateApp(ctx context.Context, code, name string, now int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO apps(code, name, active, created_at) VALUES(?, ?, 1, ?)`, code, name, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AppByCode(ctx context.Context, code string) (App, error) {
	var a App
	err := s.db.QueryRowContext(ctx,
		`SELECT id, code, name, active, created_at FROM apps WHERE code = ?`, code).
		Scan(&a.ID, &a.Code, &a.Name, &a.Active, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrNotFound
	}
	return a, err
}

func (s *Store) AppByID(ctx context.Context, id int64) (App, error) {
	var a App
	err := s.db.QueryRowContext(ctx,
		`SELECT id, code, name, active, created_at FROM apps WHERE id = ?`, id).
		Scan(&a.ID, &a.Code, &a.Name, &a.Active, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrNotFound
	}
	return a, err
}

func (s *Store) ListApps(ctx context.Context) ([]App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, code, name, active, created_at FROM apps ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Active, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateApp 改名称或启停。
func (s *Store) UpdateApp(ctx context.Context, id int64, name string, active int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE apps SET name = ?, active = ? WHERE id = ?`, name, active, id)
	return err
}

// EnsureApp 幂等创建，供首个应用（devour）在启动或初始化时落库。
func (s *Store) EnsureApp(ctx context.Context, code, name string, now int64) (int64, error) {
	a, err := s.AppByCode(ctx, code)
	if err == nil {
		return a.ID, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return 0, err
	}
	return s.CreateApp(ctx, code, name, now)
}

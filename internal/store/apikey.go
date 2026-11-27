package store

import (
	"context"
	"database/sql"
	"errors"
)

// APIKey 开放接口密钥。
//
// 注意：HMAC-SHA256 验签需要密钥原文，只存哈希无法验签，所以 secret 以原文
// 存放在 secret_hash 列（仅创建时返回给调用方一次）。design.md 第 13 节已同步修正。
type APIKey struct {
	ID        int64
	Name      string
	Scopes    string
	KeyID     string
	Secret    string
	LastUsed  sql.NullInt64
	Status    string
	CreatedAt int64
}

func (s *Store) CreateAPIKey(ctx context.Context, k APIKey) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys(name, scopes, key_id, secret_hash, status, created_at)
		 VALUES(?,?,?,?,?,?)`,
		k.Name, k.Scopes, k.KeyID, k.Secret, k.Status, k.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) APIKeyByKeyID(ctx context.Context, keyID string) (APIKey, error) {
	var k APIKey
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, scopes, key_id, secret_hash, last_used, status, created_at
		   FROM api_keys WHERE key_id = ?`, keyID).
		Scan(&k.ID, &k.Name, &k.Scopes, &k.KeyID, &k.Secret, &k.LastUsed, &k.Status, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	return k, err
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, scopes, key_id, secret_hash, last_used, status, created_at
		   FROM api_keys ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Scopes, &k.KeyID, &k.Secret, &k.LastUsed, &k.Status, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) SetAPIKeyStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) TouchAPIKey(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used = ? WHERE id = ?`, now, id)
	return err
}

// ActiveSessionCount 当前在线会话数（refresh 未过期且未吊销）。
func (s *Store) ActiveSessionCount(ctx context.Context, now int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL AND refresh_exp > ?`, now).Scan(&n)
	return n, err
}

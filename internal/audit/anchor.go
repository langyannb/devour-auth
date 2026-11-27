package audit

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadKey 只读取已有私钥，不生成。
func LoadKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("审计私钥长度不对：%d", len(b))
	}
	return ed25519.PrivateKey(b), nil
}

// LoadOrCreateKey 读取 Ed25519 私钥；不存在时生成并以 0600 落盘。
// 私钥只在本地文件中，不入库、不入仓库（AGENTS.md 2.1）。
func LoadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	priv, err := LoadKey(path)
	if err == nil {
		return priv, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, priv, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}

func anchorMessage(day string, lastID int64, lastHash string) []byte {
	return []byte(fmt.Sprintf("%s:%d:%s", day, lastID, lastHash))
}

// Anchor 对当前链尾签名并写入 audit_anchor；同一天已锚定则跳过。
func Anchor(ctx context.Context, db *sql.DB, priv ed25519.PrivateKey, day string, now int64) (bool, error) {
	var lastID int64
	var lastHash string
	err := db.QueryRowContext(ctx,
		`SELECT id, hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&lastID, &lastHash)
	if errors.Is(err, sql.ErrNoRows) {
		lastID, lastHash = 0, ""
	} else if err != nil {
		return false, err
	}
	sig := ed25519.Sign(priv, anchorMessage(day, lastID, lastHash))
	res, err := db.ExecContext(ctx,
		`INSERT INTO audit_anchor(day, last_id, last_hash, signature, created_at)
		 VALUES(?,?,?,?,?) ON CONFLICT(day) DO NOTHING`,
		day, lastID, lastHash, sig, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// VerifyAnchors 校验每个锚点的签名，并确认锚点记录的链尾确实还在链上。
func VerifyAnchors(ctx context.Context, db *sql.DB, pub ed25519.PublicKey) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT day, last_id, last_hash, signature FROM audit_anchor ORDER BY day`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type anchor struct {
		day      string
		lastID   int64
		lastHash string
		sig      []byte
	}
	var anchors []anchor
	for rows.Next() {
		var a anchor
		if err := rows.Scan(&a.day, &a.lastID, &a.lastHash, &a.sig); err != nil {
			return 0, err
		}
		anchors = append(anchors, a)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, a := range anchors {
		if !ed25519.Verify(pub, anchorMessage(a.day, a.lastID, a.lastHash), a.sig) {
			return 0, fmt.Errorf("锚点 %s 签名无效", a.day)
		}
		if a.lastID == 0 {
			continue
		}
		var onChain string
		err := db.QueryRowContext(ctx,
			`SELECT hash FROM audit_log WHERE id = ?`, a.lastID).Scan(&onChain)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("锚点 %s 指向的记录 id=%d 已不存在", a.day, a.lastID)
		}
		if err != nil {
			return 0, err
		}
		if onChain != a.lastHash {
			return 0, fmt.Errorf("锚点 %s 记录的链尾哈希不匹配", a.day)
		}
	}
	return len(anchors), nil
}

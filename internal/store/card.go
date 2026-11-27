package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// 卡状态（design.md 7.6）。
const (
	CardUnused    = "unused"
	CardActive    = "active"
	CardExpired   = "expired"
	CardExhausted = "exhausted"
	CardBanned    = "banned"
	CardRecycled  = "recycled"
)

type Card struct {
	ID               int64
	AppID            int64
	Kami             string
	PolicyID         int64
	Kind             string
	Status           string
	BatchID          sql.NullInt64
	AgentID          sql.NullInt64
	DeviceLimit      int64
	ExpireAt         int64
	RemainTimes      int64
	Points           int64
	RemainingSeconds int64
	Paused           int64
	RebindUsed       int64
	SnapshotJSON     string
	Note             string
	ActivatedAt      sql.NullInt64
	BannedAt         sql.NullInt64
	CreatedAt        int64
}

const cardCols = `id, app_id, kami, policy_id, kind, status, batch_id, agent_id,
  device_limit, expire_at, remain_times, points, remaining_seconds,
  paused, rebind_used, snapshot_json, COALESCE(note,''), activated_at, banned_at, created_at`

func scanCard(sc interface{ Scan(...any) error }) (Card, error) {
	var c Card
	err := sc.Scan(
		&c.ID, &c.AppID, &c.Kami, &c.PolicyID, &c.Kind, &c.Status, &c.BatchID, &c.AgentID,
		&c.DeviceLimit, &c.ExpireAt, &c.RemainTimes, &c.Points, &c.RemainingSeconds,
		&c.Paused, &c.RebindUsed, &c.SnapshotJSON, &c.Note, &c.ActivatedAt, &c.BannedAt, &c.CreatedAt,
	)
	return c, err
}

// Executor 让同一段 SQL 可在 *sql.DB 或 *sql.Tx 上执行。
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertCard(ctx context.Context, ex Executor, c Card) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`INSERT INTO cards(app_id, kami, policy_id, kind, status, batch_id, agent_id,
		   device_limit, expire_at, remain_times, points, remaining_seconds,
		   paused, rebind_used, snapshot_json, note, activated_at, banned_at, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AppID, c.Kami, c.PolicyID, c.Kind, c.Status, c.BatchID, c.AgentID,
		c.DeviceLimit, c.ExpireAt, c.RemainTimes, c.Points, c.RemainingSeconds,
		c.Paused, c.RebindUsed, c.SnapshotJSON, c.Note, c.ActivatedAt, c.BannedAt, c.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertCard 在给定事务内插入一张卡，供发卡批次使用。
func (s *Store) InsertCard(ctx context.Context, tx *sql.Tx, c Card) (int64, error) {
	return insertCard(ctx, tx, c)
}

func updateCard(ctx context.Context, ex Executor, c Card) error {
	_, err := ex.ExecContext(ctx,
		`UPDATE cards SET status=?, device_limit=?, expire_at=?, remain_times=?, points=?,
		   remaining_seconds=?, paused=?, rebind_used=?, note=?, activated_at=?, banned_at=?
		 WHERE id = ?`,
		c.Status, c.DeviceLimit, c.ExpireAt, c.RemainTimes, c.Points,
		c.RemainingSeconds, c.Paused, c.RebindUsed, c.Note, c.ActivatedAt, c.BannedAt, c.ID)
	return err
}

// UpdateCard 覆盖可变字段：状态、余额、暂停、续期、换绑计数、备注。
func (s *Store) UpdateCard(ctx context.Context, c Card) error {
	return updateCard(ctx, s.db, c)
}

// CardTx 在事务内读卡，配合 UpdateCardTx 做读-改-写。
func (s *Store) CardTx(ctx context.Context, tx *sql.Tx, id int64) (Card, error) {
	c, err := scanCard(tx.QueryRowContext(ctx,
		`SELECT `+cardCols+` FROM cards WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	return c, err
}

func (s *Store) UpdateCardTx(ctx context.Context, tx *sql.Tx, c Card) error {
	return updateCard(ctx, tx, c)
}

func (s *Store) CardByID(ctx context.Context, id int64) (Card, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`SELECT `+cardCols+` FROM cards WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	return c, err
}

func (s *Store) CardByKami(ctx context.Context, kami string) (Card, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`SELECT `+cardCols+` FROM cards WHERE kami = ?`, kami))
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	return c, err
}

type CardFilter struct {
	AppID    int64
	Status   string
	PolicyID int64
	Kami     string
	AgentID  int64
	Limit    int
	Offset   int
}

func (f *CardFilter) where() (string, []any) {
	var (
		conds []string
		args  []any
	)
	if f.AppID > 0 {
		conds = append(conds, "app_id = ?")
		args = append(args, f.AppID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	if f.PolicyID > 0 {
		conds = append(conds, "policy_id = ?")
		args = append(args, f.PolicyID)
	}
	if f.Kami != "" {
		conds = append(conds, "kami LIKE ?")
		args = append(args, "%"+f.Kami+"%")
	}
	if f.AgentID > 0 {
		conds = append(conds, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (s *Store) ListCards(ctx context.Context, f CardFilter) ([]Card, int64, error) {
	where, args := f.where()
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cards`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cardCols+` FROM cards`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// UnbindDevices 清空该卡全部绑定；unbound_at 保留历史。
func (s *Store) UnbindDevices(ctx context.Context, ex Executor, cardID, now int64) (int64, error) {
	res, err := ex.ExecContext(ctx,
		`UPDATE devices SET unbound_at = ? WHERE card_id = ? AND unbound_at IS NULL`, now, cardID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) CountBoundDevices(ctx context.Context, cardID int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices WHERE card_id = ? AND unbound_at IS NULL`, cardID).Scan(&n)
	return n, err
}

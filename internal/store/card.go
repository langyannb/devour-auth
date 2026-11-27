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
	RebindAt         int64
	SnapshotJSON     string
	Note             string
	ActivatedAt      sql.NullInt64
	BannedAt         sql.NullInt64
	CreatedAt        int64
	VerifyDevice     int64 // 0 = 这张卡不校验设备上限
}

const cardCols = `id, app_id, kami, policy_id, kind, status, batch_id, agent_id,
  device_limit, expire_at, remain_times, points, remaining_seconds,
  paused, rebind_used, rebind_at, snapshot_json, COALESCE(note,''), activated_at, banned_at, created_at, verify_device`

func scanCard(sc interface{ Scan(...any) error }) (Card, error) {
	var c Card
	err := sc.Scan(
		&c.ID, &c.AppID, &c.Kami, &c.PolicyID, &c.Kind, &c.Status, &c.BatchID, &c.AgentID,
		&c.DeviceLimit, &c.ExpireAt, &c.RemainTimes, &c.Points, &c.RemainingSeconds,
		&c.Paused, &c.RebindUsed, &c.RebindAt, &c.SnapshotJSON, &c.Note, &c.ActivatedAt, &c.BannedAt, &c.CreatedAt, &c.VerifyDevice,
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
		   paused, rebind_used, snapshot_json, note, activated_at, banned_at, created_at, verify_device)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.AppID, c.Kami, c.PolicyID, c.Kind, c.Status, c.BatchID, c.AgentID,
		c.DeviceLimit, c.ExpireAt, c.RemainTimes, c.Points, c.RemainingSeconds,
		c.Paused, c.RebindUsed, c.SnapshotJSON, c.Note, c.ActivatedAt, c.BannedAt, c.CreatedAt, c.VerifyDevice)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertCard 在给定事务内插入一张卡，供发卡批次使用。
func (s *Store) InsertCard(ctx context.Context, tx *sql.Tx, c Card) (int64, error) {
	// 新卡默认校验设备；关掉只能通过 SetVerifyDevice 单独设置
	if c.VerifyDevice == 0 {
		c.VerifyDevice = 1
	}
	return insertCard(ctx, tx, c)
}

func updateCard(ctx context.Context, ex Executor, c Card) error {
	_, err := ex.ExecContext(ctx,
		`UPDATE cards SET status=?, device_limit=?, expire_at=?, remain_times=?, points=?,
		   remaining_seconds=?, paused=?, rebind_used=?, rebind_at=?, snapshot_json=?,
		   note=?, activated_at=?, banned_at=?, verify_device=?
		 WHERE id = ?`,
		c.Status, c.DeviceLimit, c.ExpireAt, c.RemainTimes, c.Points,
		c.RemainingSeconds, c.Paused, c.RebindUsed, c.RebindAt, c.SnapshotJSON,
		c.Note, c.ActivatedAt, c.BannedAt, c.VerifyDevice, c.ID)
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

func (s *Store) CardByKamiTx(ctx context.Context, tx *sql.Tx, appID int64, kami string) (Card, error) {
	q := `SELECT ` + cardCols + ` FROM cards WHERE kami = ?`
	args := []any{kami}
	if appID > 0 {
		q += ` AND app_id = ?`
		args = append(args, appID)
	}
	c, err := scanCard(tx.QueryRowContext(ctx, q, args...))
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
	// AgentIDs 非空时按 IN 过滤（代理只能看自己与下级）
	AgentIDs []int64
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
	if len(f.AgentIDs) > 0 {
		ph := make([]string, len(f.AgentIDs))
		for i, id := range f.AgentIDs {
			ph[i] = "?"
			args = append(args, id)
		}
		conds = append(conds, "agent_id IN ("+strings.Join(ph, ",")+")")
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

// BoundDeviceCounts 一批卡各自绑定的设备数，列表页一次查完。
func (s *Store) BoundDeviceCounts(ctx context.Context, cardIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(cardIDs))
	if len(cardIDs) == 0 {
		return out, nil
	}
	q := `SELECT card_id, COUNT(*) FROM devices
	       WHERE unbound_at IS NULL AND card_id IN (?` + repeatComma(len(cardIDs)-1) + `)
	       GROUP BY card_id`
	args := make([]any, len(cardIDs))
	for i, id := range cardIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// OnlineDeviceCounts 一批卡各有多少台设备在线：access token 未过期的会话。
func (s *Store) OnlineDeviceCounts(ctx context.Context, cardIDs []int64, now int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(cardIDs))
	if len(cardIDs) == 0 {
		return out, nil
	}
	q := `SELECT card_id, COUNT(DISTINCT device_id) FROM sessions
	       WHERE revoked_at IS NULL AND access_exp > ? AND card_id IN (?` + repeatComma(len(cardIDs)-1) + `)
	       GROUP BY card_id`
	args := make([]any, 0, len(cardIDs)+1)
	args = append(args, now)
	for _, id := range cardIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// UnbindDevicesByIDs 解绑指定设备；ids 为空时解绑全部。返回实际解绑数。
func (s *Store) UnbindDevicesByIDs(ctx context.Context, tx *sql.Tx, cardID int64, ids []int64, now int64) (int64, error) {
	if len(ids) == 0 {
		return s.UnbindDevices(ctx, tx, cardID, now)
	}
	q := `UPDATE devices SET unbound_at = ? WHERE card_id = ? AND unbound_at IS NULL AND id IN (?` +
		repeatComma(len(ids)-1) + `)`
	args := make([]any, 0, len(ids)+2)
	args = append(args, now, cardID)
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListBoundDevices 一张卡当前绑定的设备（解绑页用，不含已解绑）。
func (s *Store) ListBoundDevices(ctx context.Context, cardID int64) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE card_id = ? AND unbound_at IS NULL ORDER BY id`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.CardID, &d.Fingerprint, &d.Model, &d.Brand, &d.OS,
			&d.AppVersion, &d.FirstSeen, &d.LastSeen, &d.Banned, &d.UnboundAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

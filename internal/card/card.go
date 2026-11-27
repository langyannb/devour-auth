// Package card 实现卡密生成、策略快照与状态流转（design.md 第 7、9 节）。
package card

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/store"
)

// MaxBatchQty 单批发卡上限（design.md 第 9 节）。
const MaxBatchQty = 500

// kamiAlphabet 去掉易混字符；32 个字符正好整除 256，取模无偏。
const kamiAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

var (
	ErrInvalid = errors.New("请求不合法")
	ErrState   = errors.New("当前状态不允许该操作")
)

// NewKami 生成「前缀-XXXX-XXXX-XXXX」格式的卡密。
func NewKami(prefix string) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	if p := strings.Trim(strings.TrimSpace(prefix), "-"); p != "" {
		sb.WriteString(p)
		sb.WriteByte('-')
	}
	for i := 0; i < 12; i++ {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteString(string(kamiAlphabet[int(b[i])%len(kamiAlphabet)]))
	}
	return sb.String(), nil
}

// Snapshot 出卡时冻结的策略副本，之后改 policy 不影响已生成的卡。
type Snapshot struct {
	PolicyID           int64    `json:"policy_id"`
	PolicyCode         string   `json:"policy_code"`
	Kind               string   `json:"kind"`
	DurationDays       int64    `json:"duration_days"`
	DurationHours      int64    `json:"duration_hours"`
	DurationSeconds    int64    `json:"duration_seconds"`
	InitialTimes       int64    `json:"initial_times"`
	InitialPoints      int64    `json:"initial_points"`
	DeviceLimit        int64    `json:"device_limit"`
	ConcurrentLimit    int64    `json:"concurrent_limit"`
	HeartbeatSec       int64    `json:"heartbeat_sec"`
	OfflineHours       int64    `json:"offline_hours"`
	OfflineStarts      int64    `json:"offline_starts"`
	RebindFree         int64    `json:"rebind_free"`
	RebindCooldownDays int64    `json:"rebind_cooldown_days"`
	RebindCostHours    int64    `json:"rebind_cost_hours"`
	Renewable          bool     `json:"renewable"`
	TrialPerDevice     int64    `json:"trial_per_device"`
	PriceCents         int64    `json:"price_cents"`
	Entitlements       []string `json:"entitlements"`
}

func BuildSnapshot(p store.Policy, entitlementCodes []string) Snapshot {
	if entitlementCodes == nil {
		entitlementCodes = []string{}
	}
	return Snapshot{
		PolicyID:           p.ID,
		PolicyCode:         p.Code,
		Kind:               p.Kind,
		DurationDays:       p.DurationDays,
		DurationHours:      p.DurationHours,
		DurationSeconds:    p.DurationSeconds,
		InitialTimes:       p.InitialTimes,
		InitialPoints:      p.InitialPoints,
		DeviceLimit:        p.DeviceLimit,
		ConcurrentLimit:    p.ConcurrentLimit,
		HeartbeatSec:       p.HeartbeatSec,
		OfflineHours:       p.OfflineHours,
		OfflineStarts:      p.OfflineStarts,
		RebindFree:         p.RebindFree,
		RebindCooldownDays: p.RebindCooldownDays,
		RebindCostHours:    p.RebindCostHours,
		Renewable:          p.Renewable == 1,
		TrialPerDevice:     p.TrialPerDevice,
		PriceCents:         p.PriceCents,
		Entitlements:       entitlementCodes,
	}
}

// TotalSeconds 快照里的总时长（duration / trial 用）。
func (s Snapshot) TotalSeconds() int64 {
	return s.DurationDays*86400 + s.DurationHours*3600 + s.DurationSeconds
}

// initialBalance 按 kind 给出 0 态的余额（design.md 第 7 节表格）。
func initialBalance(p store.Policy) (expireAt, remainTimes, points, remainingSeconds int64) {
	switch p.Kind {
	case store.KindCount:
		return 0, p.InitialTimes, 0, 0
	case store.KindCredit:
		return 0, 0, p.InitialPoints, 0
	case store.KindTimed:
		total := p.DurationDays*86400 + p.DurationHours*3600 + p.DurationSeconds
		return 0, 0, 0, total
	default: // duration / forever / trial：激活时才开始计时
		return 0, 0, 0, 0
	}
}

type Service struct {
	st  *store.Store
	now func() time.Time
}

func New(st *store.Store) *Service {
	return &Service{st: st, now: time.Now}
}

type GenerateReq struct {
	AppID     int64
	PolicyID  int64
	Qty       int
	Prefix    string
	AgentID   int64 // 0 = 平台直发
	CreatedBy int64
	Actor     string // admin:1
	IP        string
	Note      string
	// Hook 在同一事务提交前执行，用于代理记账等需要与发卡原子化的写入。
	Hook func(ctx context.Context, tx *sql.Tx, batchID int64) error
}

type GenerateResult struct {
	BatchID int64
	Kamis   []string
}

func (s *Service) Generate(ctx context.Context, req GenerateReq) (GenerateResult, error) {
	if req.Qty < 1 || req.Qty > MaxBatchQty {
		return GenerateResult{}, fmt.Errorf("%w: 单批数量需在 1..%d", ErrInvalid, MaxBatchQty)
	}
	p, err := s.st.PolicyByID(ctx, req.PolicyID)
	if err != nil {
		return GenerateResult{}, err
	}
	if p.AppID != req.AppID || p.Active != 1 {
		return GenerateResult{}, fmt.Errorf("%w: 卡类型不可用", ErrInvalid)
	}
	entCodes, err := s.st.ListPolicyEntitlementCodes(ctx, p.ID)
	if err != nil {
		return GenerateResult{}, err
	}
	snapJSON, err := json.Marshal(BuildSnapshot(p, entCodes))
	if err != nil {
		return GenerateResult{}, err
	}

	now := s.now().Unix()
	expireAt, remainTimes, points, remainSec := initialBalance(p)

	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return GenerateResult{}, err
	}
	defer tx.Rollback()

	var agentID sql.NullInt64
	if req.AgentID > 0 {
		agentID = sql.NullInt64{Int64: req.AgentID, Valid: true}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO batches(app_id, policy_id, agent_id, qty, created_by, created_at)
		 VALUES(?,?,?,?,?,?)`, req.AppID, p.ID, agentID, req.Qty, req.CreatedBy, now)
	if err != nil {
		return GenerateResult{}, err
	}
	batchID, err := res.LastInsertId()
	if err != nil {
		return GenerateResult{}, err
	}

	kamis := make([]string, 0, req.Qty)
	for i := 0; i < req.Qty; i++ {
		inserted := false
		for attempt := 0; attempt < 5; attempt++ {
			kami, err := NewKami(req.Prefix)
			if err != nil {
				return GenerateResult{}, err
			}
			_, err = s.st.InsertCard(ctx, tx, store.Card{
				AppID:            req.AppID,
				Kami:             kami,
				PolicyID:         p.ID,
				Kind:             p.Kind,
				Status:           store.CardUnused,
				BatchID:          sql.NullInt64{Int64: batchID, Valid: true},
				AgentID:          agentID,
				DeviceLimit:      p.DeviceLimit,
				ExpireAt:         expireAt,
				RemainTimes:      remainTimes,
				Points:           points,
				RemainingSeconds: remainSec,
				SnapshotJSON:     string(snapJSON),
				Note:             req.Note,
				CreatedAt:        now,
			})
			if err == nil {
				kamis = append(kamis, kami)
				inserted = true
				break
			}
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return GenerateResult{}, err
			}
		}
		if !inserted {
			return GenerateResult{}, fmt.Errorf("卡密连续重复，请换一个前缀")
		}
	}

	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     now,
		Actor:  req.Actor,
		Action: "card.generate",
		Target: fmt.Sprintf("batch:%d", batchID),
		After:  fmt.Sprintf(`{"policy":%q,"qty":%d}`, p.Code, req.Qty),
		IP:     req.IP,
	}); err != nil {
		return GenerateResult{}, err
	}
	if req.Hook != nil {
		if err := req.Hook(ctx, tx, batchID); err != nil {
			return GenerateResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{BatchID: batchID, Kamis: kamis}, nil
}

// deriveLiveStatus 在解除封禁后推断应有的状态。
func deriveLiveStatus(c store.Card, now int64) string {
	switch {
	case c.ExpireAt > 0 && c.ExpireAt <= now:
		return store.CardExpired
	case c.Kind == store.KindCount && c.RemainTimes <= 0:
		return store.CardExhausted
	case c.Kind == store.KindCredit && c.Points <= 0:
		return store.CardExhausted
	case c.Kind == store.KindTimed && c.RemainingSeconds <= 0:
		return store.CardExhausted
	case c.ActivatedAt.Valid:
		return store.CardActive
	default:
		return store.CardUnused
	}
}

type StatusChange struct {
	Action string // 审计 action
	Actor  string
	IP     string
	Reason string
}

func (s *Service) Ban(ctx context.Context, id int64, ch StatusChange) error {
	return s.mutate(ctx, id, ch, func(c *store.Card, now int64) error {
		if c.Status == store.CardBanned {
			return fmt.Errorf("%w: 已封禁", ErrState)
		}
		if c.Status == store.CardRecycled {
			return fmt.Errorf("%w: 已回收", ErrState)
		}
		c.Status = store.CardBanned
		c.BannedAt = sql.NullInt64{Int64: now, Valid: true}
		return nil
	})
}

func (s *Service) Unban(ctx context.Context, id int64, ch StatusChange) error {
	return s.mutate(ctx, id, ch, func(c *store.Card, now int64) error {
		if c.Status != store.CardBanned {
			return fmt.Errorf("%w: 未封禁", ErrState)
		}
		c.BannedAt = sql.NullInt64{}
		c.Status = deriveLiveStatus(*c, now)
		return nil
	})
}

func (s *Service) Recycle(ctx context.Context, id int64, ch StatusChange) error {
	return s.mutate(ctx, id, ch, func(c *store.Card, now int64) error {
		if c.Status == store.CardRecycled {
			return fmt.Errorf("%w: 已回收", ErrState)
		}
		c.Status = store.CardRecycled
		return nil
	})
}

type ExtendAdd struct {
	Days    int64
	Times   int64
	Points  int64
	Seconds int64
}

func (s *Service) Extend(ctx context.Context, id int64, add ExtendAdd, ch StatusChange) error {
	if add.Days < 0 || add.Times < 0 || add.Points < 0 || add.Seconds < 0 {
		return fmt.Errorf("%w: 增量不能为负", ErrInvalid)
	}
	return s.mutate(ctx, id, ch, func(c *store.Card, now int64) error {
		if c.Status == store.CardRecycled {
			return fmt.Errorf("%w: 已回收", ErrState)
		}
		if add.Days > 0 {
			if c.ExpireAt == 0 {
				return fmt.Errorf("%w: 永久卡无需续期", ErrState)
			}
			c.ExpireAt += add.Days * 86400
		}
		c.RemainTimes += add.Times
		c.Points += add.Points
		c.RemainingSeconds += add.Seconds
		if c.Status == store.CardExpired || c.Status == store.CardExhausted {
			c.Status = deriveLiveStatus(*c, now)
		}
		return nil
	})
}

func (s *Service) Unbind(ctx context.Context, id int64, ch StatusChange) error {
	now := s.now().Unix()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	c, err := s.st.CardTx(ctx, tx, id)
	if err != nil {
		return err
	}
	before := cardAudit(c)
	if _, err := s.st.UnbindDevices(ctx, tx, id, now); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     now,
		Actor:  ch.Actor,
		Action: ch.Action,
		Target: fmt.Sprintf("card:%d", id),
		Before: before,
		After:  before,
		IP:     ch.IP,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ResetRebind(ctx context.Context, id int64, ch StatusChange) error {
	return s.mutate(ctx, id, ch, func(c *store.Card, now int64) error {
		c.RebindUsed = 0
		return nil
	})
}

// mutate 读卡、改字段、写回，并在同一事务里落审计。
func (s *Service) mutate(ctx context.Context, id int64, ch StatusChange, fn func(c *store.Card, now int64) error) error {
	now := s.now().Unix()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 事务内读，避免与并发操作打架
	c, err := s.st.CardTx(ctx, tx, id)
	if err != nil {
		return err
	}
	before := cardAudit(c)
	if err := fn(&c, now); err != nil {
		return err
	}
	if err := s.st.UpdateCardTx(ctx, tx, c); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     now,
		Actor:  ch.Actor,
		Action: ch.Action,
		Target: fmt.Sprintf("card:%d", id),
		Before: before,
		After:  cardAudit(c),
		IP:     ch.IP,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) writeAudit(ctx context.Context, ch StatusChange, id int64, before, after string) error {
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     s.now().Unix(),
		Actor:  ch.Actor,
		Action: ch.Action,
		Target: fmt.Sprintf("card:%d", id),
		Before: before,
		After:  after,
		IP:     ch.IP,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// cardAudit 审计里只用 id，不带卡密原文（AGENTS.md 硬约束）。
func cardAudit(c store.Card) string {
	return fmt.Sprintf(
		`{"id":%d,"status":%q,"device_limit":%d,"expire_at":%d,"remain_times":%d,"points":%d,"remaining_seconds":%d,"rebind_used":%d}`,
		c.ID, c.Status, c.DeviceLimit, c.ExpireAt, c.RemainTimes, c.Points, c.RemainingSeconds, c.RebindUsed)
}

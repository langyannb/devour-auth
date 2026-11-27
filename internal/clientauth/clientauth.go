// Package clientauth 实现终端客户端的登录、会话与刷新（design.md 第 12 节）。
package clientauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

const (
	// AccessTTL / RefreshTTL 见 design.md 12：15 分钟 / 30 天。
	AccessTTL      = 15 * time.Minute
	RefreshTTL     = 30 * 24 * time.Hour
	maxClockSkew   = 300
	nonceTTL       = 5 * time.Minute
	reconnectGrace = 5 * time.Minute
)

type Service struct {
	st     *store.Store
	now    func() time.Time
	nonces *nonceSet
}

func New(st *store.Store) *Service {
	return &Service{st: st, now: time.Now, nonces: newNonceSet()}
}

// ---------- nonce 去重 ----------

type nonceSet struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newNonceSet() *nonceSet {
	return &nonceSet{seen: make(map[string]time.Time)}
}

// Use 首次出现返回 true；窗口内重复返回 false。
func (n *nonceSet) Use(nonce string, now time.Time, ttl time.Duration) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if t, ok := n.seen[nonce]; ok && t.After(now.Add(-ttl)) {
		return false
	}
	if len(n.seen) > 8192 {
		for k, t := range n.seen {
			if !t.After(now.Add(-ttl)) {
				delete(n.seen, k)
			}
		}
	}
	n.seen[nonce] = now
	return true
}

// ---------- 请求与视图 ----------

type LoginReq struct {
	Kami        string
	Fingerprint string
	Model       string
	Brand       string
	OS          string
	AppVersion  string
	TS          int64
	Nonce       string
}

type CardView struct {
	Kind             string `json:"kind"`
	ExpireAt         int64  `json:"expire_at"`
	Points           int64  `json:"points"`
	RemainTimes      int64  `json:"remain_times"`
	RemainingSeconds int64  `json:"remaining_seconds"`
	Paused           bool   `json:"paused"`
	DeviceLimit      int64  `json:"device_limit"`
	BoundDevices     int64  `json:"bound_devices"`
}

type EntitlementView struct {
	Code       string `json:"code"`
	CostPoints int64  `json:"cost_points"`
	CostTimes  int64  `json:"cost_times"`
}

type Tokens struct {
	AccessToken  string `json:"access_token,omitempty"`
	AccessExp    int64  `json:"access_exp,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	RefreshExp   int64  `json:"refresh_exp,omitempty"`
}

type SessionView struct {
	Tokens
	Card         CardView          `json:"card"`
	Entitlements []EntitlementView `json:"entitlements"`
	ServerTime   int64             `json:"server_time"`
}

// ---------- 登录 ----------

func (s *Service) Login(ctx context.Context, req LoginReq, ip, ua string) (SessionView, error) {
	now := s.now()
	if req.Fingerprint == "" || req.Kami == "" {
		return SessionView{}, apierr.New(apierr.BadInput, "缺少卡密或设备指纹")
	}
	if d := now.Unix() - req.TS; d > maxClockSkew || d < -maxClockSkew {
		return SessionView{}, apierr.New(apierr.BadInput, "客户端时间偏差超过 5 分钟")
	}
	if req.Nonce == "" || len(req.Nonce) < 16 {
		return SessionView{}, apierr.New(apierr.BadInput, "nonce 缺失或过短")
	}
	if !s.nonces.Use(req.Nonce, now, nonceTTL) {
		return SessionView{}, apierr.New(apierr.BadInput, "nonce 重复")
	}

	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return SessionView{}, err
	}
	defer tx.Rollback()

	row, err := s.st.CardByKamiTx(ctx, tx, req.Kami)
	if errors.Is(err, store.ErrNotFound) {
		return SessionView{}, apierr.New(apierr.CardNotFound, "卡密不存在")
	} else if err != nil {
		return SessionView{}, err
	}
	if err := checkUsable(row, now.Unix()); err != nil {
		return SessionView{}, err
	}

	// 黑名单：IP 与设备指纹命中直接拒绝
	if blocked, err := s.st.BlacklistedTx(ctx, tx, row.AppID, "ip", ip, now.Unix()); err != nil {
		return SessionView{}, err
	} else if blocked {
		return SessionView{}, apierr.New(apierr.CardBanned, "该网络已被限制")
	}
	if blocked, err := s.st.BlacklistedTx(ctx, tx, row.AppID, "device", req.Fingerprint, now.Unix()); err != nil {
		return SessionView{}, err
	} else if blocked {
		return SessionView{}, apierr.New(apierr.DeviceMismatch, "该设备已被限制")
	}

	snap := SnapshotOf(row)
	if snap.DeviceLimit <= 0 {
		snap.DeviceLimit = row.DeviceLimit
	}
	if snap.ConcurrentLimit <= 0 {
		snap.ConcurrentLimit = 1
	}

	// 管理员开关：设备验证关掉就跳过设备数量上限；IP 验证只记录异常，不拦登录。
	flags, err := s.st.ListFlagsTx(ctx, tx, row.AppID)
	if err != nil {
		return SessionView{}, err
	}
	verifyDevice := flags["verify_device"] != "0"
	verifyIP := flags["verify_ip"] == "1"

	// 单卡开关优先：卡上关掉设备校验就不限设备数量
	enforceDevice := verifyDevice && row.VerifyDevice != 0
	devID, err := s.bindDevice(ctx, tx, &row, snap, req, now.Unix(), enforceDevice)
	if err != nil {
		return SessionView{}, err
	}
	if verifyIP && ip != "" {
		if last, _ := s.st.LastSessionIPByCardTx(ctx, tx, row.ID); last != "" && !sameSlash24(last, ip) {
			_ = s.st.InsertEventTx(ctx, tx, store.Event{
				TS: now.Unix(), Kind: "ip_anomaly", CardID: row.ID, DeviceID: devID,
				IP: ip, Code: "IP_CHANGED", Detail: "上次登录 " + last,
			})
		}
	}

	// 时长卡 / 试用卡：首次登录开始计时
	if !row.ActivatedAt.Valid && (row.Kind == store.KindDuration || row.Kind == store.KindTrial) {
		if dur := snap.TotalSeconds(); dur > 0 {
			row.ActivatedAt = sql.NullInt64{Int64: now.Unix(), Valid: true}
			row.ExpireAt = now.Unix() + dur
		}
	}

	// 次数卡：新会话扣 1 次，同设备 5 分钟内重连不重复扣
	if row.Kind == store.KindCount {
		recent, err := s.st.RecentSessionForDeviceTx(ctx, tx, row.ID, devID, now.Add(-reconnectGrace).Unix())
		if err != nil {
			return SessionView{}, err
		}
		if !recent {
			if row.RemainTimes < 1 {
				return SessionView{}, apierr.New(apierr.CardExhausted, "次数已用尽")
			}
			row.RemainTimes--
			if !row.ActivatedAt.Valid {
				row.ActivatedAt = sql.NullInt64{Int64: now.Unix(), Valid: true}
			}
		}
	}

	// 试用卡：一台设备一个应用只能领一次
	if row.Kind == store.KindTrial {
		if err := s.st.ClaimTrialTx(ctx, tx, row.AppID, req.Fingerprint, row.ID, now.Unix()); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return SessionView{}, apierr.New(apierr.DeviceLimit, "该设备已领取过试用")
			}
			return SessionView{}, err
		}
	}

	// 并发上限：挤掉最旧会话
	active, err := s.st.ActiveClientSessionsTx(ctx, tx, row.ID, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	if int64(len(active)) >= snap.ConcurrentLimit {
		drop := int64(len(active)) - snap.ConcurrentLimit + 1
		for i := int64(0); i < drop && i < int64(len(active)); i++ {
			if err := s.st.RevokeClientSessionTx(ctx, tx, active[i].ID, now.Unix()); err != nil {
				return SessionView{}, err
			}
		}
	}

	access, err := auth.NewToken()
	if err != nil {
		return SessionView{}, err
	}
	refresh, err := auth.NewToken()
	if err != nil {
		return SessionView{}, err
	}
	view := SessionView{
		Tokens: Tokens{
			AccessToken:  access,
			AccessExp:    now.Add(AccessTTL).Unix(),
			RefreshToken: refresh,
			RefreshExp:   now.Add(RefreshTTL).Unix(),
		},
		ServerTime: now.Unix(),
	}
	if _, err := s.st.CreateClientSessionTx(ctx, tx, store.ClientSession{
		CardID:        row.ID,
		DeviceID:      devID,
		AccessHash:    auth.HashToken(access),
		RefreshHash:   auth.HashToken(refresh),
		AccessExp:     view.AccessExp,
		RefreshExp:    view.RefreshExp,
		LastHeartbeat: now.Unix(),
		IP:            ip,
		UA:            ua,
		CreatedAt:     now.Unix(),
	}); err != nil {
		return SessionView{}, err
	}

	row.Status = liveStatus(row, now.Unix())
	if err := s.st.UpdateCardTx(ctx, tx, row); err != nil {
		return SessionView{}, err
	}

	ents, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	bound, err := s.st.CountBoundDevicesTx(ctx, tx, row.ID)
	if err != nil {
		return SessionView{}, err
	}
	view.Card = viewOf(row, snap.DeviceLimit, bound)
	view.Entitlements = entViews(ents)

	if err := s.st.InsertEventTx(ctx, tx, store.Event{
		TS: now.Unix(), Kind: "login", CardID: row.ID, DeviceID: devID,
		IP: ip, UA: ua, Code: "OK",
	}); err != nil {
		return SessionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionView{}, err
	}
	return view, nil
}

func (s *Service) bindDevice(ctx context.Context, tx *sql.Tx, row *store.Card, snap card.Snapshot, req LoginReq, now int64, enforceLimit bool) (int64, error) {
	dev, err := s.st.BoundDeviceTx(ctx, tx, row.ID, req.Fingerprint)
	if err == nil {
		if dev.Banned == 1 {
			return 0, apierr.New(apierr.DeviceMismatch, "该设备已被封禁")
		}
		if err := s.st.TouchDeviceTx(ctx, tx, dev.ID, now); err != nil {
			return 0, err
		}
		return dev.ID, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return 0, err
	}
	n, err := s.st.CountBoundDevicesTx(ctx, tx, row.ID)
	if err != nil {
		return 0, err
	}
	if enforceLimit && n >= snap.DeviceLimit {
		return 0, apierr.New(apierr.DeviceLimit, "设备数量已达上限（%d）", snap.DeviceLimit)
	}
	return s.st.InsertDeviceTx(ctx, tx, store.Device{
		CardID: row.ID, Fingerprint: req.Fingerprint,
		Model: req.Model, Brand: req.Brand, OS: req.OS, AppVersion: req.AppVersion,
		FirstSeen: now, LastSeen: now,
	})
}

// ---------- 会话状态 / 心跳 ----------

func (s *Service) Status(ctx context.Context, accessToken, ip, ua string) (SessionView, error) {
	now := s.now()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return SessionView{}, err
	}
	defer tx.Rollback()

	sess, err := s.st.ClientSessionByAccessHashTx(ctx, tx, auth.HashToken(accessToken))
	if errors.Is(err, store.ErrNotFound) {
		return SessionView{}, apierr.New(apierr.Unauthorized, "令牌无效")
	} else if err != nil {
		return SessionView{}, err
	}
	if sess.RevokedAt.Valid {
		return SessionView{}, apierr.New(apierr.Revoked, "会话已吊销")
	}
	if sess.AccessExp <= now.Unix() || sess.RefreshExp <= now.Unix() {
		return SessionView{}, apierr.New(apierr.Unauthorized, "令牌已过期")
	}

	row, err := s.st.CardTx(ctx, tx, sess.CardID)
	if err != nil {
		return SessionView{}, err
	}
	if err := checkUsable(row, now.Unix()); err != nil {
		return SessionView{}, err
	}
	snap := SnapshotOf(row)

	// 计时卡：服务端按心跳间隔扣秒，不采信客户端上报
	if row.Kind == store.KindTimed && row.Paused == 0 {
		maxStep := snap.HeartbeatSec * 2
		if maxStep <= 0 {
			maxStep = 1200
		}
		delta := now.Unix() - sess.LastHeartbeat
		if delta > maxStep {
			delta = maxStep
		}
		if delta > 0 {
			row.RemainingSeconds -= delta
			if row.RemainingSeconds < 0 {
				row.RemainingSeconds = 0
			}
			if row.RemainingSeconds == 0 {
				row.Status = store.CardExhausted
			}
			if err := s.st.UpdateCardTx(ctx, tx, row); err != nil {
				return SessionView{}, err
			}
		}
	}

	ents, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	bound, err := s.st.CountBoundDevicesTx(ctx, tx, row.ID)
	if err != nil {
		return SessionView{}, err
	}

	if err := s.st.TouchClientSessionTx(ctx, tx, sess.ID, now.Unix()); err != nil {
		return SessionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionView{}, err
	}

	view := SessionView{
		Card:         viewOf(row, snap.DeviceLimit, bound),
		Entitlements: entViews(ents),
		ServerTime:   now.Unix(),
	}
	if row.Kind == store.KindTimed && row.Paused == 1 {
		return view, apierr.New(apierr.Paused, "卡已暂停")
	}
	if row.RemainingSeconds == 0 && row.Kind == store.KindTimed {
		return view, apierr.New(apierr.CardExhausted, "时长已用尽")
	}
	return view, nil
}

// ---------- 刷新 ----------

func (s *Service) Refresh(ctx context.Context, refreshToken, ip, ua string) (SessionView, error) {
	now := s.now()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return SessionView{}, err
	}
	defer tx.Rollback()

	oldHash := auth.HashToken(refreshToken)

	// 旧 refresh 被再次使用 = 盗用，整卡吊销
	if cardID, reused, err := s.st.RefreshReuseCardTx(ctx, tx, oldHash); err != nil {
		return SessionView{}, err
	} else if reused {
		if _, err := s.st.RevokeAllClientSessionsTx(ctx, tx, cardID, now.Unix()); err != nil {
			return SessionView{}, err
		}
		if err := s.st.InsertEventTx(ctx, tx, store.Event{
			TS: now.Unix(), Kind: "refresh_reuse", CardID: cardID, IP: ip, UA: ua, Code: apierr.Revoked,
		}); err != nil {
			return SessionView{}, err
		}
		if err := tx.Commit(); err != nil {
			return SessionView{}, err
		}
		return SessionView{}, apierr.New(apierr.Revoked, "令牌被重复使用，全部会话已吊销")
	}

	sess, err := s.st.ClientSessionByRefreshHashTx(ctx, tx, oldHash)
	if errors.Is(err, store.ErrNotFound) {
		return SessionView{}, apierr.New(apierr.Unauthorized, "refresh 无效")
	} else if err != nil {
		return SessionView{}, err
	}
	if sess.RevokedAt.Valid {
		return SessionView{}, apierr.New(apierr.Revoked, "会话已吊销")
	}
	if sess.RefreshExp <= now.Unix() {
		return SessionView{}, apierr.New(apierr.Unauthorized, "refresh 已过期")
	}

	row, err := s.st.CardTx(ctx, tx, sess.CardID)
	if err != nil {
		return SessionView{}, err
	}
	if err := checkUsable(row, now.Unix()); err != nil {
		return SessionView{}, err
	}

	access, err := auth.NewToken()
	if err != nil {
		return SessionView{}, err
	}
	refresh, err := auth.NewToken()
	if err != nil {
		return SessionView{}, err
	}
	view := SessionView{
		Tokens: Tokens{
			AccessToken:  access,
			AccessExp:    now.Add(AccessTTL).Unix(),
			RefreshToken: refresh,
			RefreshExp:   now.Add(RefreshTTL).Unix(),
		},
		ServerTime: now.Unix(),
	}

	if err := s.st.InsertRefreshHistoryTx(ctx, tx, oldHash, sess.ID, sess.CardID, now.Unix()); err != nil {
		return SessionView{}, err
	}
	if err := s.st.RotateTokens(ctx, tx, sess.ID,
		auth.HashToken(access), view.AccessExp, auth.HashToken(refresh), view.RefreshExp); err != nil {
		return SessionView{}, err
	}

	snap := SnapshotOf(row)
	ents, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	bound, err := s.st.CountBoundDevicesTx(ctx, tx, row.ID)
	if err != nil {
		return SessionView{}, err
	}
	view.Card = viewOf(row, snap.DeviceLimit, bound)
	view.Entitlements = entViews(ents)

	if err := s.st.InsertEventTx(ctx, tx, store.Event{
		TS: now.Unix(), Kind: "refresh", CardID: row.ID, DeviceID: sess.DeviceID, IP: ip, UA: ua, Code: "OK",
	}); err != nil {
		return SessionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionView{}, err
	}
	return view, nil
}

// ---------- 登出 ----------

func (s *Service) Logout(ctx context.Context, accessToken string) error {
	now := s.now()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	sess, err := s.st.ClientSessionByAccessHashTx(ctx, tx, auth.HashToken(accessToken))
	if errors.Is(err, store.ErrNotFound) {
		return apierr.New(apierr.Unauthorized, "令牌无效")
	} else if err != nil {
		return err
	}
	if err := s.st.RevokeClientSessionTx(ctx, tx, sess.ID, now.Unix()); err != nil {
		return err
	}
	if err := s.st.InsertEventTx(ctx, tx, store.Event{
		TS: now.Unix(), Kind: "logout", CardID: sess.CardID, DeviceID: sess.DeviceID, Code: "OK",
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- 消耗 ----------

type ConsumeResult struct {
	Code            string `json:"code"`
	BalanceAfter    int64  `json:"balance_after"`
	CostPoints      int64  `json:"cost_points"`
	CostTimes       int64  `json:"cost_times"`
	RemainingPoints int64  `json:"remaining_points"`
	RemainingTimes  int64  `json:"remaining_times"`
	Replayed        bool   `json:"replayed"`
	ServerTime      int64  `json:"server_time"`
}

// Consume 按权限定义扣点/扣次，nonce 全局幂等（design.md 第 8、12 节）。
func (s *Service) Consume(ctx context.Context, accessToken, code, nonce, ip, ua string) (ConsumeResult, error) {
	now := s.now()
	if code == "" || nonce == "" || len(nonce) < 16 {
		return ConsumeResult{}, apierr.New(apierr.BadInput, "entitlement_code / nonce 缺失")
	}

	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return ConsumeResult{}, err
	}
	defer tx.Rollback()

	sess, row, err := s.sessionCardTx(ctx, tx, accessToken, now.Unix())
	if err != nil {
		return ConsumeResult{}, err
	}
	_ = sess

	// 幂等：同一 nonce 重放返回上次结果
	if u, err := s.st.UsageByNonceTx(ctx, tx, nonce); err == nil {
		if u.CardID != row.ID {
			return ConsumeResult{}, apierr.New(apierr.BadInput, "nonce 已被其他卡使用")
		}
		return ConsumeResult{
			Code: u.Code, BalanceAfter: u.BalanceAfter,
			CostPoints: u.CostPoints, CostTimes: u.CostTimes,
			RemainingPoints: row.Points, RemainingTimes: row.RemainTimes,
			Replayed: true, ServerTime: now.Unix(),
		}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return ConsumeResult{}, err
	}

	snap := SnapshotOf(row)
	eff, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return ConsumeResult{}, err
	}
	var ent store.Entitlement
	found := false
	for _, e := range eff {
		if e.Code == code {
			ent, found = e, true
			break
		}
	}
	if !found {
		return ConsumeResult{}, apierr.New(apierr.EntitlementMissing, "没有该权限")
	}

	balance := row.Points
	if ent.CostTimes > 0 {
		balance = row.RemainTimes
	}
	if ent.CostPoints > 0 {
		bal, ok, err := s.st.DeductPointsTx(ctx, tx, row.ID, ent.CostPoints)
		if err != nil {
			return ConsumeResult{}, err
		}
		if !ok {
			return ConsumeResult{}, apierr.New(apierr.CardExhausted, "点数不足")
		}
		row.Points, balance = bal, bal
	}
	if ent.CostTimes > 0 {
		bal, ok, err := s.st.DeductTimesTx(ctx, tx, row.ID, ent.CostTimes)
		if err != nil {
			return ConsumeResult{}, err
		}
		if !ok {
			return ConsumeResult{}, apierr.New(apierr.CardExhausted, "次数不足")
		}
		row.RemainTimes, balance = bal, bal
	}

	if (row.Kind == store.KindCredit && row.Points == 0) ||
		(row.Kind == store.KindCount && row.RemainTimes == 0) {
		row.Status = store.CardExhausted
		if err := s.st.UpdateCardTx(ctx, tx, row); err != nil {
			return ConsumeResult{}, err
		}
	}

	if err := s.st.InsertUsageTx(ctx, tx, store.Usage{
		CardID: row.ID, Code: code,
		CostPoints: ent.CostPoints, CostTimes: ent.CostTimes,
		BalanceAfter: balance, Nonce: nonce, CreatedAt: now.Unix(),
	}); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return ConsumeResult{}, apierr.New(apierr.BadInput, "nonce 冲突")
		}
		return ConsumeResult{}, err
	}
	if err := s.st.InsertEventTx(ctx, tx, store.Event{
		TS: now.Unix(), Kind: "consume", CardID: row.ID, IP: ip, UA: ua, Code: code,
	}); err != nil {
		return ConsumeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConsumeResult{}, err
	}
	return ConsumeResult{
		Code: code, BalanceAfter: balance,
		CostPoints: ent.CostPoints, CostTimes: ent.CostTimes,
		RemainingPoints: row.Points, RemainingTimes: row.RemainTimes,
		ServerTime: now.Unix(),
	}, nil
}

// ---------- 暂停 ----------

func (s *Service) Pause(ctx context.Context, accessToken string, paused bool, ip, ua string) (SessionView, error) {
	now := s.now()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return SessionView{}, err
	}
	defer tx.Rollback()

	_, row, err := s.sessionCardTx(ctx, tx, accessToken, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	if row.Kind != store.KindTimed {
		return SessionView{}, apierr.New(apierr.BadInput, "仅计时卡支持暂停")
	}
	if err := s.st.SetCardPausedTx(ctx, tx, row.ID, paused); err != nil {
		return SessionView{}, err
	}
	if paused {
		row.Paused = 1
	} else {
		row.Paused = 0
	}

	snap := SnapshotOf(row)
	ents, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return SessionView{}, err
	}
	bound, err := s.st.CountBoundDevicesTx(ctx, tx, row.ID)
	if err != nil {
		return SessionView{}, err
	}
	if err := s.st.InsertEventTx(ctx, tx, store.Event{
		TS: now.Unix(), Kind: "pause", CardID: row.ID, IP: ip, UA: ua,
		Detail: fmt.Sprintf("%t", paused),
	}); err != nil {
		return SessionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionView{}, err
	}
	return SessionView{
		Card:         viewOf(row, snap.DeviceLimit, bound),
		Entitlements: entViews(ents),
		ServerTime:   now.Unix(),
	}, nil
}

// sessionCardTx 从 access token 取会话与卡，并做吊销/过期/可用性检查。
func (s *Service) sessionCardTx(ctx context.Context, tx *sql.Tx, accessToken string, now int64) (store.ClientSession, store.Card, error) {
	sess, err := s.st.ClientSessionByAccessHashTx(ctx, tx, auth.HashToken(accessToken))
	if errors.Is(err, store.ErrNotFound) {
		return store.ClientSession{}, store.Card{}, apierr.New(apierr.Unauthorized, "令牌无效")
	} else if err != nil {
		return store.ClientSession{}, store.Card{}, err
	}
	if sess.RevokedAt.Valid {
		return store.ClientSession{}, store.Card{}, apierr.New(apierr.Revoked, "会话已吊销")
	}
	if sess.AccessExp <= now || sess.RefreshExp <= now {
		return store.ClientSession{}, store.Card{}, apierr.New(apierr.Unauthorized, "令牌已过期")
	}
	row, err := s.st.CardTx(ctx, tx, sess.CardID)
	if err != nil {
		return store.ClientSession{}, store.Card{}, err
	}
	if err := checkUsable(row, now); err != nil {
		return store.ClientSession{}, store.Card{}, err
	}
	return sess, row, nil
}

// effectiveEntitlements 生效权限 = 卡类型权限 ∪ 单卡附加 − 单卡撤销，撤销优先。
func (s *Service) effectiveEntitlements(ctx context.Context, tx *sql.Tx, row store.Card, snap card.Snapshot, now int64) ([]store.Entitlement, error) {
	eff := make(map[string]bool, len(snap.Entitlements))
	for _, c := range snap.Entitlements {
		eff[c] = true
	}
	overrides, err := s.st.CardEntitlementRowsTx(ctx, tx, row.ID)
	if err != nil {
		return nil, err
	}
	for _, o := range overrides {
		if o.Revoked {
			delete(eff, o.Code)
			continue
		}
		if o.ExpiresAt == 0 || o.ExpiresAt > now {
			eff[o.Code] = true
		}
	}
	codes := make([]string, 0, len(eff))
	for c := range eff {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return s.st.EntitlementsByCodesTx(ctx, tx, row.AppID, codes)
}

// TicketData 取离线票据所需的数据：卡、设备指纹、快照与生效权限。
func (s *Service) TicketData(ctx context.Context, accessToken string) (store.Card, card.Snapshot, string, []store.Entitlement, error) {
	now := s.now()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return store.Card{}, card.Snapshot{}, "", nil, err
	}
	defer tx.Rollback()

	sess, row, err := s.sessionCardTx(ctx, tx, accessToken, now.Unix())
	if err != nil {
		return store.Card{}, card.Snapshot{}, "", nil, err
	}
	dev, err := s.st.DeviceByIDTx(ctx, tx, sess.DeviceID)
	if err != nil {
		return store.Card{}, card.Snapshot{}, "", nil, err
	}
	snap := SnapshotOf(row)
	ents, err := s.effectiveEntitlements(ctx, tx, row, snap, now.Unix())
	if err != nil {
		return store.Card{}, card.Snapshot{}, "", nil, err
	}
	return row, snap, dev.Fingerprint, ents, nil
}

// ---------- 工具 ----------

// SnapshotOf 解析出卡时冻结的快照；解析失败一律按空快照处理，不阻断登录。
func SnapshotOf(c store.Card) card.Snapshot {
	var snap card.Snapshot
	_ = json.Unmarshal([]byte(c.SnapshotJSON), &snap)
	if snap.DeviceLimit <= 0 {
		snap.DeviceLimit = c.DeviceLimit
	}
	if snap.ConcurrentLimit <= 0 {
		snap.ConcurrentLimit = 1
	}
	return snap
}

func checkUsable(c store.Card, now int64) error {
	switch c.Status {
	case store.CardBanned, store.CardRecycled:
		return apierr.New(apierr.CardBanned, "卡密已被封禁")
	case store.CardExpired:
		return apierr.New(apierr.CardExpired, "卡密已到期")
	case store.CardExhausted:
		return apierr.New(apierr.CardExhausted, "卡密余额已用尽")
	}
	if c.ExpireAt > 0 && c.ExpireAt <= now {
		return apierr.New(apierr.CardExpired, "卡密已到期")
	}
	switch c.Kind {
	case store.KindCount:
		if c.RemainTimes <= 0 {
			return apierr.New(apierr.CardExhausted, "次数已用尽")
		}
	case store.KindCredit:
		if c.Points <= 0 {
			return apierr.New(apierr.CardExhausted, "点数已用尽")
		}
	case store.KindTimed:
		if c.RemainingSeconds <= 0 {
			return apierr.New(apierr.CardExhausted, "时长已用尽")
		}
	}
	return nil
}

func liveStatus(c store.Card, now int64) string {
	if c.ExpireAt > 0 && c.ExpireAt <= now {
		return store.CardExpired
	}
	if c.ActivatedAt.Valid {
		return store.CardActive
	}
	return store.CardUnused
}

func viewOf(c store.Card, deviceLimit, bound int64) CardView {
	return CardView{
		Kind:             c.Kind,
		ExpireAt:         c.ExpireAt,
		Points:           c.Points,
		RemainTimes:      c.RemainTimes,
		RemainingSeconds: c.RemainingSeconds,
		Paused:           c.Paused == 1,
		DeviceLimit:      deviceLimit,
		BoundDevices:     bound,
	}
}

func entViews(es []store.Entitlement) []EntitlementView {
	out := make([]EntitlementView, 0, len(es))
	for _, e := range es {
		out = append(out, EntitlementView{Code: e.Code, CostPoints: e.CostPoints, CostTimes: e.CostTimes})
	}
	return out
}

// sameSlash24 判断两个 IP 是否同网段：IPv4 看 /24，IPv6 看 /48；解析失败按不同处理。
func sameSlash24(a, b string) bool {
	ia, ib := net.ParseIP(a), net.ParseIP(b)
	if ia == nil || ib == nil {
		return a == b
	}
	if v4a, v4b := ia.To4(), ib.To4(); v4a != nil && v4b != nil {
		return v4a[0] == v4b[0] && v4a[1] == v4b[1] && v4a[2] == v4b[2]
	}
	v6a, v6b := ia.To16(), ib.To16()
	if v6a == nil || v6b == nil {
		return false
	}
	for i := 0; i < 6; i++ {
		if v6a[i] != v6b[i] {
			return false
		}
	}
	return true
}

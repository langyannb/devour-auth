package clientauth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

type harness struct {
	st    *store.Store
	svc   *Service
	cards *card.Service
	now   time.Time
	appID int64
	nonce int
}

func setup(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	appID, err := st.CreateApp(ctx, "devour", "Devour", 1000)
	if err != nil {
		t.Fatalf("app: %v", err)
	}
	h := &harness{
		st:    st,
		svc:   New(st),
		cards: card.New(st),
		now:   time.Unix(1_800_000_000, 0),
		appID: appID,
	}
	h.svc.now = func() time.Time { return h.now }
	h.cards = card.New(st)
	return h
}

func (h *harness) makePolicy(t *testing.T, p store.Policy) store.Policy {
	t.Helper()
	ctx := context.Background()
	p.AppID = h.appID
	if p.Active == 0 {
		p.Active = 1
	}
	if p.DeviceLimit == 0 {
		p.DeviceLimit = 2
	}
	if p.ConcurrentLimit == 0 {
		p.ConcurrentLimit = 1
	}
	if p.HeartbeatSec == 0 {
		p.HeartbeatSec = 600
	}
	p.CreatedAt = 1000
	id, err := h.st.CreatePolicy(ctx, p)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	got, err := h.st.PolicyByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (h *harness) issue(t *testing.T, policyID int64) string {
	t.Helper()
	res, err := h.cards.Generate(context.Background(), card.GenerateReq{
		AppID: h.appID, PolicyID: policyID, Qty: 1, Prefix: "DV",
		CreatedBy: 1, Actor: "admin:1",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return res.Kamis[0]
}

func (h *harness) login(kami, fp string) (SessionView, error) {
	h.nonce++
	return h.svc.Login(context.Background(), LoginReq{
		Kami: kami, Fingerprint: fp,
		Model: "m", Brand: "b", OS: "android", AppVersion: "2.0.0",
		TS:    h.now.Unix(),
		Nonce: fmt.Sprintf("nonce-%06d-%s", h.nonce, "0123456789abcdef"),
	}, "1.2.3.4", "test-agent")
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var ae *apierr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("不是 apierr: %v", err)
	}
	return ae.Code
}

func TestLoginBindsDeviceAndRejectsSecondDevice(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever, DeviceLimit: 1})
	kami := h.issue(t, p.ID)

	if _, err := h.login(kami, "fp-1"); err != nil {
		t.Fatalf("第一台设备应能登录: %v", err)
	}
	_, err := h.login(kami, "fp-2")
	if err == nil {
		t.Fatal("第二台设备应被拒绝")
	}
	if got := codeOf(t, err); got != apierr.DeviceLimit {
		t.Fatalf("code = %s want DEVICE_LIMIT", got)
	}
}

// 验收项 3：同一 nonce 重放被拒。
func TestNonceReplayRejected(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	kami := h.issue(t, p.ID)

	req := LoginReq{
		Kami: kami, Fingerprint: "fp-1", TS: h.now.Unix(),
		Nonce: "0123456789abcdef0123456789abcdef",
	}
	if _, err := h.svc.Login(context.Background(), req, "1.1.1.1", "ua"); err != nil {
		t.Fatalf("首次登录: %v", err)
	}
	_, err := h.svc.Login(context.Background(), req, "1.1.1.1", "ua")
	if err == nil {
		t.Fatal("同 nonce 重放应被拒")
	}
	if got := codeOf(t, err); got != apierr.BadInput {
		t.Fatalf("code = %s", got)
	}

	// 时间偏差过大也拒
	req2 := req
	req2.Nonce = "ffffffffffffffffffffffffffffffff"
	req2.TS = h.now.Add(10 * time.Minute).Unix()
	if _, err := h.svc.Login(context.Background(), req2, "1.1.1.1", "ua"); err == nil {
		t.Fatal("时间偏差过大应被拒")
	}
}

// 验收项 8：次数卡同设备 5 分钟内重连不重复扣。
func TestCountCardDeduction(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "count5", Name: "count5", Kind: store.KindCount,
		InitialTimes: 5, DeviceLimit: 3})
	kami := h.issue(t, p.ID)

	if _, err := h.login(kami, "fp-1"); err != nil {
		t.Fatal(err)
	}
	c, _ := h.st.CardByKami(context.Background(), kami)
	if c.RemainTimes != 4 {
		t.Fatalf("首次登录后 remain_times = %d want 4", c.RemainTimes)
	}

	// 3 分钟后同设备重连：不扣
	h.now = h.now.Add(3 * time.Minute)
	if _, err := h.login(kami, "fp-1"); err != nil {
		t.Fatal(err)
	}
	c, _ = h.st.CardByKami(context.Background(), kami)
	if c.RemainTimes != 4 {
		t.Fatalf("5 分钟内重连不应扣次: remain_times = %d", c.RemainTimes)
	}

	// 6 分钟后重连：扣一次
	h.now = h.now.Add(6 * time.Minute)
	if _, err := h.login(kami, "fp-1"); err != nil {
		t.Fatal(err)
	}
	c, _ = h.st.CardByKami(context.Background(), kami)
	if c.RemainTimes != 3 {
		t.Fatalf("超过 5 分钟应扣次: remain_times = %d", c.RemainTimes)
	}

	// 换设备 6 分钟后：也扣
	h.now = h.now.Add(6 * time.Minute)
	if _, err := h.login(kami, "fp-2"); err != nil {
		t.Fatal(err)
	}
	c, _ = h.st.CardByKami(context.Background(), kami)
	if c.RemainTimes != 2 {
		t.Fatalf("新设备应扣次: remain_times = %d", c.RemainTimes)
	}
}

// 验收项 5：试用卡第二台设备领取被拒。
func TestTrialPerDevice(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "trial", Name: "trial", Kind: store.KindTrial,
		DurationDays: 1, DeviceLimit: 5, TrialPerDevice: 1})
	k1 := h.issue(t, p.ID)
	k2 := h.issue(t, p.ID)

	if _, err := h.login(k1, "fp-1"); err != nil {
		t.Fatalf("首次领取: %v", err)
	}
	if _, err := h.login(k2, "fp-1"); err == nil {
		t.Fatal("同设备第二次领取试用应被拒")
	} else if codeOf(t, err) != apierr.DeviceLimit {
		t.Fatalf("code = %s", codeOf(t, err))
	}
	// 换设备可以领
	if _, err := h.login(k2, "fp-2"); err != nil {
		t.Fatalf("另一台设备应能领: %v", err)
	}
}

func TestDurationActivatesOnFirstLogin(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "d30", Name: "d30", Kind: store.KindDuration, DurationDays: 30})
	kami := h.issue(t, p.ID)

	before, _ := h.st.CardByKami(context.Background(), kami)
	if before.ExpireAt != 0 || before.ActivatedAt.Valid {
		t.Fatal("新时长卡不应已有到期时间")
	}

	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Card.ExpireAt != h.now.Unix()+30*86400 {
		t.Fatalf("expire_at = %d want %d", view.Card.ExpireAt, h.now.Unix()+30*86400)
	}
	c, _ := h.st.CardByKami(context.Background(), kami)
	if c.Status != store.CardActive || !c.ActivatedAt.Valid {
		t.Fatalf("状态 = %s activated = %v", c.Status, c.ActivatedAt)
	}
}

// 验收项 6：计时卡暂停后心跳不扣秒且返回 PAUSED。
func TestTimedCardPause(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "timed", Name: "timed", Kind: store.KindTimed,
		DurationHours: 2, HeartbeatSec: 600})
	kami := h.issue(t, p.ID)

	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Card.RemainingSeconds != 7200 {
		t.Fatalf("初始剩余 = %d", view.Card.RemainingSeconds)
	}

	// 过 10 分钟心跳：扣 600 秒
	h.now = h.now.Add(10 * time.Minute)
	if _, err := h.svc.Status(ctx, view.AccessToken, "1.2.3.4", "ua"); err != nil {
		t.Fatal(err)
	}
	c, _ := h.st.CardByKami(ctx, kami)
	if c.RemainingSeconds != 6600 {
		t.Fatalf("心跳后剩余 = %d want 6600", c.RemainingSeconds)
	}

	// 暂停（access 15 分钟有效，这里只推进 4 分钟）
	c.Paused = 1
	if err := h.st.UpdateCard(ctx, c); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(4 * time.Minute)
	_, err = h.svc.Status(ctx, view.AccessToken, "1.2.3.4", "ua")
	if err == nil || codeOf(t, err) != apierr.Paused {
		t.Fatalf("暂停时应返回 PAUSED: %v", err)
	}
	c2, _ := h.st.CardByKami(ctx, kami)
	if c2.RemainingSeconds != 6600 {
		t.Fatalf("暂停期间不应扣秒: %d", c2.RemainingSeconds)
	}
}

// 验收项 13：旧 refresh 复用 → 全量吊销。
func TestRefreshRotationAndReuse(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever, ConcurrentLimit: 2})
	kami := h.issue(t, p.ID)

	first, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.Refresh(ctx, first.RefreshToken, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("刷新: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh 应轮换")
	}
	// 新 access 可用
	if _, err := h.svc.Status(ctx, second.AccessToken, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("新 access 应可用: %v", err)
	}

	// 旧 refresh 再用 → 盗用，吊销整卡
	if _, err := h.svc.Refresh(ctx, first.RefreshToken, "1.2.3.4", "ua"); err == nil {
		t.Fatal("旧 refresh 应被判盗用")
	} else if codeOf(t, err) != apierr.Revoked {
		t.Fatalf("code = %s", codeOf(t, err))
	}

	// 新 refresh 也失效
	_, err = h.svc.Refresh(ctx, second.RefreshToken, "1.2.3.4", "ua")
	if err == nil || codeOf(t, err) != apierr.Revoked {
		t.Fatalf("整卡吊销后新 refresh 也应失败: %v", err)
	}
	// access 同样是 REVOKED
	_, err = h.svc.Status(ctx, second.AccessToken, "1.2.3.4", "ua")
	if err == nil || codeOf(t, err) != apierr.Revoked {
		t.Fatalf("access 应返回 REVOKED: %v", err)
	}
}

func TestConcurrentLimitEvictsOldest(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever,
		DeviceLimit: 1, ConcurrentLimit: 1})
	kami := h.issue(t, p.ID)

	a, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	// 同一设备重连：会话数超过并发上限（= 设备上限），旧会话被挤掉
	if _, err := h.login(kami, "fp-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Status(context.Background(), a.AccessToken, "1.2.3.4", "ua"); err == nil {
		t.Fatal("最旧会话应被吊销")
	} else if codeOf(t, err) != apierr.Revoked {
		t.Fatalf("code = %s", codeOf(t, err))
	}
}

func TestBannedCardRejected(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	kami := h.issue(t, p.ID)
	c, _ := h.st.CardByKami(context.Background(), kami)
	if err := h.cards.Ban(context.Background(), c.ID, card.StatusChange{Actor: "admin:1"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.login(kami, "fp-1")
	if err == nil || codeOf(t, err) != apierr.CardBanned {
		t.Fatalf("封禁卡应拒绝: %v", err)
	}
}

func TestRecycledCardRejected(t *testing.T) {
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	kami := h.issue(t, p.ID)
	c, _ := h.st.CardByKami(context.Background(), kami)
	if err := h.cards.Recycle(context.Background(), c.ID, card.StatusChange{Actor: "admin:1"}); err != nil {
		t.Fatal(err)
	}
	_, err := h.login(kami, "fp-1")
	if err == nil || codeOf(t, err) != apierr.CardRecycled {
		t.Fatalf("回收卡应拒绝: %v", err)
	}
}

// 验收项 2：10 个并发 consume 扣 500 点，最终不为负。
func TestConcurrentConsumeNeverNegative(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "credit", Name: "credit", Kind: store.KindCredit,
		InitialPoints: 500, DeviceLimit: 5, ConcurrentLimit: 5})
	entID, err := h.st.CreateEntitlement(ctx, store.Entitlement{
		AppID: h.appID, Code: "aim", Name: "aim", CostPoints: 100, Active: 1, CreatedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.st.SetPolicyEntitlements(ctx, p.ID, []int64{entID}); err != nil {
		t.Fatal(err)
	}
	kami := h.issue(t, p.ID)

	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Entitlements) != 1 || view.Entitlements[0].Code != "aim" || view.Entitlements[0].CostPoints != 100 {
		t.Fatalf("权限下发不对: %+v", view.Entitlements)
	}

	const n = 10
	var (
		wg      sync.WaitGroup
		okCount atomic.Int64
		badErrs atomic.Int64
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := h.svc.Consume(ctx, view.AccessToken, "aim",
				fmt.Sprintf("nonce-concurrent-%02d-0123456789", i), "1.2.3.4", "ua")
			if err == nil {
				okCount.Add(1)
				return
			}
			if codeOfQuiet(err) != apierr.CardExhausted {
				badErrs.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if badErrs.Load() != 0 {
		t.Fatalf("%d 次失败不是 CARD_EXHAUSTED", badErrs.Load())
	}
	if okCount.Load() != 5 {
		t.Fatalf("成功次数 = %d want 5", okCount.Load())
	}
	c, _ := h.st.CardByKami(ctx, kami)
	if c.Points != 0 {
		t.Fatalf("points = %d want 0", c.Points)
	}
	if c.Status != store.CardExhausted {
		t.Fatalf("状态 = %s want exhausted", c.Status)
	}
	// 再消费一次必须失败
	if _, err := h.svc.Consume(ctx, view.AccessToken, "aim", "nonce-after-0123456789", "1.2.3.4", "ua"); err == nil {
		t.Fatal("点数耗尽后应拒绝")
	}
}

func codeOfQuiet(err error) string {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// consume 的 nonce 幂等：重复请求返回上次结果，不重复扣。
func TestConsumeNonceIdempotent(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "credit", Name: "credit", Kind: store.KindCredit,
		InitialPoints: 100, DeviceLimit: 3})
	entID, _ := h.st.CreateEntitlement(ctx, store.Entitlement{
		AppID: h.appID, Code: "aim", Name: "aim", CostPoints: 30, Active: 1, CreatedAt: 1000,
	})
	_ = h.st.SetPolicyEntitlements(ctx, p.ID, []int64{entID})
	kami := h.issue(t, p.ID)
	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}

	nonce := "nonce-idem-0123456789abcdef"
	first, err := h.svc.Consume(ctx, view.AccessToken, "aim", nonce, "ip", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if first.BalanceAfter != 70 || first.Replayed {
		t.Fatalf("第一次: balance=%d replayed=%v", first.BalanceAfter, first.Replayed)
	}
	second, err := h.svc.Consume(ctx, view.AccessToken, "aim", nonce, "ip", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.BalanceAfter != 70 {
		t.Fatalf("重放应返回上次结果: %+v", second)
	}
	c, _ := h.st.CardByKami(ctx, kami)
	if c.Points != 70 {
		t.Fatalf("重放不应重复扣: points = %d", c.Points)
	}
}

// 验收项 4：同一权限「附加 + 撤销」，撤销生效。
func TestEntitlementGrantRevoke(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "credit", Name: "credit", Kind: store.KindCredit,
		InitialPoints: 100, DeviceLimit: 3})
	basicID, _ := h.st.CreateEntitlement(ctx, store.Entitlement{
		AppID: h.appID, Code: "basic", Name: "basic", Active: 1, CreatedAt: 1000,
	})
	aimID, _ := h.st.CreateEntitlement(ctx, store.Entitlement{
		AppID: h.appID, Code: "aim", Name: "aim", CostPoints: 10, Active: 1, CreatedAt: 1000,
	})
	_ = h.st.SetPolicyEntitlements(ctx, p.ID, []int64{basicID})
	kami := h.issue(t, p.ID)
	c, _ := h.st.CardByKami(ctx, kami)

	// 附加 aim，撤销 basic
	tx, _ := h.st.DB().BeginTx(ctx, nil)
	if err := h.st.GrantCardEntitlementTx(ctx, tx, c.ID, aimID, 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := h.st.RevokeCardEntitlementTx(ctx, tx, c.ID, basicID, 1, h.now.Unix()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, e := range view.Entitlements {
		codes[e.Code] = true
	}
	if codes["basic"] {
		t.Fatal("撤销的权限不应下发")
	}
	if !codes["aim"] {
		t.Fatal("附加的权限应下发")
	}
	// 撤销的权限不能消费
	if _, err := h.svc.Consume(ctx, view.AccessToken, "basic", "nonce-basic-0123456789ab", "ip", "ua"); err == nil ||
		codeOfQuiet(err) != apierr.EntitlementMissing {
		t.Fatalf("撤销的权限应 ENTITLEMENT_MISSING: %v", err)
	}
}

func TestPauseAPI(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "timed", Name: "timed", Kind: store.KindTimed,
		DurationHours: 1, HeartbeatSec: 600})
	kami := h.issue(t, p.ID)
	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.svc.Pause(ctx, view.AccessToken, true, "ip", "ua")
	if err != nil {
		t.Fatalf("暂停: %v", err)
	}
	_, err = h.svc.Status(ctx, view.AccessToken, "ip", "ua")
	if err == nil || codeOfQuiet(err) != apierr.Paused {
		t.Fatalf("暂停后心跳应 PAUSED: %v", err)
	}
	if _, err := h.svc.Pause(ctx, view.AccessToken, false, "ip", "ua"); err != nil {
		t.Fatalf("恢复: %v", err)
	}
	if _, err := h.svc.Status(ctx, view.AccessToken, "ip", "ua"); err != nil {
		t.Fatalf("恢复后心跳应正常: %v", err)
	}

	// 非计时卡不能暂停
	p2 := h.makePolicy(t, store.Policy{Code: "forever", Name: "forever", Kind: store.KindForever})
	k2 := h.issue(t, p2.ID)
	v2, _ := h.login(k2, "fp-2")
	if _, err := h.svc.Pause(ctx, v2.AccessToken, true, "ip", "ua"); err == nil {
		t.Fatal("非计时卡暂停应报错")
	}
}

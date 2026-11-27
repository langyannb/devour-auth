package card_test

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

type env struct {
	st    *store.Store
	svc   *card.Service
	appID int64
}

func setup(t *testing.T) env {
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
	return env{st: st, svc: card.New(st), appID: appID}
}

func (e env) policy(t *testing.T, p store.Policy) store.Policy {
	t.Helper()
	p.AppID = e.appID
	if p.Active == 0 {
		p.Active = 1
	}
	p.DeviceLimit = orDefault(p.DeviceLimit, 2)
	p.ConcurrentLimit = orDefault(p.ConcurrentLimit, 1)
	p.HeartbeatSec = orDefault(p.HeartbeatSec, 600)
	p.CreatedAt = 1000
	id, err := e.st.CreatePolicy(context.Background(), p)
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	got, err := e.st.PolicyByID(context.Background(), id)
	if err != nil {
		t.Fatalf("policy by id: %v", err)
	}
	return got
}

func orDefault(v, def int64) int64 {
	if v == 0 {
		return def
	}
	return v
}

func (e env) generate(t *testing.T, policyID int64, qty int) card.GenerateResult {
	t.Helper()
	res, err := e.svc.Generate(context.Background(), card.GenerateReq{
		AppID:     e.appID,
		PolicyID:  policyID,
		Qty:       qty,
		Prefix:    "DV",
		CreatedBy: 1,
		Actor:     "admin:1",
		IP:        "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return res
}

func TestInitialBalancePerKind(t *testing.T) {
	ctx := context.Background()
	e := setup(t)

	cases := []struct {
		kind           string
		policy         store.Policy
		wantTimes      int64
		wantPoints     int64
		wantSeconds    int64
		wantExpireZero bool
	}{
		{store.KindDuration, store.Policy{DurationDays: 30}, 0, 0, 0, true},
		{store.KindForever, store.Policy{}, 0, 0, 0, true},
		{store.KindCount, store.Policy{InitialTimes: 10}, 10, 0, 0, true},
		{store.KindCredit, store.Policy{InitialPoints: 500}, 0, 500, 0, true},
		{store.KindTimed, store.Policy{DurationDays: 1, DurationHours: 2}, 0, 0, 93600, true},
		{store.KindTrial, store.Policy{DurationDays: 1}, 0, 0, 0, true},
	}

	for _, tc := range cases {
		p := e.policy(t, store.Policy{Code: tc.kind, Name: tc.kind, Kind: tc.kind,
			DurationDays: tc.policy.DurationDays, DurationHours: tc.policy.DurationHours,
			InitialTimes: tc.policy.InitialTimes, InitialPoints: tc.policy.InitialPoints})
		res := e.generate(t, p.ID, 1)
		c, err := e.st.CardByKami(ctx, res.Kamis[0])
		if err != nil {
			t.Fatalf("%s: 读卡: %v", tc.kind, err)
		}
		if c.Status != store.CardUnused {
			t.Errorf("%s: 新卡状态 = %s", tc.kind, c.Status)
		}
		if c.RemainTimes != tc.wantTimes {
			t.Errorf("%s: remain_times = %d want %d", tc.kind, c.RemainTimes, tc.wantTimes)
		}
		if c.Points != tc.wantPoints {
			t.Errorf("%s: points = %d want %d", tc.kind, c.Points, tc.wantPoints)
		}
		if c.RemainingSeconds != tc.wantSeconds {
			t.Errorf("%s: remaining_seconds = %d want %d", tc.kind, c.RemainingSeconds, tc.wantSeconds)
		}
		if tc.wantExpireZero && c.ExpireAt != 0 {
			t.Errorf("%s: expire_at = %d want 0", tc.kind, c.ExpireAt)
		}
	}
}

// 验收项 7：改卡类型不得改变已生成卡的行为。
func TestSnapshotFrozen(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "credit500", Name: "点数 500", Kind: store.KindCredit,
		InitialPoints: 500, DeviceLimit: 2})

	res := e.generate(t, p.ID, 1)
	before, err := e.st.CardByKami(ctx, res.Kamis[0])
	if err != nil {
		t.Fatal(err)
	}

	p.DeviceLimit = 99
	p.InitialPoints = 9999
	if err := e.st.UpdatePolicy(ctx, p); err != nil {
		t.Fatal(err)
	}

	after, err := e.st.CardByKami(ctx, res.Kamis[0])
	if err != nil {
		t.Fatal(err)
	}
	if after.DeviceLimit != 2 || after.Points != 500 {
		t.Fatalf("已生成的卡被改动了: device_limit=%d points=%d", after.DeviceLimit, after.Points)
	}
	if !strings.Contains(after.SnapshotJSON, `"device_limit":2`) {
		t.Fatalf("快照未冻结: %s", after.SnapshotJSON)
	}
	_ = before
}

func TestKamiFormatAndUniqueness(t *testing.T) {
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	res := e.generate(t, p.ID, 200)

	re := regexp.MustCompile(`^DV-[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}$`)
	seen := make(map[string]bool, len(res.Kamis))
	for _, k := range res.Kamis {
		if !re.MatchString(k) {
			t.Fatalf("卡密格式不对: %s", k)
		}
		if seen[k] {
			t.Fatalf("卡密重复: %s", k)
		}
		seen[k] = true
	}
}

func TestBanUnbanRecycleWritesAuditWithoutKami(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever, InitialPoints: 10})
	res := e.generate(t, p.ID, 1)
	kami := res.Kamis[0]
	c, err := e.st.CardByKami(ctx, kami)
	if err != nil {
		t.Fatal(err)
	}

	ch := card.StatusChange{Actor: "admin:1", IP: "127.0.0.1"}
	if err := e.svc.Ban(ctx, c.ID, ch); err != nil {
		t.Fatal(err)
	}
	banned, _ := e.st.CardByID(ctx, c.ID)
	if banned.Status != store.CardBanned || !banned.BannedAt.Valid {
		t.Fatalf("封禁未生效: %+v", banned)
	}
	if err := e.svc.Ban(ctx, c.ID, ch); err == nil {
		t.Fatal("重复封禁应报错")
	}

	if err := e.svc.Unban(ctx, c.ID, ch); err != nil {
		t.Fatal(err)
	}
	unbanned, _ := e.st.CardByID(ctx, c.ID)
	if unbanned.Status != store.CardUnused || unbanned.BannedAt.Valid {
		t.Fatalf("解封未回到 unused: %+v", unbanned)
	}

	if err := e.svc.Recycle(ctx, c.ID, ch); err != nil {
		t.Fatal(err)
	}
	rec, _ := e.st.CardByID(ctx, c.ID)
	if rec.Status != store.CardRecycled {
		t.Fatalf("回收未生效: %s", rec.Status)
	}

	var n int64
	if err := e.st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 4 { // generate + ban + unban + recycle
		t.Fatalf("审计条数 = %d，期望 ≥ 4", n)
	}
	var detail string
	if err := e.st.DB().QueryRowContext(ctx,
		`SELECT COALESCE(before,'') || COALESCE(after,'') || COALESCE(target,'') FROM audit_log`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(detail, kami) {
		t.Fatal("审计里出现了卡密原文")
	}
}

func TestExtend(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	ch := card.StatusChange{Actor: "admin:1"}

	// 点数卡：续点数
	pc := e.policy(t, store.Policy{Code: "credit", Name: "credit", Kind: store.KindCredit, InitialPoints: 100})
	res := e.generate(t, pc.ID, 1)
	c, _ := e.st.CardByKami(ctx, res.Kamis[0])
	if err := e.svc.Extend(ctx, c.ID, card.ExtendAdd{Points: 50}, ch); err != nil {
		t.Fatal(err)
	}
	got, _ := e.st.CardByID(ctx, c.ID)
	if got.Points != 150 {
		t.Fatalf("points = %d want 150", got.Points)
	}

	// 永久卡：加天数应拒绝
	pf := e.policy(t, store.Policy{Code: "forever", Name: "forever", Kind: store.KindForever})
	res2 := e.generate(t, pf.ID, 1)
	c2, _ := e.st.CardByKami(ctx, res2.Kamis[0])
	if err := e.svc.Extend(ctx, c2.ID, card.ExtendAdd{Days: 30}, ch); err == nil {
		t.Fatal("永久卡加天数应拒绝")
	}

	// 负数拒绝
	if err := e.svc.Extend(ctx, c.ID, card.ExtendAdd{Points: -1}, ch); err == nil {
		t.Fatal("负数增量应拒绝")
	}
}

func TestGenerateRejectsBadQty(t *testing.T) {
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	for _, q := range []int{0, -1, card.MaxBatchQty + 1} {
		if _, err := e.svc.Generate(context.Background(), card.GenerateReq{
			AppID: e.appID, PolicyID: p.ID, Qty: q, CreatedBy: 1, Actor: "admin:1",
		}); err == nil {
			t.Fatalf("qty=%d 应被拒绝", q)
		}
	}
}

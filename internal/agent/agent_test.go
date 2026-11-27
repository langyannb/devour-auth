package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

type env struct {
	st    *store.Store
	svc   *Service
	appID int64
	now   time.Time
}

func setup(t *testing.T) *env {
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
		t.Fatal(err)
	}
	e := &env{st: st, svc: New(st), appID: appID, now: time.Unix(1_800_000_000, 0)}
	e.svc.now = func() time.Time { return e.now }
	return e
}

func (e *env) policy(t *testing.T, priceCents int64) store.Policy {
	t.Helper()
	p := store.Policy{
		AppID: e.appID, Code: "day30", Name: "30 天卡", Kind: store.KindDuration,
		DurationDays: 30, DeviceLimit: 2, ConcurrentLimit: 1, HeartbeatSec: 600,
		OfflineHours: 24, RebindFree: 1, RebindCooldownDays: 30, Renewable: 1,
		PriceCents: priceCents, Active: 1, CreatedAt: e.now.Unix(),
	}
	id, err := e.st.CreatePolicy(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.st.PolicyByID(context.Background(), id)
	return got
}

func (e *env) agent(t *testing.T, name string, parent, discountBP, rebateBP, quota int64) store.Agent {
	t.Helper()
	hash, err := auth.HashPassword("a-very-long-agent-password")
	if err != nil {
		t.Fatal(err)
	}
	a := store.Agent{
		Name: name, Level: 1, DiscountBP: discountBP, RebateBP: rebateBP,
		MonthlyQuotaCents: quota, PassHash: hash, Status: "active", CreatedAt: e.now.Unix(),
	}
	if parent > 0 {
		a.ParentID = store.NullInt64(parent)
		a.Level = 2
	}
	id, err := e.st.CreateAgent(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.st.AgentByID(context.Background(), id)
	return got
}

func TestIssuePricingAndLedger(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, 1000) // 10 元
	a := e.agent(t, "ag1", 0, 7000, 0, 0)

	res, err := e.svc.Issue(ctx, IssueReq{AgentID: a.ID, PolicyID: p.ID, Qty: 3, Prefix: "AG"})
	if err != nil {
		t.Fatalf("出卡: %v", err)
	}
	if len(res.Kamis) != 3 || res.BatchID == 0 {
		t.Fatalf("结果不对: %+v", res)
	}
	// 3 × 1000 × 70% = 2100
	if res.Payable != 2100 {
		t.Fatalf("应付 = %d want 2100", res.Payable)
	}
	rows, total, err := e.st.ListAgentLedger(ctx, a.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || rows[0].Kind != "issue" || rows[0].AmountCents != 2100 {
		t.Fatalf("流水不对: %+v", rows)
	}
}

func TestQuotaBlocksIssue(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, 1000)
	a := e.agent(t, "ag2", 0, 10000, 0, 2500) // 额度 2500 分

	if _, err := e.svc.Issue(ctx, IssueReq{AgentID: a.ID, PolicyID: p.ID, Qty: 2, Prefix: "AG"}); err != nil {
		t.Fatalf("首批发卡应通过: %v", err)
	}
	// 已用 2000，再来 1 张 1000 → 3000 超过 2500
	_, err := e.svc.Issue(ctx, IssueReq{AgentID: a.ID, PolicyID: p.ID, Qty: 1, Prefix: "AG"})
	if err == nil || !errors.Is(err, ErrQuota) {
		t.Fatalf("应被额度拒绝: %v", err)
	}
}

func TestRebateToParent(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, 1000)
	parent := e.agent(t, "top", 0, 6000, 500, 0) // 返点 5%
	child := e.agent(t, "sub", parent.ID, 8000, 0, 0)

	res, err := e.svc.Issue(ctx, IssueReq{AgentID: child.ID, PolicyID: p.ID, Qty: 2, Prefix: "AG"})
	if err != nil {
		t.Fatal(err)
	}
	// 应付 2 × 1000 × 80% = 1600，上级返点 5% = 80
	if res.Payable != 1600 || res.Rebate != 80 {
		t.Fatalf("payable=%d rebate=%d", res.Payable, res.Rebate)
	}
	rows, _, err := e.st.ListAgentLedger(ctx, parent.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Kind != "rebate" || rows[0].AmountCents != -80 {
		t.Fatalf("上级返点流水不对: %+v", rows)
	}
}

func TestFrozenAgentCannotIssue(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, 1000)
	a := e.agent(t, "ag3", 0, 8000, 0, 0)
	if err := e.st.SetAgentStatus(ctx, a.ID, "frozen", store.NullInt64(e.now.Unix())); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Issue(ctx, IssueReq{AgentID: a.ID, PolicyID: p.ID, Qty: 1}); err == nil {
		t.Fatal("冻结代理不应能出卡")
	}
}

func TestVisibleScope(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	parent := e.agent(t, "top2", 0, 6000, 0, 0)
	child := e.agent(t, "sub2", parent.ID, 8000, 0, 0)
	other := e.agent(t, "other", 0, 8000, 0, 0)

	ids, err := e.svc.VisibleAgentIDs(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[parent.ID] || !seen[child.ID] {
		t.Fatalf("应能看到自己与下级: %v", ids)
	}
	if seen[other.ID] {
		t.Fatal("不应看到其他代理")
	}
}

func TestBillGeneration(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, 1000)
	a := e.agent(t, "ag4", 0, 10000, 0, 0)

	e.now = time.Date(2026, 11, 15, 10, 0, 0, 0, time.Local)
	if _, err := e.svc.Issue(ctx, IssueReq{AgentID: a.ID, PolicyID: p.ID, Qty: 2, Prefix: "AG"}); err != nil {
		t.Fatal(err)
	}

	n, err := e.svc.GenerateBills(ctx, "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("生成账单数 = %d want 1", n)
	}
	bills, err := e.st.ListAgentBills(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 1 || bills[0].AmountCents != 2000 || bills[0].Status != "open" {
		t.Fatalf("账单不对: %+v", bills)
	}
}

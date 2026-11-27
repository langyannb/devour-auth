package card_test

import (
	"context"
	"errors"
	"testing"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

func bindDevice(t *testing.T, st *store.Store, cardID int64, fp string, at int64) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(),
		`INSERT INTO devices(card_id, fingerprint, first_seen, last_seen) VALUES(?,?,?,?)`,
		cardID, fp, at, at); err != nil {
		t.Fatal(err)
	}
}

// 换绑冷却：冷却期内不允许再次自助解绑。
func TestSelfUnbindCooldown(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{
		Code: "cd", Name: "cd", Kind: store.KindForever,
		RebindFree: 5, RebindCooldownDays: 1,
	})
	res := e.generate(t, p.ID, 1)
	c, _ := e.st.CardByKami(ctx, res.Kamis[0])

	bindDevice(t, e.st, c.ID, "fp-1", e.svc.NowUnixForTest())
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err != nil {
		t.Fatalf("首次换绑: %v", err)
	}

	// 立刻再换：冷却中
	bindDevice(t, e.st, c.ID, "fp-2", e.svc.NowUnixForTest())
	err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"})
	if err == nil || !errors.Is(err, card.ErrState) {
		t.Fatalf("冷却期内应拒绝: %v", err)
	}

	// 推进 2 天后再换：允许
	e.svc.SetNowForTest(e.svc.NowUnixForTest() + 2*86400)
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err != nil {
		t.Fatalf("冷却过后应允许: %v", err)
	}
}

// 超出免费次数后按 rebind_cost_hours 扣剩余时长。
func TestSelfUnbindDeductsTime(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{
		Code: "timed", Name: "timed", Kind: store.KindTimed,
		DurationHours: 2, RebindFree: 0, RebindCostHours: 1, RebindCooldownDays: 0,
	})
	res := e.generate(t, p.ID, 1)
	c, _ := e.st.CardByKami(ctx, res.Kamis[0])
	if c.RemainingSeconds != 7200 {
		t.Fatalf("初始剩余 = %d", c.RemainingSeconds)
	}
	bindDevice(t, e.st, c.ID, "fp-1", e.svc.NowUnixForTest())

	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err != nil {
		t.Fatalf("扣时换绑: %v", err)
	}
	got, _ := e.st.CardByID(ctx, c.ID)
	if got.RemainingSeconds != 3600 {
		t.Fatalf("扣时后剩余 = %d want 3600", got.RemainingSeconds)
	}

	// 剩 1 小时，再扣 1 小时会被拒绝（要求严格大于）
	bindDevice(t, e.st, c.ID, "fp-2", e.svc.NowUnixForTest())
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err != nil {
		t.Fatalf("刚好剩 1 小时应允许: %v", err)
	}
	got, _ = e.st.CardByID(ctx, c.ID)
	if got.RemainingSeconds != 0 {
		t.Fatalf("第二次扣时后剩余 = %d want 0", got.RemainingSeconds)
	}
}

// 永久卡超出免费次数时无法扣时，应明确拒绝。
func TestSelfUnbindRejectsTimeCostOnForever(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{
		Code: "fw", Name: "fw", Kind: store.KindForever,
		RebindFree: 0, RebindCostHours: 1, RebindCooldownDays: 0,
	})
	res := e.generate(t, p.ID, 1)
	c, _ := e.st.CardByKami(ctx, res.Kamis[0])
	bindDevice(t, e.st, c.ID, "fp-1", e.svc.NowUnixForTest())

	err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"})
	if err == nil || !errors.Is(err, card.ErrState) {
		t.Fatalf("永久卡扣时应拒绝: %v", err)
	}
}

package card_test

import (
	"context"
	"errors"
	"testing"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

func TestSelfUnbindRespectsFreeQuota(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever, RebindFree: 1})
	res := e.generate(t, p.ID, 1)
	c, err := e.st.CardByKami(ctx, res.Kamis[0])
	if err != nil {
		t.Fatal(err)
	}

	// 没绑定设备时应拒绝
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err == nil ||
		!errors.Is(err, card.ErrState) {
		t.Fatalf("无设备时应拒绝: %v", err)
	}

	// 绑一台设备
	if _, err := e.st.DB().ExecContext(ctx,
		`INSERT INTO devices(card_id, fingerprint, first_seen, last_seen) VALUES(?,?,?,?)`,
		c.ID, "fp-x", 1000, 1000); err != nil {
		t.Fatal(err)
	}

	// 第一次免费解绑成功
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err != nil {
		t.Fatalf("首次解绑应成功: %v", err)
	}
	got, _ := e.st.CardByID(ctx, c.ID)
	if got.RebindUsed != 1 {
		t.Fatalf("rebind_used = %d want 1", got.RebindUsed)
	}
	bound, _ := e.st.CountBoundDevices(ctx, c.ID)
	if bound != 0 {
		t.Fatalf("解绑后设备数 = %d", bound)
	}

	// 再绑一台，第二次应被免费次数挡住
	if _, err := e.st.DB().ExecContext(ctx,
		`INSERT INTO devices(card_id, fingerprint, first_seen, last_seen) VALUES(?,?,?,?)`,
		c.ID, "fp-y", 2000, 2000); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SelfUnbind(ctx, c.ID, card.StatusChange{Actor: "user:query"}); err == nil {
		t.Fatal("免费次数用完应拒绝")
	}
}

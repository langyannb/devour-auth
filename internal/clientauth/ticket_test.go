package clientauth

import (
	"context"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/store"
)

// 回归：TicketData 在事务里读设备，若用 *sql.DB 会与单连接互相等死。
func TestTicketDataDoesNotDeadlock(t *testing.T) {
	ctx := context.Background()
	h := setup(t)
	p := h.makePolicy(t, store.Policy{Code: "c", Name: "c", Kind: store.KindForever})
	kami := h.issue(t, p.ID)

	view, err := h.login(kami, "fp-1")
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		card store.Card
		fp   string
		ents []store.Entitlement
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, _, fp, ents, err := h.svc.TicketData(ctx, view.AccessToken)
		ch <- result{card: c, fp: fp, ents: ents, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("TicketData: %v", r.err)
		}
		if r.card.ID == 0 || r.fp != "fp-1" {
			t.Fatalf("返回不对: card=%d fp=%q", r.card.ID, r.fp)
		}
		if len(r.ents) != 0 {
			t.Fatalf("该卡没有权限，得到 %d 个", len(r.ents))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("TicketData 超时：事务内可能混用了 *sql.DB")
	}
}

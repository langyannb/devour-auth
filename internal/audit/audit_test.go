package audit_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/store"
)

func newStore(t *testing.T) *store.Store {
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
	return st
}

func appendOne(t *testing.T, st *store.Store, e audit.Entry) {
	t.Helper()
	ctx := context.Background()
	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := audit.Append(ctx, tx, e); err != nil {
		tx.Rollback()
		t.Fatalf("append: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestChainVerify(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)

	appendOne(t, st, audit.Entry{TS: 1, Actor: "admin:1", Action: "card.create", Target: "card:1", After: `{"kami":"DV-A"}`, IP: "1.2.3.4"})
	appendOne(t, st, audit.Entry{TS: 2, Actor: "admin:1", Action: "card.ban", Target: "card:1", Before: `{"status":"active"}`, After: `{"status":"banned"}`, IP: "1.2.3.4"})

	n, err := audit.Verify(ctx, st.DB())
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if n != 2 {
		t.Fatalf("verify 条数 = %d，期望 2", n)
	}
}

// 触发器必须拦住改写；插入伪造记录必须被 Verify 发现。
func TestChainTamperDetection(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	appendOne(t, st, audit.Entry{TS: 1, Actor: "admin:1", Action: "a"})

	if _, err := st.DB().ExecContext(ctx, `UPDATE audit_log SET action='b' WHERE id=1`); err == nil {
		t.Fatal("UPDATE audit_log 应当被触发器拒绝")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("错误信息不对: %v", err)
	}

	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO audit_log(ts, actor, action, prev_hash, hash) VALUES(2,'admin:9','fake','x','y')`); err != nil {
		t.Fatalf("插入: %v", err)
	}
	if _, err := audit.Verify(ctx, st.DB()); err == nil {
		t.Fatal("伪造记录应当被 Verify 发现")
	}
}

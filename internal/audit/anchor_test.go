package audit_test

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"

	"github.com/langyannb/devour-auth/internal/audit"
)

func TestAnchorAndVerify(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	appendOne(t, st, audit.Entry{TS: 1, Actor: "admin:1", Action: "a"})
	appendOne(t, st, audit.Entry{TS: 2, Actor: "admin:1", Action: "b"})

	keyPath := filepath.Join(t.TempDir(), "audit.key")
	priv, err := audit.LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("生成私钥: %v", err)
	}
	// 二次加载返回同一把钥匙
	again, err := audit.LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !priv.Equal(again) {
		t.Fatal("重复加载应得到同一把私钥")
	}

	if ok, err := audit.Anchor(ctx, st.DB(), priv, "2026-11-27", 100); err != nil || !ok {
		t.Fatalf("锚定: ok=%v err=%v", ok, err)
	}
	// 同一天重复锚定应跳过
	if ok, err := audit.Anchor(ctx, st.DB(), priv, "2026-11-27", 200); err != nil || ok {
		t.Fatalf("重复锚定应跳过: ok=%v err=%v", ok, err)
	}

	pub := priv.Public().(ed25519.PublicKey)
	n, err := audit.VerifyAnchors(ctx, st.DB(), pub)
	if err != nil {
		t.Fatalf("校验锚点: %v", err)
	}
	if n != 1 {
		t.Fatalf("锚点数 = %d want 1", n)
	}

	// 篡改锚点记录 → 校验失败
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE audit_anchor SET last_hash = 'deadbeef' WHERE day = '2026-11-27'`); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.VerifyAnchors(ctx, st.DB(), pub); err == nil {
		t.Fatal("篡改后的锚点应校验失败")
	}
}

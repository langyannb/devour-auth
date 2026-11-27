package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/langyannb/devour-auth/internal/store"
)

func TestRollupDay(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	appID, err := st.CreateApp(ctx, "devour", "Devour", 1000)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := st.CreatePolicy(ctx, store.Policy{
		AppID: appID, Code: "p", Name: "p", Kind: store.KindDuration,
		DurationDays: 1, DeviceLimit: 1, ConcurrentLimit: 1, HeartbeatSec: 600,
		Active: 1, PriceCents: 990, CreatedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 同一天：两张卡、三条登录事件（两台设备）
	const dayStart int64 = 1_800_000_000
	dayEnd := dayStart + 86400
	for i := 0; i < 2; i++ {
		if _, err := st.DB().ExecContext(ctx,
			`INSERT INTO cards(app_id, kami, policy_id, kind, status, device_limit, snapshot_json, activated_at, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			appID, "K"+string(rune('A'+i)), pid, store.KindDuration, store.CardActive,
			1, "{}", dayStart+10, dayStart+5); err != nil {
			t.Fatal(err)
		}
	}
	for i, dev := range []int64{7, 7, 9} {
		ts := dayStart + int64(i*100)
		if _, err := st.DB().ExecContext(ctx,
			`INSERT INTO events(ts, kind, device_id) VALUES(?, 'login', ?)`, ts, dev); err != nil {
			t.Fatal(err)
		}
	}
	// 前一天的一条，不该被算进来
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO events(ts, kind, device_id) VALUES(?, 'login', ?)`, dayStart-100, 5); err != nil {
		t.Fatal(err)
	}

	stat, err := st.RollupDay(ctx, "2027-01-15", dayStart, dayEnd)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Activations != 2 || stat.Logins != 3 || stat.UniqueDevices != 2 || stat.IssuedCents != 1980 {
		t.Fatalf("汇总不对: %+v", stat)
	}

	// 重复执行应覆盖而不是累加
	again, err := st.RollupDay(ctx, "2027-01-15", dayStart, dayEnd)
	if err != nil {
		t.Fatal(err)
	}
	if again.Logins != 3 {
		t.Fatalf("重复汇总应覆盖: %+v", again)
	}

	list, err := st.ListDailyStats(ctx, "2027-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Day != "2027-01-15" {
		t.Fatalf("列表不对: %+v", list)
	}
}

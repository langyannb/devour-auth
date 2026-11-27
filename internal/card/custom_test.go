package card_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

func TestGenerateCustomKamis(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "custom", Name: "custom", Kind: store.KindForever})

	// 正常：两张指定卡密
	res, err := e.svc.Generate(ctx, card.GenerateReq{
		AppID: e.appID, PolicyID: p.ID, Prefix: "IGNORED",
		Kamis:     []string{"VIP-0001", "VIP-0002"},
		CreatedBy: 1, Actor: "admin:1",
	})
	if err != nil {
		t.Fatalf("自定义发卡: %v", err)
	}
	if len(res.Kamis) != 2 || res.Kamis[0] != "VIP-0001" {
		t.Fatalf("返回卡密不对: %v", res.Kamis)
	}
	if _, err := e.st.CardByKami(ctx, "VIP-0002"); err != nil {
		t.Fatalf("库里应有 VIP-0002: %v", err)
	}

	// qty 与列表长度不一致
	if _, err := e.svc.Generate(ctx, card.GenerateReq{
		AppID: e.appID, PolicyID: p.ID, Qty: 3, Kamis: []string{"X-1"},
		CreatedBy: 1, Actor: "admin:1",
	}); err == nil {
		t.Fatal("数量不一致应报错")
	}

	// 同批重复
	if _, err := e.svc.Generate(ctx, card.GenerateReq{
		AppID: e.appID, PolicyID: p.ID, Kamis: []string{"DUP-1", "DUP-1"},
		CreatedBy: 1, Actor: "admin:1",
	}); err == nil {
		t.Fatal("同批重复应报错")
	}

	// 与库里已有重复
	if _, err := e.svc.Generate(ctx, card.GenerateReq{
		AppID: e.appID, PolicyID: p.ID, Kamis: []string{"VIP-0001"},
		CreatedBy: 1, Actor: "admin:1",
	}); err == nil {
		t.Fatal("与已有卡密重复应报错")
	}

	// 太短
	if _, err := e.svc.Generate(ctx, card.GenerateReq{
		AppID: e.appID, PolicyID: p.ID, Kamis: []string{"A"},
		CreatedBy: 1, Actor: "admin:1",
	}); err == nil {
		t.Fatal("过短卡密应报错")
	}
}

func TestSetDeviceLimitUpdatesSnapshot(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	p := e.policy(t, store.Policy{Code: "dl", Name: "dl", Kind: store.KindForever, DeviceLimit: 2})
	res := e.generate(t, p.ID, 1)
	c, _ := e.st.CardByKami(ctx, res.Kamis[0])

	if err := e.svc.SetDeviceLimit(ctx, c.ID, 5, card.StatusChange{Actor: "admin:1"}); err != nil {
		t.Fatalf("改上限: %v", err)
	}
	got, _ := e.st.CardByID(ctx, c.ID)
	if got.DeviceLimit != 5 {
		t.Fatalf("device_limit = %d", got.DeviceLimit)
	}
	snap := card.Snapshot{}
	if err := jsonUnmarshal(got.SnapshotJSON, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.DeviceLimit != 5 {
		t.Fatalf("快照 device_limit = %d，登录会按旧值执行", snap.DeviceLimit)
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/langyannb/devour-auth/internal/store"
)

// 单码卡密 = 份数：10 份天卡是 10 天，价格也必须 ×10，否则代理可以按 1 天的价卖 10 天。
func TestIssueByPlanMultiplierPriced(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	a := e.agent(t, "agplan", 0, 10000, 0, -1)

	planID, err := e.st.SaveAgentPlan(ctx, store.AgentPlan{
		AppID: e.appID, Name: "p1", Active: 1,
		Items: map[string]int64{"day": 300, "forever": 9900},
	}, e.now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.st.SetAgentPlanAndPerms(ctx, a.ID, planID, store.AgentPermissions{MaxQty: 10}); err != nil {
		t.Fatal(err)
	}

	res, err := e.svc.IssueByPlan(ctx, PlanIssueReq{
		AgentID: a.ID, AppID: e.appID, Kind: "day", Multiplier: 10, Qty: 2, DeviceLimit: 1,
	})
	if err != nil {
		t.Fatalf("出卡: %v", err)
	}
	// 300 分 × 10 份 × 2 张 = 6000 分
	if res.Payable != 6000 {
		t.Fatalf("应付 = %d want 6000", res.Payable)
	}

	var snapJSON string
	if err := e.st.DB().QueryRow(`SELECT snapshot_json FROM cards WHERE batch_id = ?`, res.BatchID).Scan(&snapJSON); err != nil {
		t.Fatal(err)
	}
	var snap struct {
		DurationSeconds int64 `json:"duration_seconds"`
	}
	if err := json.Unmarshal([]byte(snapJSON), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.DurationSeconds != 10*86400 {
		t.Fatalf("时长 = %d 秒 want 10 天", snap.DurationSeconds)
	}

	rows, _, err := e.st.ListAgentLedger(ctx, a.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].AmountCents != 6000 {
		t.Fatalf("流水不对: %+v", rows)
	}

	// 永久卡不受份数影响
	res2, err := e.svc.IssueByPlan(ctx, PlanIssueReq{
		AgentID: a.ID, AppID: e.appID, Kind: "forever", Multiplier: 5, Qty: 1, DeviceLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Payable != 9900 {
		t.Fatalf("永久卡应付 = %d want 9900", res2.Payable)
	}
}

package agent

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// KindBaseSeconds 各类型单份时长。
var KindBaseSeconds = map[string]int64{
	"day":   86400,
	"week":  7 * 86400,
	"month": 30 * 86400,
}

func unitSeconds(unit string) int64 {
	switch unit {
	case "minute":
		return 60
	case "hour":
		return 3600
	default:
		return 86400
	}
}

// PlanIssueReq 代理按「价格模板」出卡。
type PlanIssueReq struct {
	AgentID     int64
	AppID       int64
	Kind        string // day|week|month|forever|custom
	Multiplier  int64
	Value       int64
	Unit        string
	Qty         int
	DeviceLimit int64
	Kamis       []string
	Note        string
	IP          string
}

// IssueByPlan 出卡：价格来自代理绑定的模板，权限来自代理配置。
func (s *Service) IssueByPlan(ctx context.Context, req PlanIssueReq) (IssueResult, error) {
	if req.Kind == "" {
		return IssueResult{}, apierr.New(apierr.BadInput, "缺少卡类型")
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Multiplier <= 0 {
		req.Multiplier = 1
	}

	a, err := s.st.AgentByID(ctx, req.AgentID)
	if err != nil {
		return IssueResult{}, err
	}
	if a.Status != "active" {
		return IssueResult{}, apierr.New(apierr.Unauthorized, "账号已冻结")
	}

	perms, planID, err := s.st.AgentPermissions(ctx, a.ID)
	if err != nil {
		return IssueResult{}, err
	}
	if planID == 0 {
		return IssueResult{}, apierr.New(apierr.BadInput, "管理员还没有给你分配价格模板")
	}
	plan, err := s.st.AgentPlanByID(ctx, planID)
	if err != nil {
		return IssueResult{}, apierr.New(apierr.BadInput, "价格模板不存在")
	}
	unitPrice, ok := plan.Items[req.Kind]
	if !ok || unitPrice <= 0 {
		return IssueResult{}, apierr.New(apierr.BadInput, "这个模板里没有该卡类型")
	}

	// 数量上限
	if perms.MaxQty > 0 && int64(req.Qty) > perms.MaxQty {
		return IssueResult{}, apierr.New(apierr.BadInput, "单批最多 %d 张", perms.MaxQty)
	}
	// 设备数量：没权限就固定 1
	if !perms.SetDeviceLimit {
		req.DeviceLimit = 1
	} else if req.DeviceLimit <= 0 {
		req.DeviceLimit = 1
	}
	// 自定义卡密：没权限就拒绝
	if len(req.Kamis) > 0 && !perms.CustomKami {
		return IssueResult{}, apierr.New(apierr.BadInput, "没有自定义卡密的权限")
	}

	// 时长
	var duration int64
	switch req.Kind {
	case "forever":
		duration = 0
	case "custom":
		v := req.Value
		if v <= 0 {
			v = 1
		}
		duration = v * unitSeconds(req.Unit) * req.Multiplier
	default:
		duration = KindBaseSeconds[req.Kind] * req.Multiplier
	}

	// 单码卡密按份数计价：10 份天卡 = 10 天，价格也 ×10；永久卡不受份数影响
	factor := req.Multiplier
	if req.Kind == "forever" {
		factor = 1
	}
	if req.Kind == "custom" {
		// 自定义时间的单价按所选单位计：3 天 = 3 × 单价
		v := req.Value
		if v <= 0 {
			v = 1
		}
		factor *= v
	}
	payable := unitPrice * int64(req.Qty) * factor
	now := s.now()

	// 额度：-1 = 不限，其余为可用余额
	if a.QuotaCents >= 0 && a.QuotaCents < payable {
		return IssueResult{}, fmt.Errorf("%w: 剩余额度 %d 分，本次需要 %d 分", ErrQuota, a.QuotaCents, payable)
	}

	// 模板卡类型对应的内部策略
	policyKind := store.KindDuration
	if req.Kind == "forever" {
		policyKind = store.KindForever
	}
	tpl, err := s.st.EnsureQuickPolicy(ctx, req.AppID, policyKind, now.Unix())
	if err != nil {
		return IssueResult{}, err
	}

	var parent store.Agent
	if a.ParentID.Valid {
		if parent, err = s.st.AgentByID(ctx, a.ParentID.Int64); err != nil {
			return IssueResult{}, err
		}
	}
	rebate := payable * parent.RebateBP / 10000

	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID:           req.AppID,
		PolicyID:        tpl.ID,
		Qty:             req.Qty,
		KamiLength:      card.DefaultKamiLength,
		DurationSeconds: duration,
		DeviceLimit:     req.DeviceLimit,
		AgentID:         a.ID,
		CreatedBy:       a.ID,
		Actor:           fmt.Sprintf("agent:%d", a.ID),
		IP:              req.IP,
		Note:            req.Note,
		Kamis:           req.Kamis,
		Hook: func(ctx context.Context, tx *sql.Tx, batchID int64) error {
			ref := fmt.Sprintf("batch:%d", batchID)
			if a.QuotaCents >= 0 {
				res, err := tx.ExecContext(ctx,
					`UPDATE agents SET quota_cents = quota_cents - ? WHERE id = ? AND quota_cents >= ?`,
					payable, a.ID, payable)
				if err != nil {
					return err
				}
				if n, _ := res.RowsAffected(); n == 0 {
					return fmt.Errorf("%w: 额度不足", ErrQuota)
				}
			}
			if err := s.st.AddAgentLedgerTx(ctx, tx, a.ID, "issue", payable, ref, now.Unix()); err != nil {
				return err
			}
			if a.ParentID.Valid && rebate > 0 {
				return s.st.AddAgentLedgerTx(ctx, tx, parent.ID, "rebate", -rebate, ref, now.Unix())
			}
			return nil
		},
	})
	if err != nil {
		return IssueResult{}, err
	}
	return IssueResult{BatchID: res.BatchID, Kamis: res.Kamis, Payable: payable, Rebate: rebate}, nil
}

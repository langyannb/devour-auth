package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runSeed 造本地测试数据：权限、六种卡类型、每种两张卡。可重复执行。
func runSeed(cfg config.Config) error {
	ctx := context.Background()
	if dir := filepath.Dir(cfg.DBPath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	now := time.Now().Unix()
	appID, err := st.EnsureApp(ctx, "devour", "Devour", now)
	if err != nil {
		return err
	}

	ents := []struct {
		code, name string
		points     int64
	}{
		{"basic", "基础功能", 0},
		{"aim_assist", "辅助瞄准", 2},
		{"macro", "宏录制", 5},
	}
	entIDs := make([]int64, 0, len(ents))
	for _, e := range ents {
		existing, err := st.EntitlementByCode(ctx, appID, e.code)
		if err == nil {
			entIDs = append(entIDs, existing.ID)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		id, err := st.CreateEntitlement(ctx, store.Entitlement{
			AppID: appID, Code: e.code, Name: e.name,
			CostPoints: e.points, Active: 1, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		entIDs = append(entIDs, id)
	}

	specs := []store.Policy{
		{Code: "day1", Name: "天卡", Kind: store.KindDuration, DurationDays: 1, PriceCents: 300},
		{Code: "week1", Name: "周卡", Kind: store.KindDuration, DurationDays: 7, PriceCents: 1500},
		{Code: "month1", Name: "月卡", Kind: store.KindDuration, DurationDays: 30, PriceCents: 2500},
		{Code: "forever", Name: "永久卡", Kind: store.KindForever, PriceCents: 9990},
	}

	cards := card.New(st)
	created := 0
	for _, p := range specs {
		p.AppID = appID
		p.Active = 1
		if p.DeviceLimit == 0 {
			p.DeviceLimit = 2
		}
		if p.ConcurrentLimit == 0 {
			p.ConcurrentLimit = 1
		}
		if p.HeartbeatSec == 0 {
			p.HeartbeatSec = 600
		}
		if p.OfflineHours == 0 {
			p.OfflineHours = 24
		}
		if p.RebindFree == 0 {
			p.RebindFree = 1
		}
		if p.RebindCooldownDays == 0 {
			p.RebindCooldownDays = 30
		}
		p.Renewable = 1
		p.CreatedAt = now

		var (
			policyID int64
			isNew    bool
		)
		existing, err := st.PolicyByCode(ctx, appID, p.Code)
		switch {
		case err == nil:
			policyID = existing.ID
			fmt.Printf("跳过已存在的卡类型 %s\n", p.Code)
		case errors.Is(err, store.ErrNotFound):
			policyID, err = st.CreatePolicy(ctx, p)
			if err != nil {
				return err
			}
			if err := st.SetPolicyEntitlements(ctx, policyID, entIDs); err != nil {
				return err
			}
			isNew = true
			fmt.Printf("新建卡类型 %s（%s）\n", p.Code, p.Kind)
		default:
			return err
		}
		if !isNew {
			continue
		}

		res, err := cards.Generate(ctx, card.GenerateReq{
			AppID: appID, PolicyID: policyID, Qty: 2, Prefix: "TEST",
			CreatedBy: 0, Actor: "seed",
		})
		if err != nil {
			return err
		}
		created += len(res.Kamis)
	}

	fmt.Printf("\n测试数据就绪：应用 devour、权限 %d 个、卡类型 %d 种、新发卡 %d 张\n", len(ents), len(specs), created)
	return nil
}

package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// quickReq 发卡页的简化入参。
//
// 单码卡密（Multiplier）= 一张卡的时长倍数：周卡 + 2 → 2 周。
// 卡密数量（Qty）= 生成几张卡，两者互不影响。
type quickReq struct {
	Kind        string `json:"kind"` // day|week|month|forever|custom
	Multiplier  int64  `json:"multiplier"`
	Value       int64  `json:"value"` // custom 用
	Unit        string `json:"unit"`  // minute|hour|day（custom 用）
	DeviceLimit int64  `json:"device_limit"`
	Length      int    `json:"length"`
	Prefix      string `json:"prefix"`
	Qty         int    `json:"qty"`
	Note        string `json:"note"`
}

// baseSeconds 各类型的单码基础时长。
var baseSeconds = map[string]int64{
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

func (s *Server) quickGenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req quickReq
	if !decode(w, r, &req) {
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.DeviceLimit <= 0 {
		req.DeviceLimit = 1
	}
	if req.Length <= 0 {
		req.Length = card.DefaultKamiLength
	}
	if req.Multiplier <= 0 {
		req.Multiplier = 1
	}
	if req.Multiplier > 10000 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "单码卡密倍数过大")
		return
	}

	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}

	var (
		policyKind string
		duration   int64
	)
	switch req.Kind {
	case "day", "week", "month":
		policyKind = store.KindDuration
		duration = baseSeconds[req.Kind] * req.Multiplier
	case "forever":
		policyKind = store.KindForever
		duration = 0
	case "custom":
		policyKind = store.KindDuration
		v := req.Value
		if v <= 0 {
			v = 1
		}
		duration = v * unitSeconds(req.Unit) * req.Multiplier
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "类型需为 day/week/month/forever/custom")
		return
	}

	tpl, err := s.st.EnsureQuickPolicy(ctx, app.ID, policyKind, s.now().Unix())
	if err != nil {
		internalErr(w, s.log, "准备模板策略失败", err)
		return
	}

	c := adminFrom(r)
	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID:           app.ID,
		PolicyID:        tpl.ID,
		Qty:             req.Qty,
		Prefix:          req.Prefix,
		KamiLength:      req.Length,
		DurationSeconds: duration,
		DeviceLimit:     req.DeviceLimit,
		CreatedBy:       c.Admin.ID,
		Actor:           fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		IP:              clientIP(r),
		Note:            req.Note,
	})
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{
		"batch_id":   res.BatchID,
		"kamis":      res.Kamis,
		"qty":        len(res.Kamis),
		"kind":       req.Kind,
		"seconds":    duration,
		"per_card":   duration,
		"multiplier": req.Multiplier,
	})
}

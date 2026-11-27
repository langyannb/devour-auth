package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// quickGenerate 发卡页的简化入口：只选时间类型 + 数量。
type quickReq struct {
	Kind        string `json:"kind"` // day|week|month|forever|custom
	Value       int64  `json:"value"`
	Unit        string `json:"unit"` // day|hour（custom 用）
	DeviceLimit int64  `json:"device_limit"`
	Length      int    `json:"length"`
	Prefix      string `json:"prefix"`
	Qty         int    `json:"qty"`
	Note        string `json:"note"`
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

	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}

	// 类型 → 模板策略 + 覆盖时长
	var (
		kind      string
		duration  int64
		policyRaw store.Policy
	)
	switch req.Kind {
	case "day":
		kind, duration = store.KindDuration, 86400
	case "week":
		kind, duration = store.KindDuration, 7*86400
	case "month":
		kind, duration = store.KindDuration, 30*86400
	case "forever":
		kind, duration = store.KindForever, 0
	case "custom":
		kind = store.KindDuration
		v := req.Value
		if v <= 0 {
			v = 1
		}
		if req.Unit == "hour" {
			duration = v * 3600
		} else {
			duration = v * 86400
		}
	case "time":
		// 时间卡：按传进来的 value + unit，默认 1 天
		kind = store.KindDuration
		v := req.Value
		if v <= 0 {
			v = 1
		}
		if req.Unit == "hour" {
			duration = v * 3600
		} else {
			duration = v * 86400
		}
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "类型需为 day/week/month/forever/custom/time")
		return
	}

	policyRaw, err = s.st.EnsureQuickPolicy(ctx, app.ID, kind, s.now().Unix())
	if err != nil {
		internalErr(w, s.log, "准备模板策略失败", err)
		return
	}

	c := adminFrom(r)
	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID:           app.ID,
		PolicyID:        policyRaw.ID,
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
		"batch_id": res.BatchID,
		"kamis":    res.Kamis,
		"qty":      len(res.Kamis),
		"kind":     req.Kind,
		"seconds":  duration,
	})
}

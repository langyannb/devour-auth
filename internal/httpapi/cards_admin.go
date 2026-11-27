package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// patchCard 改单卡设备上限与备注。
func (s *Server) patchCard(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		DeviceLimit  *int64  `json:"device_limit"`
		VerifyDevice *bool   `json:"verify_device"`
		Note         *string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	c := adminFrom(r)
	ch := func(action string) card.StatusChange {
		return card.StatusChange{
			Action: action,
			Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
			IP:     clientIP(r),
		}
	}
	if req.DeviceLimit != nil {
		if err := s.cards.SetDeviceLimit(ctx, id, *req.DeviceLimit, ch("card.set_device_limit")); err != nil {
			s.writeAPIError(w, err)
			return
		}
	}
	if req.VerifyDevice != nil {
		if err := s.cards.SetVerifyDevice(ctx, id, *req.VerifyDevice, ch("card.set_verify_device")); err != nil {
			s.writeAPIError(w, err)
			return
		}
	}
	if req.Note != nil {
		if err := s.cards.SetNote(ctx, id, *req.Note, ch("card.set_note")); err != nil {
			s.writeAPIError(w, err)
			return
		}
	}
	got, err := s.st.CardByID(ctx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
		return
	}
	writeData(w, cardJSON(got))
}

// batchCards 批量操作：封禁 / 解封 / 回收 / 解绑 / 续期。
func (s *Server) batchCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
		Extend *struct {
			Days    int64 `json:"days"`
			Times   int64 `json:"times"`
			Points  int64 `json:"points"`
			Seconds int64 `json:"seconds"`
		} `json:"extend"`
	}
	if !decode(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > 500 {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "一次最多操作 500 张")
		return
	}
	ctx := r.Context()
	c := adminFrom(r)
	ch := func(action string) card.StatusChange {
		return card.StatusChange{
			Action: action,
			Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
			IP:     clientIP(r),
		}
	}

	type failure struct {
		ID    int64  `json:"id"`
		Error string `json:"error"`
	}
	var failed []failure
	okCount := 0

	for _, id := range req.IDs {
		var err error
		switch req.Action {
		case "ban":
			err = s.cards.Ban(ctx, id, ch("card.ban"))
		case "unban":
			err = s.cards.Unban(ctx, id, ch("card.unban"))
		case "recycle":
			err = s.cards.Recycle(ctx, id, ch("card.recycle"))
		case "unbind":
			err = s.cards.Unbind(ctx, id, ch("card.unbind"))
		case "extend":
			if req.Extend == nil {
				err = fmt.Errorf("缺少 extend 参数")
			} else {
				err = s.cards.Extend(ctx, id, card.ExtendAdd{
					Days: req.Extend.Days, Times: req.Extend.Times,
					Points: req.Extend.Points, Seconds: req.Extend.Seconds,
				}, ch("card.extend"))
			}
		default:
			writeErr(w, http.StatusBadRequest, CodeBadInput, "未知动作 "+req.Action)
			return
		}
		if err != nil {
			failed = append(failed, failure{ID: id, Error: err.Error()})
			continue
		}
		okCount++
	}
	writeData(w, map[string]any{"ok_count": okCount, "failed": failed})
}

// exportCards 导出 CSV（带 BOM，Excel 直接打开不乱码）。
func (s *Server) exportCards(w http.ResponseWriter, r *http.Request) {
	filter := store.CardFilter{
		AppID:    queryInt64(r, "app_id"),
		Status:   r.URL.Query().Get("status"),
		PolicyID: queryInt64(r, "policy_id"),
		Kami:     r.URL.Query().Get("kami"),
	}
	// 单页上限 200，分页拉全量，导出不截断
	var cards []store.Card
	for offset := 0; ; offset += 200 {
		filter.Limit, filter.Offset = 200, offset
		rows, total, err := s.st.ListCards(r.Context(), filter)
		if err != nil {
			internalErr(w, s.log, "导出失败", err)
			return
		}
		cards = append(cards, rows...)
		if len(rows) == 0 || int64(offset+len(rows)) >= total {
			break
		}
	}

	ids := make([]int64, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	devices, err := s.st.BoundDeviceCounts(r.Context(), ids)
	if err != nil {
		internalErr(w, s.log, "导出失败", err)
		return
	}
	online, err := s.st.OnlineDeviceCounts(r.Context(), ids, s.now().Unix())
	if err != nil {
		internalErr(w, s.log, "导出失败", err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cards.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "kami", "kind", "status", "duration_seconds", "expire_at", "points", "remain_times", "remaining_seconds", "bound_devices", "online_devices", "device_limit", "rebind_used", "policy_id", "agent_id", "note", "created_at"})
	for _, c := range cards {
		agentID := ""
		if c.AgentID.Valid {
			agentID = strconv.FormatInt(c.AgentID.Int64, 10)
		}
		var snap card.Snapshot
		_ = json.Unmarshal([]byte(c.SnapshotJSON), &snap)
		_ = cw.Write([]string{
			strconv.FormatInt(c.ID, 10), c.Kami, c.Kind, c.Status,
			strconv.FormatInt(snap.TotalSeconds(), 10),
			strconv.FormatInt(c.ExpireAt, 10),
			strconv.FormatInt(c.Points, 10),
			strconv.FormatInt(c.RemainTimes, 10),
			strconv.FormatInt(c.RemainingSeconds, 10),
			strconv.FormatInt(devices[c.ID], 10),
			strconv.FormatInt(online[c.ID], 10),
			strconv.FormatInt(c.DeviceLimit, 10),
			strconv.FormatInt(c.RebindUsed, 10),
			strconv.FormatInt(c.PolicyID, 10),
			agentID,
			strings.ReplaceAll(c.Note, "\n", " "),
			strconv.FormatInt(c.CreatedAt, 10),
		})
	}
	cw.Flush()
}

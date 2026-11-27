package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

// agentSummary 代理端概览：额度、本月出卡、卡数、在线、下级数。
func (s *Server) agentSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := agentFrom(r)
	now := s.now()

	ids, err := s.agents.VisibleAgentIDs(ctx, c.Agent.ID)
	if err != nil {
		internalErr(w, s.log, "查可见范围失败", err)
		return
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Unix()
	used, err := s.st.MonthlyIssuedCents(ctx, c.Agent.ID, monthStart)
	if err != nil {
		internalErr(w, s.log, "查本月出卡失败", err)
		return
	}
	cardCount, _ := s.st.CountCardsByAgents(ctx, ids)
	online, _ := s.st.CountSessionsByAgents(ctx, ids, now.Unix())
	subs, _ := s.st.ListAgents(ctx, c.Agent.ID)

	writeData(w, map[string]any{
		"name":             c.Agent.Name,
		"level":            c.Agent.Level,
		"quota_cents":      c.Agent.QuotaCents,
		"month_used_cents": used,
		"month_remaining":  c.Agent.QuotaCents,
		"cards":            cardCount,
		"online":           online,
		"subagents":        len(subs),
		"server_time":      now.Unix(),
	})
}

// agentCardAction 代理操作自己出的卡：封禁 / 解封 / 解绑。
func (s *Server) agentCardAction(action string, fn func(context.Context, int64, card.StatusChange) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
			return
		}
		ctx := r.Context()
		c, err := s.st.CardByID(ctx, id)
		if err != nil {
			writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
			return
		}
		ag := agentFrom(r).Agent
		if !c.AgentID.Valid || c.AgentID.Int64 != ag.ID {
			writeErr(w, http.StatusForbidden, CodeUnauthorized, "只能操作自己出的卡")
			return
		}
		ch := card.StatusChange{
			Action: "card." + action,
			Actor:  fmt.Sprintf("agent:%d", ag.ID),
			IP:     clientIP(r),
		}
		if err := fn(ctx, id, ch); err != nil {
			switch {
			case errors.Is(err, card.ErrState), errors.Is(err, card.ErrInvalid):
				writeErr(w, http.StatusConflict, CodeBadInput, err.Error())
			default:
				internalErr(w, s.log, "代理操作卡失败", err)
			}
			return
		}
		writeData(w, map[string]any{"id": id, "updated": true})
	}
}

// agentNotices 代理端公告（读管理端发布的 active 公告）。
func (s *Server) agentNotices(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeData(w, []map[string]any{})
		return
	}
	list, err := s.st.ListNotices(r.Context(), app.ID, true)
	if err != nil {
		internalErr(w, s.log, "查公告失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, map[string]any{
			"id": n.ID, "kind": n.Kind, "title": n.Title, "body": n.Body,
			"created_at": n.CreatedAt,
		})
	}
	writeData(w, out)
}

var _ = store.ErrNotFound

package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/store"
)

// ---------- 版本 ----------

func releaseJSON(r store.Release) map[string]any {
	return map[string]any{
		"id": r.ID, "app_id": r.AppID, "version": r.Version, "info": r.Info,
		"url": r.URL, "must": r.Must, "rollout_bp": r.RolloutBP,
		"active": r.Active, "created_at": r.CreatedAt,
	}
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	list, err := s.st.ListReleases(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查版本失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, x := range list {
		out = append(out, releaseJSON(x))
	}
	writeData(w, out)
}

type releaseReq struct {
	Version   string `json:"version"`
	Info      string `json:"info"`
	URL       string `json:"url"`
	Must      bool   `json:"must"`
	RolloutBP int64  `json:"rollout_bp"`
}

func (s *Server) createRelease(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	var req releaseReq
	if !decode(w, r, &req) {
		return
	}
	if req.Version == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "版本号必填")
		return
	}
	rollout := req.RolloutBP
	if rollout <= 0 || rollout > 10000 {
		rollout = 10000
	}
	rel := store.Release{
		AppID: app.ID, Version: req.Version, Info: req.Info, URL: req.URL,
		Must: boolInt(req.Must), RolloutBP: rollout, Active: 1, CreatedAt: s.now().Unix(),
	}
	id, err := s.st.CreateRelease(ctx, rel)
	if err != nil {
		internalErr(w, s.log, "创建版本失败", err)
		return
	}
	rel.ID = id
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "release.create", fmt.Sprintf("release:%d", id), "", jsonOf(releaseJSON(rel)), clientIP(r))
	writeData(w, releaseJSON(rel))
}

func (s *Server) toggleRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.st.SetReleaseActive(r.Context(), id, req.Active); err != nil {
		internalErr(w, s.log, "更新版本失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "release.toggle", fmt.Sprintf("release:%d", id), "", fmt.Sprintf(`{"active":%t}`, req.Active), clientIP(r))
	writeData(w, map[string]any{"id": id, "active": req.Active})
}

// updateRelease 编辑已发布的版本（版本号/说明/地址/灰度/强更）。
func (s *Server) updateRelease(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req releaseReq
	if !decode(w, r, &req) {
		return
	}
	if req.Version == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "版本号必填")
		return
	}
	rollout := req.RolloutBP
	if rollout <= 0 || rollout > 10000 {
		rollout = 10000
	}
	hit, err := s.st.UpdateRelease(r.Context(), app.ID, id, store.Release{
		Version: req.Version, Info: req.Info, URL: req.URL,
		Must: boolInt(req.Must), RolloutBP: rollout,
	})
	if err != nil {
		internalErr(w, s.log, "修改版本失败", err)
		return
	}
	if !hit {
		writeErr(w, http.StatusNotFound, CodeBadInput, "版本不存在")
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "release.update", fmt.Sprintf("release:%d", id), "", jsonOf(map[string]any{
		"version": req.Version,
	}), clientIP(r))
	writeData(w, map[string]any{"id": id, "version": req.Version})
}

// deleteRelease 删除版本记录。
func (s *Server) deleteRelease(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	hit, err := s.st.DeleteRelease(r.Context(), app.ID, id)
	if err != nil {
		internalErr(w, s.log, "删除版本失败", err)
		return
	}
	if !hit {
		writeErr(w, http.StatusNotFound, CodeBadInput, "版本不存在")
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "release.delete", fmt.Sprintf("release:%d", id), "", "", clientIP(r))
	writeData(w, map[string]any{"id": id, "deleted": true})
}

// ---------- 公告 ----------

func (s *Server) listNoticesAdmin(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	list, err := s.st.ListNotices(r.Context(), app.ID, false)
	if err != nil {
		internalErr(w, s.log, "查公告失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, map[string]any{
			"id": n.ID, "kind": n.Kind, "title": n.Title, "body": n.Body,
			"active": n.Active, "created_at": n.CreatedAt,
		})
	}
	writeData(w, out)
}

type noticeReq struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (s *Server) createNotice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	var req noticeReq
	if !decode(w, r, &req) {
		return
	}
	if req.Body == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "正文必填")
		return
	}
	switch req.Kind {
	case "list", "popup", "immediate":
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "kind 需为 list/popup/immediate")
		return
	}
	n := store.Notice{
		AppID: app.ID, Kind: req.Kind, Title: req.Title, Body: req.Body,
		Active: 1, CreatedAt: s.now().Unix(),
	}
	id, err := s.st.CreateNotice(ctx, n)
	if err != nil {
		internalErr(w, s.log, "创建公告失败", err)
		return
	}
	n.ID = id
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "notice.create", fmt.Sprintf("notice:%d", id), "", jsonOf(map[string]any{
		"kind": n.Kind, "title": n.Title,
	}), clientIP(r))
	writeData(w, map[string]any{"id": id, "kind": n.Kind, "title": n.Title, "body": n.Body})
}

func (s *Server) toggleNotice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.st.SetNoticeActive(r.Context(), id, req.Active); err != nil {
		internalErr(w, s.log, "更新公告失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "notice.toggle", fmt.Sprintf("notice:%d", id), "", fmt.Sprintf(`{"active":%t}`, req.Active), clientIP(r))
	writeData(w, map[string]any{"id": id, "active": req.Active})
}

// updateNotice 编辑公告（类型/标题/正文）。
func (s *Server) updateNotice(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req noticeReq
	if !decode(w, r, &req) {
		return
	}
	if req.Body == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "正文必填")
		return
	}
	switch req.Kind {
	case "list", "popup", "immediate":
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "kind 需为 list/popup/immediate")
		return
	}
	hit, err := s.st.UpdateNotice(r.Context(), app.ID, id, store.Notice{
		Kind: req.Kind, Title: req.Title, Body: req.Body,
	})
	if err != nil {
		internalErr(w, s.log, "修改公告失败", err)
		return
	}
	if !hit {
		writeErr(w, http.StatusNotFound, CodeBadInput, "公告不存在")
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "notice.update", fmt.Sprintf("notice:%d", id), "", jsonOf(map[string]any{
		"kind": req.Kind, "title": req.Title,
	}), clientIP(r))
	writeData(w, map[string]any{"id": id, "kind": req.Kind, "title": req.Title})
}

// deleteNotice 删除公告。
func (s *Server) deleteNotice(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	hit, err := s.st.DeleteNotice(r.Context(), app.ID, id)
	if err != nil {
		internalErr(w, s.log, "删除公告失败", err)
		return
	}
	if !hit {
		writeErr(w, http.StatusNotFound, CodeBadInput, "公告不存在")
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(r.Context(), c, "notice.delete", fmt.Sprintf("notice:%d", id), "", "", clientIP(r))
	writeData(w, map[string]any{"id": id, "deleted": true})
}

// ---------- 远程变量 ----------

func (s *Server) listFlagsAdmin(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	flags, err := s.st.ListFlags(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查变量失败", err)
		return
	}
	writeData(w, flags)
}

func (s *Server) setFlag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Key == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "key 必填")
		return
	}
	if err := s.st.SetFlag(ctx, app.ID, req.Key, req.Value, s.now().Unix()); err != nil {
		internalErr(w, s.log, "写变量失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "flag.set", "flag:"+req.Key, "", jsonOf(map[string]string{"value": req.Value}), clientIP(r))
	writeData(w, map[string]any{"key": req.Key, "value": req.Value})
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

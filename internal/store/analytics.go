package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Bucket 一维聚合结果（卡类型、状态等）。
type Bucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// AgentBucket 代理维度排行。
type AgentBucket struct {
	AgentID int64  `json:"agent_id"`
	Name    string `json:"name"`
	Count   int64  `json:"count"`
}

// DayPoint 代理可见范围内某一天的出卡与激活。
type DayPoint struct {
	Day       string `json:"day"`
	Issued    int64  `json:"issued"`
	Activated int64  `json:"activated"`
}

// CardsByKind 全应用按卡类型汇总（appID=0 表示所有应用）。
func (s *Store) CardsByKind(ctx context.Context, appID int64) ([]Bucket, error) {
	return s.buckets(ctx,
		`SELECT kind, COUNT(*) FROM cards WHERE (? = 0 OR app_id = ?) GROUP BY kind ORDER BY 2 DESC`,
		appID, appID)
}

// TopAgentsByCards 出卡最多的代理（平台直发没有 agent_id，不计入）。
func (s *Store) TopAgentsByCards(ctx context.Context, appID int64, limit int) ([]AgentBucket, error) {
	if limit <= 0 || limit > 20 {
		limit = 6
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.name, COUNT(c.id) FROM agents a
		   JOIN cards c ON c.agent_id = a.id
		  WHERE (? = 0 OR a.app_id = ?)
		  GROUP BY a.id, a.name ORDER BY 3 DESC, a.id LIMIT ?`,
		appID, appID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentBucket{}
	for rows.Next() {
		var b AgentBucket
		if err := rows.Scan(&b.AgentID, &b.Name, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AgentCardsBy 代理可见范围内按维度汇总，dim 只能是 kind 或 status。
func (s *Store) AgentCardsBy(ctx context.Context, ids []int64, dim string) ([]Bucket, error) {
	if dim != "kind" && dim != "status" {
		return nil, fmt.Errorf("不支持的统计维度 %q", dim)
	}
	in, args := idList(ids)
	if in == "" {
		return []Bucket{}, nil
	}
	return s.buckets(ctx,
		`SELECT `+dim+`, COUNT(*) FROM cards WHERE agent_id IN (`+in+`) GROUP BY `+dim+` ORDER BY 2 DESC`,
		args...)
}

// AgentDailyCards 代理可见范围内按本地日期统计出卡与激活。
func (s *Store) AgentDailyCards(ctx context.Context, ids []int64, since int64) ([]DayPoint, error) {
	in, args := idList(ids)
	if in == "" {
		return nil, nil
	}
	byDay := map[string]*DayPoint{}
	read := func(q string, assign func(*DayPoint, int64)) error {
		rows, err := s.db.QueryContext(ctx, q, append(append([]any{}, args...), since)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var day string
			var n int64
			if err := rows.Scan(&day, &n); err != nil {
				return err
			}
			p := byDay[day]
			if p == nil {
				p = &DayPoint{Day: day}
				byDay[day] = p
			}
			assign(p, n)
		}
		return rows.Err()
	}
	if err := read(`SELECT date(created_at, 'unixepoch', 'localtime') d, COUNT(*)
	                  FROM cards WHERE agent_id IN (`+in+`) AND created_at >= ? GROUP BY d`,
		func(p *DayPoint, n int64) { p.Issued = n }); err != nil {
		return nil, err
	}
	if err := read(`SELECT date(activated_at, 'unixepoch', 'localtime') d, COUNT(*)
	                  FROM cards WHERE agent_id IN (`+in+`) AND activated_at >= ? GROUP BY d`,
		func(p *DayPoint, n int64) { p.Activated = n }); err != nil {
		return nil, err
	}
	out := make([]DayPoint, 0, len(byDay))
	for _, p := range byDay {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

// ActiveDevices 区间内活跃过的设备数（按最后活跃时间）。
func (s *Store) ActiveDevices(ctx context.Context, appID, since int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices d JOIN cards c ON c.id = d.card_id
		  WHERE d.unbound_at IS NULL AND d.last_seen >= ? AND (? = 0 OR c.app_id = ?)`,
		since, appID, appID).Scan(&n)
	return n, err
}

// AgentActiveDevices 代理可见范围内区间活跃设备数。
func (s *Store) AgentActiveDevices(ctx context.Context, ids []int64, since int64) (int64, error) {
	in, args := idList(ids)
	if in == "" {
		return 0, nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices d JOIN cards c ON c.id = d.card_id
		  WHERE d.unbound_at IS NULL AND d.last_seen >= ? AND c.agent_id IN (`+in+`)`,
		append([]any{since}, args...)...).Scan(&n)
	return n, err
}

func (s *Store) buckets(ctx context.Context, q string, args ...any) ([]Bucket, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bucket{}
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// idList 把 id 切片变成 IN (?,?,...) 与参数。
func idList(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "", nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	return strings.Join(ph, ","), args
}

package store

import (
	"context"
	"database/sql"
	"errors"
)

// AgentPlan 价格模板：把「卡类型 → 价格」做成可复用的表。
type AgentPlan struct {
	ID        int64
	AppID     int64
	Name      string
	Active    int64
	CreatedAt int64
	Items     map[string]int64 // kind -> price_cents
}

func (s *Store) ListAgentPlans(ctx context.Context, appID int64) ([]AgentPlan, error) {
	q := `SELECT id, app_id, name, active, created_at FROM agent_plans`
	var args []any
	if appID > 0 {
		q += ` WHERE app_id = ?`
		args = append(args, appID)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentPlan
	for rows.Next() {
		var p AgentPlan
		if err := rows.Scan(&p.ID, &p.AppID, &p.Name, &p.Active, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		items, err := s.planItems(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Items = items
	}
	return out, nil
}

func (s *Store) planItems(ctx context.Context, planID int64) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, price_cents FROM agent_plan_items WHERE plan_id = ?`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) AgentPlanByID(ctx context.Context, id int64) (AgentPlan, error) {
	var p AgentPlan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, name, active, created_at FROM agent_plans WHERE id = ?`, id).
		Scan(&p.ID, &p.AppID, &p.Name, &p.Active, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentPlan{}, ErrNotFound
	}
	if err != nil {
		return AgentPlan{}, err
	}
	p.Items, err = s.planItems(ctx, p.ID)
	return p, err
}

// SaveAgentPlan 创建或覆盖模板（id=0 新建）。
func (s *Store) SaveAgentPlan(ctx context.Context, p AgentPlan, now int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if p.ID == 0 {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO agent_plans(app_id, name, active, created_at) VALUES(?,?,?,?)`,
			p.AppID, p.Name, 1, now)
		if err != nil {
			if isUniqueViolation(err) {
				return 0, ErrConflict
			}
			return 0, err
		}
		p.ID, err = res.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE agent_plans SET name = ?, active = ? WHERE id = ?`, p.Name, p.Active, p.ID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_plan_items WHERE plan_id = ?`, p.ID); err != nil {
		return 0, err
	}
	for kind, price := range p.Items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO agent_plan_items(plan_id, kind, price_cents) VALUES(?,?,?)`,
			p.ID, kind, price); err != nil {
			return 0, err
		}
	}
	return p.ID, tx.Commit()
}

// AgentPermissions 代理能做什么。
type AgentPermissions struct {
	SetDeviceLimit bool
	CustomKami     bool
	MaxQty         int64
}

func (s *Store) AgentPermissions(ctx context.Context, agentID int64) (AgentPermissions, int64, error) {
	var (
		planID sql.NullInt64
		setDev int
		custom int
		maxQty int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT plan_id, perm_set_device_limit, perm_custom_kami, perm_max_qty
		   FROM agents WHERE id = ?`, agentID).Scan(&planID, &setDev, &custom, &maxQty)
	if err != nil {
		return AgentPermissions{}, 0, err
	}
	return AgentPermissions{SetDeviceLimit: setDev == 1, CustomKami: custom == 1, MaxQty: maxQty}, planID.Int64, nil
}

func (s *Store) SetAgentPlanAndPerms(ctx context.Context, agentID, planID int64, perms AgentPermissions) error {
	var pid any
	if planID > 0 {
		pid = planID
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET plan_id = ?, perm_set_device_limit = ?, perm_custom_kami = ?, perm_max_qty = ?
		  WHERE id = ?`,
		pid, boolInt64(perms.SetDeviceLimit), boolInt64(perms.CustomKami), perms.MaxQty, agentID)
	return err
}

func boolInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

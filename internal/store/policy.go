package store

import (
	"context"
	"database/sql"
	"errors"
)

// 卡类型 kind 取值（design.md 第 7 节）。
const (
	KindDuration = "duration"
	KindForever  = "forever"
	KindCount    = "count"
	KindCredit   = "credit"
	KindTimed    = "timed"
	KindTrial    = "trial"
)

type Policy struct {
	ID                 int64
	AppID              int64
	Code               string
	Name               string
	Kind               string
	DurationDays       int64
	DurationHours      int64
	DurationSeconds    int64
	InitialTimes       int64
	InitialPoints      int64
	DeviceLimit        int64
	ConcurrentLimit    int64
	HeartbeatSec       int64
	OfflineHours       int64
	OfflineStarts      int64
	RebindFree         int64
	RebindCooldownDays int64
	RebindCostHours    int64
	Renewable          int64
	TrialPerDevice     int64
	PriceCents         int64
	Active             int64
	CreatedAt          int64
}

const policyCols = `id, app_id, code, name, kind,
  duration_days, duration_hours, duration_seconds,
  initial_times, initial_points,
  device_limit, concurrent_limit, heartbeat_sec,
  offline_hours, offline_starts,
  rebind_free, rebind_cooldown_days, rebind_cost_hours,
  renewable, trial_per_device, price_cents, active, created_at`

func scanPolicy(sc interface{ Scan(...any) error }) (Policy, error) {
	var p Policy
	err := sc.Scan(
		&p.ID, &p.AppID, &p.Code, &p.Name, &p.Kind,
		&p.DurationDays, &p.DurationHours, &p.DurationSeconds,
		&p.InitialTimes, &p.InitialPoints,
		&p.DeviceLimit, &p.ConcurrentLimit, &p.HeartbeatSec,
		&p.OfflineHours, &p.OfflineStarts,
		&p.RebindFree, &p.RebindCooldownDays, &p.RebindCostHours,
		&p.Renewable, &p.TrialPerDevice, &p.PriceCents, &p.Active, &p.CreatedAt,
	)
	return p, err
}

func (s *Store) CreatePolicy(ctx context.Context, p Policy) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO policies(app_id, code, name, kind,
		   duration_days, duration_hours, duration_seconds,
		   initial_times, initial_points,
		   device_limit, concurrent_limit, heartbeat_sec,
		   offline_hours, offline_starts,
		   rebind_free, rebind_cooldown_days, rebind_cost_hours,
		   renewable, trial_per_device, price_cents, active, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.AppID, p.Code, p.Name, p.Kind,
		p.DurationDays, p.DurationHours, p.DurationSeconds,
		p.InitialTimes, p.InitialPoints,
		p.DeviceLimit, p.ConcurrentLimit, p.HeartbeatSec,
		p.OfflineHours, p.OfflineStarts,
		p.RebindFree, p.RebindCooldownDays, p.RebindCostHours,
		p.Renewable, p.TrialPerDevice, p.PriceCents, p.Active, p.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) PolicyByID(ctx context.Context, id int64) (Policy, error) {
	p, err := scanPolicy(s.db.QueryRowContext(ctx,
		`SELECT `+policyCols+` FROM policies WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	return p, err
}

func (s *Store) PolicyByCode(ctx context.Context, appID int64, code string) (Policy, error) {
	p, err := scanPolicy(s.db.QueryRowContext(ctx,
		`SELECT `+policyCols+` FROM policies WHERE app_id = ? AND code = ?`, appID, code))
	if errors.Is(err, sql.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ListPolicies(ctx context.Context, appID int64) ([]Policy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+policyCols+` FROM policies WHERE app_id = ? ORDER BY id`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListPolicyEntitlementCodes(ctx context.Context, policyID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT e.code FROM policy_entitlements pe
		   JOIN entitlements e ON e.id = pe.entitlement_id
		  WHERE pe.policy_id = ? AND e.active = 1
		  ORDER BY e.code`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

// UpdatePolicy 只改策略定义，已生成的卡按各自 snapshot 执行（design.md 7.1）。
func (s *Store) UpdatePolicy(ctx context.Context, p Policy) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE policies SET name=?, kind=?,
		   duration_days=?, duration_hours=?, duration_seconds=?,
		   initial_times=?, initial_points=?,
		   device_limit=?, concurrent_limit=?, heartbeat_sec=?,
		   offline_hours=?, offline_starts=?,
		   rebind_free=?, rebind_cooldown_days=?, rebind_cost_hours=?,
		   renewable=?, trial_per_device=?, price_cents=?, active=?
		 WHERE id = ?`,
		p.Name, p.Kind,
		p.DurationDays, p.DurationHours, p.DurationSeconds,
		p.InitialTimes, p.InitialPoints,
		p.DeviceLimit, p.ConcurrentLimit, p.HeartbeatSec,
		p.OfflineHours, p.OfflineStarts,
		p.RebindFree, p.RebindCooldownDays, p.RebindCostHours,
		p.Renewable, p.TrialPerDevice, p.PriceCents, p.Active, p.ID)
	return err
}

// EnsureQuickPolicy 取或创建「快捷发卡」用的模板策略（code 以 _quick 开头）。
func (s *Store) EnsureQuickPolicy(ctx context.Context, appID int64, kind string, now int64) (Policy, error) {
	code := "_quick_" + kind
	p, err := s.PolicyByCode(ctx, appID, code)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Policy{}, err
	}
	tpl := Policy{
		AppID: appID, Code: code, Name: "快捷-" + kind, Kind: kind,
		DeviceLimit: 1, ConcurrentLimit: 1, HeartbeatSec: 600,
		OfflineHours: 24, OfflineStarts: 20, RebindFree: 1,
		RebindCooldownDays: 30, Renewable: 1, TrialPerDevice: 1,
		Active: 1, CreatedAt: now,
	}
	id, err := s.CreatePolicy(ctx, tpl)
	if err != nil {
		return Policy{}, err
	}
	return s.PolicyByID(ctx, id)
}

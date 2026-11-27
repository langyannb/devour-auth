-- 008: 代理价格模板 + 代理权限
CREATE TABLE agent_plans (
  id         INTEGER PRIMARY KEY,
  app_id     INTEGER NOT NULL,
  name       TEXT NOT NULL,
  active     INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  UNIQUE(app_id, name)
);

-- 模板里每个卡类型（day/week/month/forever/custom）一个价格
CREATE TABLE agent_plan_items (
  plan_id     INTEGER NOT NULL REFERENCES agent_plans(id),
  kind        TEXT NOT NULL,
  price_cents INTEGER NOT NULL,
  PRIMARY KEY (plan_id, kind)
);

-- 代理选模板 + 权限
ALTER TABLE agents ADD COLUMN plan_id INTEGER;
ALTER TABLE agents ADD COLUMN perm_set_device_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN perm_custom_kami INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agents ADD COLUMN perm_max_qty INTEGER NOT NULL DEFAULT 10;

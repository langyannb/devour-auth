-- 001_init: docs/design.md 第 6 节的表结构

-- 应用
CREATE TABLE apps (
  id            INTEGER PRIMARY KEY,
  code          TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  active        INTEGER NOT NULL DEFAULT 1,
  created_at    INTEGER NOT NULL
);

-- 管理员
CREATE TABLE admins (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  pass_hash     TEXT NOT NULL,
  role          TEXT NOT NULL,
  totp_secret   TEXT,
  ip_allow      TEXT,
  locked_until  INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);

CREATE TABLE admin_sessions (
  id            INTEGER PRIMARY KEY,
  admin_id      INTEGER NOT NULL REFERENCES admins(id),
  token_hash    TEXT NOT NULL UNIQUE,
  csrf_token    TEXT NOT NULL,
  ip            TEXT,
  ua            TEXT,
  created_at    INTEGER NOT NULL,
  last_seen     INTEGER NOT NULL,
  expires_at    INTEGER NOT NULL,
  reauth_until  INTEGER NOT NULL DEFAULT 0,
  revoked_at    INTEGER
);

-- 代理
CREATE TABLE agents (
  id                   INTEGER PRIMARY KEY,
  name                 TEXT NOT NULL UNIQUE,
  parent_id            INTEGER REFERENCES agents(id),
  level                INTEGER NOT NULL,
  discount_bp          INTEGER NOT NULL,
  rebate_bp            INTEGER NOT NULL DEFAULT 0,
  monthly_quota_cents  INTEGER NOT NULL DEFAULT 0,
  pass_hash            TEXT NOT NULL,
  totp_secret          TEXT,
  status               TEXT NOT NULL,
  created_at           INTEGER NOT NULL,
  frozen_at            INTEGER
);

CREATE TABLE agent_ledger (
  id            INTEGER PRIMARY KEY,
  agent_id      INTEGER NOT NULL REFERENCES agents(id),
  kind          TEXT NOT NULL,
  amount_cents  INTEGER NOT NULL,
  ref           TEXT,
  created_at    INTEGER NOT NULL
);

CREATE TABLE agent_bills (
  id            INTEGER PRIMARY KEY,
  agent_id      INTEGER NOT NULL REFERENCES agents(id),
  month         TEXT NOT NULL,
  amount_cents  INTEGER NOT NULL,
  status        TEXT NOT NULL,
  settled_at    INTEGER,
  note          TEXT,
  UNIQUE(agent_id, month)
);

-- 卡类型
CREATE TABLE policies (
  id                    INTEGER PRIMARY KEY,
  app_id                INTEGER NOT NULL REFERENCES apps(id),
  code                  TEXT NOT NULL,
  name                  TEXT NOT NULL,
  kind                  TEXT NOT NULL,
  duration_days         INTEGER NOT NULL DEFAULT 0,
  duration_hours        INTEGER NOT NULL DEFAULT 0,
  duration_seconds      INTEGER NOT NULL DEFAULT 0,
  initial_times         INTEGER NOT NULL DEFAULT 0,
  initial_points        INTEGER NOT NULL DEFAULT 0,
  device_limit          INTEGER NOT NULL DEFAULT 1,
  concurrent_limit      INTEGER NOT NULL DEFAULT 1,
  heartbeat_sec         INTEGER NOT NULL DEFAULT 600,
  offline_hours         INTEGER NOT NULL DEFAULT 24,
  offline_starts        INTEGER NOT NULL DEFAULT 20,
  rebind_free           INTEGER NOT NULL DEFAULT 1,
  rebind_cooldown_days  INTEGER NOT NULL DEFAULT 30,
  rebind_cost_hours     INTEGER NOT NULL DEFAULT 0,
  renewable             INTEGER NOT NULL DEFAULT 1,
  trial_per_device      INTEGER NOT NULL DEFAULT 1,
  price_cents           INTEGER NOT NULL DEFAULT 0,
  active                INTEGER NOT NULL DEFAULT 1,
  created_at            INTEGER NOT NULL,
  UNIQUE(app_id, code)
);

-- 功能权限
CREATE TABLE entitlements (
  id           INTEGER PRIMARY KEY,
  app_id       INTEGER NOT NULL REFERENCES apps(id),
  code         TEXT NOT NULL,
  name         TEXT NOT NULL,
  cost_points  INTEGER NOT NULL DEFAULT 0,
  cost_times   INTEGER NOT NULL DEFAULT 0,
  active       INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL,
  UNIQUE(app_id, code)
);

CREATE TABLE policy_entitlements (
  policy_id       INTEGER NOT NULL REFERENCES policies(id),
  entitlement_id  INTEGER NOT NULL REFERENCES entitlements(id),
  PRIMARY KEY (policy_id, entitlement_id)
);

-- 发卡批次（cards 引用它，先建）
CREATE TABLE batches (
  id          INTEGER PRIMARY KEY,
  app_id      INTEGER NOT NULL,
  policy_id   INTEGER NOT NULL,
  agent_id    INTEGER,
  qty         INTEGER NOT NULL,
  created_by  INTEGER NOT NULL,
  created_at  INTEGER NOT NULL
);

-- 卡密
CREATE TABLE cards (
  id                INTEGER PRIMARY KEY,
  app_id            INTEGER NOT NULL REFERENCES apps(id),
  kami              TEXT NOT NULL UNIQUE,
  policy_id         INTEGER NOT NULL REFERENCES policies(id),
  kind              TEXT NOT NULL,
  status            TEXT NOT NULL,
  batch_id          INTEGER REFERENCES batches(id),
  agent_id          INTEGER REFERENCES agents(id),
  device_limit      INTEGER NOT NULL,
  expire_at         INTEGER NOT NULL DEFAULT 0,
  remain_times      INTEGER NOT NULL DEFAULT 0,
  points            INTEGER NOT NULL DEFAULT 0,
  remaining_seconds INTEGER NOT NULL DEFAULT 0,
  paused            INTEGER NOT NULL DEFAULT 0,
  rebind_used       INTEGER NOT NULL DEFAULT 0,
  snapshot_json     TEXT NOT NULL,
  note              TEXT,
  activated_at      INTEGER,
  banned_at         INTEGER,
  created_at        INTEGER NOT NULL
);

CREATE TABLE card_entitlements (
  card_id         INTEGER NOT NULL REFERENCES cards(id),
  entitlement_id  INTEGER NOT NULL REFERENCES entitlements(id),
  expires_at      INTEGER NOT NULL DEFAULT 0,
  granted_by      INTEGER NOT NULL,
  revoked_at      INTEGER,
  PRIMARY KEY (card_id, entitlement_id)
);

-- 设备与会话
CREATE TABLE devices (
  id           INTEGER PRIMARY KEY,
  card_id      INTEGER NOT NULL REFERENCES cards(id),
  fingerprint  TEXT NOT NULL,
  model        TEXT,
  brand        TEXT,
  os           TEXT,
  app_version  TEXT,
  first_seen   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  banned       INTEGER NOT NULL DEFAULT 0,
  unbound_at   INTEGER,
  UNIQUE(card_id, fingerprint)
);

CREATE TABLE sessions (
  id              INTEGER PRIMARY KEY,
  card_id         INTEGER NOT NULL REFERENCES cards(id),
  device_id       INTEGER NOT NULL REFERENCES devices(id),
  access_hash     TEXT NOT NULL UNIQUE,
  refresh_hash    TEXT NOT NULL UNIQUE,
  access_exp      INTEGER NOT NULL,
  refresh_exp     INTEGER NOT NULL,
  last_heartbeat  INTEGER NOT NULL,
  revoked_at      INTEGER,
  reuse_detected  INTEGER NOT NULL DEFAULT 0,
  ip              TEXT,
  ua              TEXT,
  created_at      INTEGER NOT NULL
);

CREATE TABLE entitlement_usage (
  id             INTEGER PRIMARY KEY,
  card_id        INTEGER NOT NULL REFERENCES cards(id),
  code           TEXT NOT NULL,
  cost_points    INTEGER NOT NULL DEFAULT 0,
  cost_times     INTEGER NOT NULL DEFAULT 0,
  balance_after  INTEGER NOT NULL,
  nonce          TEXT NOT NULL UNIQUE,
  created_at     INTEGER NOT NULL
);

CREATE TABLE trial_claims (
  app_id       INTEGER NOT NULL,
  fingerprint  TEXT NOT NULL,
  card_id      INTEGER NOT NULL,
  claimed_at   INTEGER NOT NULL,
  PRIMARY KEY (app_id, fingerprint)
);

-- 黑名单
CREATE TABLE blacklist (
  id          INTEGER PRIMARY KEY,
  app_id      INTEGER NOT NULL DEFAULT 0,
  kind        TEXT NOT NULL,
  value       TEXT NOT NULL,
  reason      TEXT,
  expires_at  INTEGER NOT NULL DEFAULT 0,
  created_by  INTEGER NOT NULL,
  created_at  INTEGER NOT NULL,
  UNIQUE(app_id, kind, value)
);

-- 运营
CREATE TABLE releases (
  id          INTEGER PRIMARY KEY,
  app_id      INTEGER NOT NULL,
  version     TEXT NOT NULL,
  info        TEXT,
  url         TEXT,
  must        INTEGER NOT NULL DEFAULT 0,
  rollout_bp  INTEGER NOT NULL DEFAULT 10000,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);

CREATE TABLE notices (
  id          INTEGER PRIMARY KEY,
  app_id      INTEGER NOT NULL,
  kind        TEXT NOT NULL,
  title       TEXT,
  body        TEXT NOT NULL,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);

CREATE TABLE flags (
  app_id      INTEGER NOT NULL,
  key         TEXT NOT NULL,
  value       TEXT NOT NULL,
  updated_at  INTEGER NOT NULL,
  PRIMARY KEY (app_id, key)
);

-- 开放对接
CREATE TABLE api_keys (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL,
  scopes       TEXT NOT NULL,
  key_id       TEXT NOT NULL UNIQUE,
  secret_hash  TEXT NOT NULL,
  last_used    INTEGER,
  status       TEXT NOT NULL,
  created_at   INTEGER NOT NULL
);

-- 审计
CREATE TABLE events (
  id         INTEGER PRIMARY KEY,
  ts         INTEGER NOT NULL,
  kind       TEXT NOT NULL,
  card_id    INTEGER,
  device_id  INTEGER,
  agent_id   INTEGER,
  ip         TEXT,
  ua         TEXT,
  code       TEXT,
  detail     TEXT
);

CREATE TABLE audit_log (
  id         INTEGER PRIMARY KEY,
  ts         INTEGER NOT NULL,
  actor      TEXT NOT NULL,
  action     TEXT NOT NULL,
  target     TEXT,
  before     TEXT,
  after      TEXT,
  ip         TEXT,
  prev_hash  TEXT NOT NULL,
  hash       TEXT NOT NULL
);

CREATE TABLE audit_anchor (
  day         TEXT PRIMARY KEY,
  last_id     INTEGER NOT NULL,
  last_hash   TEXT NOT NULL,
  signature   TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);

CREATE TABLE daily_stats (
  day            TEXT PRIMARY KEY,
  activations    INTEGER NOT NULL DEFAULT 0,
  logins         INTEGER NOT NULL DEFAULT 0,
  unique_devices INTEGER NOT NULL DEFAULT 0,
  errors         INTEGER NOT NULL DEFAULT 0,
  online_peak    INTEGER NOT NULL DEFAULT 0,
  issued_cents   INTEGER NOT NULL DEFAULT 0
);

-- 审计链不可改写
CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit_log
BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;

CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit_log
BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;

-- 列表查询用到的索引
CREATE INDEX idx_cards_app_status ON cards(app_id, status);
CREATE INDEX idx_cards_agent ON cards(agent_id);
CREATE INDEX idx_cards_batch ON cards(batch_id);
CREATE INDEX idx_devices_card ON devices(card_id);
CREATE INDEX idx_sessions_card ON sessions(card_id);
CREATE INDEX idx_events_ts ON events(ts);
CREATE INDEX idx_audit_ts ON audit_log(ts);
CREATE INDEX idx_ledger_agent ON agent_ledger(agent_id, created_at);

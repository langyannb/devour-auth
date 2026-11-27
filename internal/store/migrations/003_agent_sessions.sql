-- 003: 代理独立会话域（design.md 第 11 节，与 admin_sessions 分开）
CREATE TABLE agent_sessions (
  id          INTEGER PRIMARY KEY,
  agent_id    INTEGER NOT NULL REFERENCES agents(id),
  token_hash  TEXT NOT NULL UNIQUE,
  csrf_token  TEXT NOT NULL,
  ip          TEXT,
  ua          TEXT,
  created_at  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,
  revoked_at  INTEGER
);

CREATE INDEX idx_agent_sessions_agent ON agent_sessions(agent_id);

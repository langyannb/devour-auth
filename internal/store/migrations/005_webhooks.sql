-- 005: 事件回调（Webhook）
CREATE TABLE webhooks (
  id         INTEGER PRIMARY KEY,
  app_id     INTEGER NOT NULL,
  url        TEXT NOT NULL,
  secret     TEXT NOT NULL,
  events     TEXT NOT NULL,   -- 逗号分隔：card.ban, card.recycle, card.activate
  active     INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_webhooks_app ON webhooks(app_id, active);

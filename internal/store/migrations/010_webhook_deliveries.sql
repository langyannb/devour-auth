-- 010: Webhook 投递记录
-- webhook 可能被删除，这里只存 URL 快照，不做外键级联删除。
CREATE TABLE webhook_deliveries (
  id          INTEGER PRIMARY KEY,
  webhook_id  INTEGER NOT NULL,
  event       TEXT NOT NULL,
  url         TEXT NOT NULL,
  attempts    INTEGER NOT NULL,
  ok          INTEGER NOT NULL,
  status_code INTEGER NOT NULL DEFAULT 0,  -- 0 = 没有拿到响应
  error       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  finished_at INTEGER NOT NULL
);

CREATE INDEX idx_webhook_deliveries_id ON webhook_deliveries(id DESC);

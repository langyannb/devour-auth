-- 002: refresh 轮换历史，用于识别旧 refresh 被盗用（design.md 12 / 16.4）
CREATE TABLE refresh_history (
  hash        TEXT PRIMARY KEY,
  session_id  INTEGER NOT NULL,
  card_id     INTEGER NOT NULL,
  rotated_at  INTEGER NOT NULL
);

CREATE INDEX idx_refresh_history_card ON refresh_history(card_id);

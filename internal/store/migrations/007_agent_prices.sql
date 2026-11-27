-- 007: 代理级卡类型价格表（管理员直接定价，不走折扣）
CREATE TABLE agent_prices (
  agent_id    INTEGER NOT NULL REFERENCES agents(id),
  policy_id   INTEGER NOT NULL REFERENCES policies(id),
  price_cents INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  PRIMARY KEY (agent_id, policy_id)
);

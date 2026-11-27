-- 006: 管理员一次性恢复码（TOTP 丢失时登录）
CREATE TABLE admin_recovery_codes (
  id         INTEGER PRIMARY KEY,
  admin_id   INTEGER NOT NULL REFERENCES admins(id),
  code_hash  TEXT NOT NULL UNIQUE,
  used_at    INTEGER
);

CREATE INDEX idx_recovery_admin ON admin_recovery_codes(admin_id);

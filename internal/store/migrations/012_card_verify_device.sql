-- 单卡开关：0 = 这张卡不校验设备数量（设备仍会绑定/记录）
ALTER TABLE cards ADD COLUMN verify_device INTEGER NOT NULL DEFAULT 1;

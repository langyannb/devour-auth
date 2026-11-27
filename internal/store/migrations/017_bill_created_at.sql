-- 账单记录生成时间；状态页的「上次账单」用它，不再按账期推算
ALTER TABLE agent_bills ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;
UPDATE agent_bills
   SET created_at = COALESCE(settled_at, CAST(strftime('%s', month || '-01') AS INTEGER))
 WHERE created_at = 0;

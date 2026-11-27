#!/bin/bash
# 每小时的 SQLite 在线备份 + GPG 加密，保留 14 天（runbook 5.1）。
# 安装位置：/opt/devour-auth/backup.sh
set -euo pipefail

DB=${DEVOUR_DB:-/var/lib/devour/app.db}
DIR=${BACKUP_DIR:-/var/backups/devour}
KEEP_DAYS=${BACKUP_KEEP_DAYS:-14}
RECIPIENT=${BACKUP_GPG_RECIPIENT:-devour-backup}
export GNUPGHOME=${GNUPGHOME:-/root/.gnupg-backup}

mkdir -p "$DIR"
TS=$(date +%Y%m%d%H)
TMP="$DIR/app-$TS.db"

# 走 SQLite 在线备份 API，避免拷贝 WAL 中间态
python3 - "$DB" "$TMP" <<'PY'
import sqlite3, sys
src, dst = sys.argv[1], sys.argv[2]
s = sqlite3.connect(src, timeout=10)
d = sqlite3.connect(dst)
with d:
    s.backup(d)
d.close()
s.close()
PY

gpg --batch --yes --trust-model always --encrypt --recipient "$RECIPIENT" \
    --output "$TMP.gpg" "$TMP"
rm -f "$TMP"
find "$DIR" -name '*.gpg' -mtime "+$KEEP_DAYS" -delete

echo "备份完成：$TMP.gpg（$(stat -c%s "$TMP.gpg") 字节），当前保留 $(find "$DIR" -name '*.gpg' | wc -l) 份"

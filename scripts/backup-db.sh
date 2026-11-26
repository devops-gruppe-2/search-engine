#!/usr/bin/env bash
# Backup af PostgreSQL-databasen på serveren.
# Ligger på serveren som /opt/whoknows/backup-db.sh og køres af cron hver nat kl. 02:00 (deploy).
# Backups ældre end 14 dage slettes.
set -euo pipefail

dir=/opt/whoknows/db-backups
mkdir -p "$dir"
file="$dir/whoknows-$(date +%Y-%m-%d_%H%M).sql.gz"

# pg_dump laver en konsistent kopi, mens appen kører
docker exec whoknows-postgres pg_dump -U whoknows --no-owner whoknows | gzip > "$file"
chmod 600 "$file"

find "$dir" -name 'whoknows-*.sql.gz' -mtime +14 -delete
echo "$(date -Is) backup ok: $file ($(du -h "$file" | cut -f1))"

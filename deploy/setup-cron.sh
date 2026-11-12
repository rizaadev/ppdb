#!/bin/bash
# ==============================================================================
# Setup Daily PostgreSQL Backup Cron Job (Runs every night at 02:00 AM)
# ==============================================================================
set -euo pipefail

CRON_JOB="0 2 * * * /bin/bash /var/www/ppdb/scripts/backup-db.sh >> /var/backups/ppdb/cron.log 2>&1"
chmod +x /var/www/ppdb/scripts/backup-db.sh

# Check if cron job is already registered
if crontab -l 2>/dev/null | grep -Fq "backup-db.sh"; then
    echo "Cron job for backup-db.sh already exists in crontab."
else
    (crontab -l 2>/dev/null; echo "${CRON_JOB}") | crontab -
    echo "Successfully installed daily backup cron job (0 2 * * *)."
fi

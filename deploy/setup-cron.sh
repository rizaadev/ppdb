#!/bin/bash
# ==============================================================================
# Setup Daily PostgreSQL Backup Cron Job (Runs every night at 02:00 AM)
# ==============================================================================
set -euo pipefail

CRON_JOB="0 2 * * * /bin/bash /var/www/ppdb/scripts/backup-db.sh >> /var/www/ppdb/backups/cron.log 2>&1"
chmod +x /var/www/ppdb/scripts/backup-db.sh

# Remove any old cron jobs for backup-db.sh and install clean job
(crontab -l 2>/dev/null | grep -v "backup-db.sh" || true; echo "${CRON_JOB}") | crontab -
echo "Successfully installed daily backup cron job (0 2 * * *)."

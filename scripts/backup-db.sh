#!/bin/bash
# ==============================================================================
# PostgreSQL Automated Backup Script for PBDB / PPDB Platform
# Target: ppdb_platform database
# Retention: 14 days
# ==============================================================================
set -euo pipefail

ENV_FILE="${ENV_FILE:-/var/www/ppdb/.env}"
if [ -f "${ENV_FILE}" ]; then
  # Extract DATABASE_URL from .env if present
  DB_URL_VAL=$(grep -E '^DATABASE_URL=' "${ENV_FILE}" | cut -d '=' -f2- || true)
  if [ -n "${DB_URL_VAL}" ]; then
    DATABASE_URL="${DB_URL_VAL}"
  fi
fi

BACKUP_DIR="${BACKUP_DIR:-/var/www/ppdb/backups}"
mkdir -p "${BACKUP_DIR}"

DB_NAME="${DB_NAME:-ppdb_platform}"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
LOG_FILE="${BACKUP_DIR}/backup.log"

echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] Starting automated backup of ${DB_NAME}..." | tee -a "${LOG_FILE}"

if [ -n "${DATABASE_URL:-}" ]; then
  pg_dump "${DATABASE_URL}" --clean --if-exists | gzip > "${BACKUP_FILE}"
else
  DB_USER="${DB_USER:-ppdb_user}"
  DB_HOST="${DB_HOST:-127.0.0.1}"
  DB_PORT="${DB_PORT:-5432}"
  PGPASSWORD="${DB_PASSWORD:-}" pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" --clean --if-exists | gzip > "${BACKUP_FILE}"
fi

BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] Backup finished successfully: ${BACKUP_FILE} (${BACKUP_SIZE})" | tee -a "${LOG_FILE}"

# Prune backups older than 14 days
echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] Cleaning up backups older than 14 days..." | tee -a "${LOG_FILE}"
find "${BACKUP_DIR}" -name "${DB_NAME}_*.sql.gz" -type f -mtime +14 -delete

echo "[$(date -u +"%Y-%m-%dT%H:%M:%SZ")] Backup routine complete." | tee -a "${LOG_FILE}"

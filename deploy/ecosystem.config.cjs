const fs = require('node:fs');
const path = require('node:path');

const appRoot = path.resolve(__dirname, '..');
const envPath = path.join(appRoot, '.env');

function loadEnvFile(filePath) {
  const parsed = {};
  if (!fs.existsSync(filePath)) return parsed;
  const raw = fs.readFileSync(filePath, 'utf8');
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const eqIdx = trimmed.indexOf('=');
    if (eqIdx <= 0) continue;
    const key = trimmed.slice(0, eqIdx).trim();
    let val = trimmed.slice(eqIdx + 1).trim();
    if (
      (val.startsWith('"') && val.endsWith('"')) ||
      (val.startsWith("'") && val.endsWith("'"))
    ) {
      val = val.slice(1, -1);
    }
    parsed[key] = val;
  }
  return parsed;
}

const fileEnv = loadEnvFile(envPath);

module.exports = {
  apps: [
    {
      name: 'ppdb-platform',
      cwd: appRoot,
      script: path.join(appRoot, 'bin', 'pbdb-server'),
      interpreter: 'none',
      instances: 1,
      exec_mode: 'fork',
      autorestart: true,
      watch: false,
      max_memory_restart: '512M',
      env: {
        ...fileEnv,
      },
    },
  ],
};


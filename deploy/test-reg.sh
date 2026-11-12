#!/bin/bash
set -euo pipefail

echo "== Testing Public Check-Status with real registration no =="
curl -s -X POST https://alhamidcintamulya.fun/api/v1/public/check-status \
  -H "Content-Type: application/json" \
  -d '{"registration_no":"REGU-26-0001"}' | jq .


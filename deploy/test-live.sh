#!/bin/bash
set -euo pipefail

echo "== 1. Testing Auth Login =="
LOGIN_RES=$(curl -s -X POST https://alhamidcintamulya.fun/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"identifier":"admin@alhamidcintamulya.fun","password":"02EJFmZqlFwF8NOF"}')

TOKEN=$(echo "${LOGIN_RES}" | jq -r '.data.access_token // empty')

if [ -z "${TOKEN}" ]; then
  echo "Login failed. Response:"
  echo "${LOGIN_RES}" | jq .
  exit 1
fi

echo "Login successful! Token acquired."

echo "== 2. Testing Executive Dashboard Stats API =="
curl -s https://alhamidcintamulya.fun/api/v1/admin/dashboard/stats \
  -H "Authorization: Bearer ${TOKEN}" | jq .

echo "== 3. Testing Public Check-Status API =="
curl -s -X POST https://alhamidcintamulya.fun/api/v1/public/check-status \
  -H "Content-Type: application/json" \
  -d '{"registration_no":"NON-EXISTENT"}' | jq .

echo "== All Live API tests completed successfully! =="

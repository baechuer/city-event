#!/bin/bash
set -e

echo "=== Testing Join Event Flow ==="
echo ""

# Get auth token (you'll need to replace with your actual token)
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiJmMmQzMzgwMS0zZjFhLTRiZmQtOGNkNS01OGZjNDU3MzY1YjgiLCJyb2xlIjoiYWRtaW4iLCJ2ZXIiOjAsImlzcyI6ImF1dGgtc2VydmljZSIsInN1YiI6ImYyZDMzODAxLTNmMWEtNGJmZC04Y2Q1LTU4ZmM0NTczNjViOCIsImV4cCI6MTc2NzM0NzExNywiaWF0IjoxNzY3MzQ2MjE3fQ.1Nobt_zDofpoVLWs9TAFsKfR9xW97f_VUzk1N-Jtn1g"

# Event ID to test
EVENT_ID="120d2cd0-0d8f-483b-a67f-8fbb3b4f168c"

echo "1. Testing BFF health..."
curl -s http://localhost:8080/api/healthz
echo -e "\n"

echo "2. Getting event details..."
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/events/$EVENT_ID/view" | jq '.event.title, .actions'
echo ""

echo "3. Attempting to join event..."
IDEMPOTENCY_KEY=$(uuidgen)
curl -v -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -d '{}' \
  "http://localhost:8080/api/events/$EVENT_ID/join" 2>&1 | grep -E "< HTTP|error|status"

echo -e "\n=== Test Complete ==="

#!/bin/bash
# Test the API server endpoints
# Usage: ./scripts/test/test-api.sh [port]

set -e

PORT="${1:-3000}"
BASE_URL="http://localhost:$PORT"
SERVER_PID=""

# Cleanup function
cleanup() {
    if [ -n "$SERVER_PID" ]; then
        echo ""
        echo "Stopping server (PID: $SERVER_PID)..."
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
}

trap cleanup EXIT

echo "========================================="
echo "  Testing API Server"
echo "========================================="
echo "Base URL: $BASE_URL"
echo ""

# Step 1: Build the service
echo "Step 1: Building tile-service..."
echo "----------------------------------------"
go build -o tile-service
echo "✓ Build complete"
echo ""

# Step 2: Start server in background
echo "Step 2: Starting server..."
echo "----------------------------------------"
./tile-service serve -port $PORT > /tmp/tile-service-test.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"
echo "Waiting for server to start..."
sleep 3

# Check if server is running
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo "Error: Server failed to start"
    cat /tmp/tile-service-test.log
    exit 1
fi

echo "✓ Server started"
echo ""

# Step 3: Test health endpoint
echo "Step 3: Testing /health endpoint..."
echo "----------------------------------------"
HEALTH_RESPONSE=$(curl -s "$BASE_URL/health")
echo "Response: $HEALTH_RESPONSE"

if echo "$HEALTH_RESPONSE" | grep -q '"status":"ok"'; then
    echo "✓ Health check passed"
else
    echo "✗ Health check failed"
    exit 1
fi
echo ""

# Step 4: Test /api/regions endpoint (NEW)
echo "Step 4: Testing /api/regions endpoint..."
echo "----------------------------------------"
REGIONS_RESPONSE=$(curl -s "$BASE_URL/api/regions")

if command -v jq &> /dev/null; then
    REGION_COUNT=$(echo "$REGIONS_RESPONSE" | jq '. | length')
    echo "✓ Found $REGION_COUNT regions"
    echo "First 10 regions:"
    echo "$REGIONS_RESPONSE" | jq '.[:10]'
else
    echo "Response: $REGIONS_RESPONSE"
    echo "(Install 'jq' for pretty output)"
fi
echo ""

# Step 5: Test job creation
echo "Step 5: Testing job creation..."
echo "----------------------------------------"
CREATE_RESPONSE=$(curl -s -X POST \
    -H "Content-Type: application/json" \
    -d '{"region":"test-region","maxZoom":10,"skipUpload":true}' \
    "$BASE_URL/api/generate")

if command -v jq &> /dev/null; then
    JOB_ID=$(echo "$CREATE_RESPONSE" | jq -r '.jobId')
    echo "✓ Job created: $JOB_ID"
    echo "Response:"
    echo "$CREATE_RESPONSE" | jq '.'
else
    echo "Response: $CREATE_RESPONSE"
    JOB_ID=$(echo "$CREATE_RESPONSE" | grep -o '"jobId":"[^"]*"' | cut -d'"' -f4)
    echo "Job ID: $JOB_ID"
fi
echo ""

# Give job a moment to start
sleep 2

# Step 6: Test job status
echo "Step 6: Testing job status..."
echo "----------------------------------------"
STATUS_RESPONSE=$(curl -s "$BASE_URL/api/jobs/$JOB_ID")

if command -v jq &> /dev/null; then
    echo "$STATUS_RESPONSE" | jq '.'
    JOB_STATUS=$(echo "$STATUS_RESPONSE" | jq -r '.job.status')
    echo "✓ Job status: $JOB_STATUS"
else
    echo "Response: $STATUS_RESPONSE"
fi
echo ""

# Step 7: Test job cancellation (NEW)
echo "Step 7: Testing job cancellation..."
echo "----------------------------------------"
CANCEL_RESPONSE=$(curl -s -X POST "$BASE_URL/api/cancel/$JOB_ID")

if command -v jq &> /dev/null; then
    echo "$CANCEL_RESPONSE" | jq '.'
    if echo "$CANCEL_RESPONSE" | jq -e '.message' | grep -q "cancelled"; then
        echo "✓ Job cancelled successfully"
    else
        echo "✗ Cancellation response unexpected"
    fi
else
    echo "Response: $CANCEL_RESPONSE"
fi
echo ""

# Step 8: Test listing jobs
echo "Step 8: Testing job listing..."
echo "----------------------------------------"
LIST_RESPONSE=$(curl -s "$BASE_URL/api/jobs")

if command -v jq &> /dev/null; then
    JOB_COUNT=$(echo "$LIST_RESPONSE" | jq '. | length')
    echo "✓ Found $JOB_COUNT active jobs"
    echo "$LIST_RESPONSE" | jq '.'
else
    echo "Response: $LIST_RESPONSE"
fi
echo ""

echo "========================================="
echo "  API Test Complete"
echo "========================================="
echo ""
echo "Server log saved to: /tmp/tile-service-test.log"

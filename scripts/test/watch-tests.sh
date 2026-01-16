#!/bin/bash
# Watch mode for continuous testing during development
# Usage: ./scripts/test/watch-tests.sh [TestName]
#   ./scripts/test/watch-tests.sh                    # Run all tests
#   ./scripts/test/watch-tests.sh TestHaversine      # Run specific test

set -e

TEST_PATTERN="${1:-}"

# Check if fswatch is available (macOS) or inotifywait (Linux)
if command -v fswatch &> /dev/null; then
    WATCHER="fswatch"
elif command -v inotifywait &> /dev/null; then
    WATCHER="inotifywait"
else
    echo "Error: No file watcher found. Install fswatch (macOS) or inotify-tools (Linux)"
    echo "  macOS: brew install fswatch"
    echo "  Linux: apt-get install inotify-tools"
    exit 1
fi

echo "========================================="
echo "  Watch Mode - Continuous Testing"
echo "========================================="
echo "Test pattern: ${TEST_PATTERN:-all tests}"
echo "Watching: **/*.go"
echo "Press Ctrl+C to stop"
echo "========================================="
echo ""

# Run tests once immediately
run_tests() {
    clear
    echo "=== Running tests at $(date '+%H:%M:%S') ==="
    echo ""

    if [ -z "$TEST_PATTERN" ]; then
        go test -v ./... 2>&1 || true
    else
        go test -v -run "$TEST_PATTERN" ./... 2>&1 || true
    fi

    echo ""
    echo "=== Waiting for changes... ==="
}

# Initial run
run_tests

# Watch for changes
if [ "$WATCHER" = "fswatch" ]; then
    # macOS
    fswatch -o --event Updated --exclude '.*' --include '\.go$' . | while read; do
        run_tests
    done
else
    # Linux
    while inotifywait -r -e modify --include '.*\.go$' .; do
        run_tests
    done
fi

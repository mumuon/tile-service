#!/bin/bash
# Run all unit tests with coverage
# Usage: ./scripts/test/test-unit.sh [options]
#   -v, --verbose    Verbose output
#   -c, --coverage   Generate coverage report
#   -h, --html       Open coverage report in browser

set -e

VERBOSE=""
COVERAGE=false
HTML=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE="-v"
            shift
            ;;
        -c|--coverage)
            COVERAGE=true
            shift
            ;;
        -h|--html)
            COVERAGE=true
            HTML=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [-v|--verbose] [-c|--coverage] [-h|--html]"
            exit 1
            ;;
    esac
done

echo "========================================="
echo "  Running Unit Tests"
echo "========================================="
echo ""

if [ "$COVERAGE" = true ]; then
    echo "Running tests with coverage..."
    go test $VERBOSE -coverprofile=coverage.out -covermode=atomic ./...

    echo ""
    echo "Coverage summary:"
    go tool cover -func=coverage.out | tail -1

    if [ "$HTML" = true ]; then
        echo ""
        echo "Generating HTML coverage report..."
        go tool cover -html=coverage.out -o coverage.html
        echo "Opening coverage.html in browser..."

        # Open in default browser (cross-platform)
        if command -v open &> /dev/null; then
            open coverage.html
        elif command -v xdg-open &> /dev/null; then
            xdg-open coverage.html
        else
            echo "Coverage report saved to coverage.html"
        fi
    fi
else
    go test $VERBOSE ./...
fi

echo ""
echo "========================================="
echo "  Tests Complete"
echo "========================================="

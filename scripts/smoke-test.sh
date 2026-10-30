#!/bin/bash

set -e

URL="http://localhost:8080/"

echo "Running smoke test..."

RESPONSE=$(curl -s -f "$URL")

if [[ "$RESPONSE" == *"¿Who Knows?"* ]]; then
    echo "Smoke test passed!"
else
    echo "Smoke test failed!"
    exit 1
fi

#!/usr/bin/env bash
set -euo pipefail

# Redis Demo Integration Test
# This script tests the Redis queue backend using the example demo

echo "🔨 TaskForge Redis Demo Integration Test"
echo "========================================"

# Check if Redis is available
if ! command -v redis-cli &> /dev/null; then
    echo "❌ redis-cli not found, installing..."
    sudo apt-get update && sudo apt-get install -y redis-tools
fi

# Test Redis connection
echo "🔍 Testing Redis connection..."
for i in {1..30}; do
    if redis-cli -h 127.0.0.1 -p 6379 ping | grep -q PONG; then
        echo "✅ Redis is responding"
        break
    fi
    echo "⏳ Waiting for Redis... ($i/30)"
    sleep 2
    if [ $i -eq 30 ]; then
        echo "❌ Redis connection failed after 60 seconds"
        exit 1
    fi
done

# Run the Redis demo
echo "🚀 Running Redis demo..."
if [ -f "./bin/redis-demo" ]; then
    echo "📦 Using pre-built redis-demo binary"
    ./bin/redis-demo
elif [ -f "./examples/redis_demo.go" ]; then
    echo "🔨 Building and running redis-demo from source"
    go run ./examples/redis_demo.go
else
    echo "❌ Redis demo not found"
    exit 1
fi

echo "✅ Redis demo integration test completed successfully!"
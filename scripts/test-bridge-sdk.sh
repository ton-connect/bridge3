#!/bin/bash

# Script to test the bridge against ton-connect/bridge-sdk
set -e

BRIDGE_URL=${BRIDGE_URL:-"http://localhost:8081"}
BRIDGE_SDK_DIR="bridge-sdk"
BRIDGE_SDK_REPO="https://github.com/ton-connect/bridge-sdk.git"

echo "🚀 Starting bridge-sdk tests..."
echo "Bridge URL: $BRIDGE_URL"

# Check if Node.js and npm are available
if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed. Please install Node.js to run bridge-sdk tests."
    exit 1
fi

if ! command -v npm &> /dev/null; then
    echo "❌ npm is not installed. Please install npm to run bridge-sdk tests."
    exit 1
fi

# Clone or update bridge-sdk repository
if [ ! -d "$BRIDGE_SDK_DIR" ]; then
    echo "📦 Cloning bridge-sdk repository..."
    git clone "$BRIDGE_SDK_REPO" "$BRIDGE_SDK_DIR"
else
    echo "📦 Updating bridge-sdk repository..."
    cd "$BRIDGE_SDK_DIR"
    git pull origin main || git pull origin master
    cd ..
fi

# Install dependencies
echo "📋 Installing bridge-sdk dependencies..."
cd "$BRIDGE_SDK_DIR"
npm install

# Check if bridge is accessible
echo "🔍 Checking if bridge is accessible at $BRIDGE_URL..."
if ! curl -f "$BRIDGE_URL/health" &> /dev/null; then
    echo "⚠️  Warning: Bridge health endpoint is not accessible at $BRIDGE_URL/health"
    echo "   Make sure your bridge is running before running the tests."
    echo "   You can start it with: make build && ./callmebridge"
fi

# Run tests
echo "🧪 Running bridge-sdk tests..."
BRIDGE_URL="$BRIDGE_URL" npx jest

echo "✅ Bridge-sdk tests completed successfully!"

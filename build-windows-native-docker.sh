#!/bin/bash

# Build Windows executable using Windows Docker container
# IMPORTANT: This requires Docker to be in Windows containers mode
# On Windows: Right-click Docker Desktop tray icon → Switch to Windows containers
# On Linux/macOS: This requires a Windows Docker host or CI/CD service

set -e

echo "════════════════════════════════════════════════════════════════"
echo "  Building BuchISY using Native Windows Docker Container"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "⚠️  REQUIREMENTS:"
echo "   - Docker must be in Windows containers mode"
echo "   - Or use this script in CI/CD with Windows runner"
echo ""

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    echo "   Visit: https://www.docker.com/get-started"
    exit 1
fi

# Build using Windows container
echo "📦 Building with Windows Server Core + Go 1.25..."
docker build -t buchisy-windows-native -f Dockerfile.windows-native .

echo ""
echo "🔨 Extracting Windows executable..."
# Create a container and copy the exe out
docker create --name buchisy-extract buchisy-windows-native
docker cp buchisy-extract:C:\\build\\BuchISY.exe ./BuchISY.exe
docker rm buchisy-extract

if [ -f "BuchISY.exe" ]; then
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "📊 Build details:"
    echo "   File: BuchISY.exe"
    ls -lh BuchISY.exe 2>/dev/null || echo "   (Size info not available on non-Windows)"
    echo "   Type: Native Windows x64 executable"
    echo "   Translations: Embedded"
    echo ""
    echo "🚀 The executable is ready for distribution!"
else
    echo ""
    echo "❌ Build failed! BuchISY.exe was not created."
    exit 1
fi
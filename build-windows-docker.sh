#!/bin/bash

# Build Windows executable using Docker with MinGW
# This works on macOS, Linux, or any platform with Docker

set -e

echo "════════════════════════════════════════════════════════════════"
echo "  Building BuchISY for Windows using Docker + MinGW"
echo "════════════════════════════════════════════════════════════════"
echo ""

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    echo "   Visit: https://www.docker.com/get-started"
    exit 1
fi

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker."
    exit 1
fi

echo "📦 Building Docker image with MinGW toolchain..."
docker build -t buchisy-windows-builder -f Dockerfile.windows .

echo ""
echo "🔨 Compiling Windows executable..."
# Run the build and copy the executable out
docker run --rm -v "$PWD:/output" buchisy-windows-builder sh -c "cp /build/BuchISY.exe /output/"

if [ -f "BuchISY.exe" ]; then
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "📊 Build details:"
    echo "   File: BuchISY.exe"
    ls -lh BuchISY.exe | awk '{print "   Size:", $5}'
    echo "   Type: Windows x64 executable (with embedded translations)"
    echo ""
    echo "🚀 The executable is ready for distribution!"
    echo "   - Translations are embedded - no assets folder needed"
    echo "   - Can run standalone on any Windows 10+ system"
else
    echo ""
    echo "❌ Build failed! BuchISY.exe was not created."
    exit 1
fi
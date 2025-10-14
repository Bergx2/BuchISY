# Building BuchISY

## Prerequisites

- Go 1.22 or later
- Git
- For Windows builds: MinGW-w64 or Visual Studio (for CGO)
- For Docker builds: Docker installed and running

## Quick Start

```bash
# Clone the repository
git clone https://github.com/bergx2/buchisy.git
cd buchisy

# Download dependencies
make deps

# Build for your current platform
make build

# Run the application
make run
```

## Building for Your Platform

### macOS

```bash
# Simple build
make build

# Create .app bundle
make package-macos
```

### Windows

There are several options for building the Windows version:

#### Option 1: Build on Windows (Recommended)

This produces the best results with full CGO support:

```cmd
# On a Windows machine with Go installed
go build -o BuchISY.exe ./cmd/buchisy

# Or use the batch script
build-windows.bat
```

#### Option 2: Use GitHub Actions

The most reliable cross-platform solution is to use GitHub Actions:

1. Push your code to GitHub
2. Use the workflow below to automatically build for Windows

Create `.github/workflows/build-windows.yml`:

```yaml
name: Build Windows Release

on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:

jobs:
  build-windows:
    runs-on: windows-latest

    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.22'

    - name: Build Windows exe
      run: go build -o BuchISY.exe ./cmd/buchisy

    - name: Upload artifact
      uses: actions/upload-artifact@v3
      with:
        name: BuchISY-Windows
        path: BuchISY.exe
```

#### Option 3: Cross-compile from macOS (Limited)

**⚠️ Important:** Cross-compiling from macOS has limitations because Fyne requires CGO for full functionality. The cross-compiled version may have reduced GUI features or may not work at all.

If you still want to try:

```bash
# This will likely fail due to Fyne's CGO requirements
./build-windows-from-mac.sh
```

#### Option 4: Use Docker with MinGW (Recommended for Cross-Platform)

Build Windows executable from macOS/Linux using Docker:

```bash
# Method 1: Use the build script
make build-windows-docker
# or
./build-windows-docker.sh

# Method 2: Use Docker directly
docker build -t buchisy-windows -f Dockerfile.windows .
docker run --rm -v $PWD:/output buchisy-windows sh -c "cp /build/BuchISY.exe /output/"

# Method 3: Use Docker Compose
docker-compose -f docker-compose.build.yml up

# Method 4: Use pre-built MinGW image
docker pull x1unix/go-mingw
docker run --rm -v "$PWD":/app -w /app x1unix/go-mingw:latest \
  bash -c "GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o BuchISY.exe ./cmd/buchisy"
```

This method uses Docker with MinGW to properly compile the Windows executable with full CGO support, ensuring all Fyne UI features work correctly.

**Requirements:**
- Docker must be installed and running
- First build may take longer due to Docker image creation

**Output:**
- `BuchISY.exe` - Standalone Windows executable with embedded translations

## Embedded Translations

All builds now include embedded translations. The app will:
1. First try to load translations from the `assets` folder (for development)
2. Fall back to embedded translations if files aren't found

This means the Windows .exe can run standalone without any additional files.

## Build Troubleshooting

### "windows.h file not found" on macOS
This happens when trying to cross-compile with CGO enabled. Use one of the alternative methods above.

### OpenGL/GLFW errors when cross-compiling
Fyne requires CGO for rendering. Build on the target platform or use GitHub Actions.

### Missing translations (showing nls keys)
The latest version embeds translations. Rebuild with the latest code.

### Large executable size
The embedded translations and Fyne framework result in ~60MB executables. This is normal.

### Docker Build Issues
- Ensure Docker is installed: `docker --version`
- Ensure Docker is running: `docker info`
- On macOS, ensure Docker Desktop is running
- If permission denied: `sudo` may be needed or add user to docker group

### Windows Build on Non-Windows
- Use the Docker method (`make build-windows-docker`)
- Direct cross-compilation without Docker may result in missing UI features due to CGO limitations

## Testing Builds

After building:

1. **Test translations**: Launch the app and check Settings → Language
2. **Test file operations**: Try processing a sample PDF
3. **Test Claude API**: If using Claude mode, verify API calls work
4. **Test file dialogs**: Ensure file selection works properly

## Distribution

### macOS
- Distribute the `.app` bundle
- Users may need to right-click → Open on first launch (Gatekeeper)
- Consider code signing for smoother installation

### Windows
- Distribute the standalone `.exe` file
- No additional files needed (translations are embedded)
- Consider code signing to avoid Windows Defender warnings

## Continuous Integration

For automated builds, use GitHub Actions:

1. Windows builds: Use `windows-latest` runner
2. macOS builds: Use `macos-latest` runner
3. Upload artifacts for distribution
4. Create releases automatically on tags

See `.github/workflows/` for example workflows.
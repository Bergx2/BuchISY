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

#### Option 2: Use GitHub Actions (Recommended for CI/CD)

The most reliable automated solution is to use GitHub Actions with a Windows runner:

1. The workflow is already set up in `.github/workflows/build-windows.yml`
2. To trigger a build:
   - Push a tag starting with `v` (e.g., `v1.2.0`)
   - Or go to Actions tab → Build Windows Release → Run workflow
3. Download the artifact from the Actions run

The workflow automatically:
- Sets up Go 1.25 on Windows
- Builds with native Windows toolchain
- Uploads the executable as an artifact
- Creates a release (when triggered by a tag)

#### Option 3: Cross-compile from macOS (Limited)

**⚠️ Important:** Cross-compiling from macOS has limitations because Fyne requires CGO for full functionality. The cross-compiled version may have reduced GUI features or may not work at all.

If you still want to try:

```bash
# This will likely fail due to Fyne's CGO requirements
./build-windows-from-mac.sh
```

#### Option 4: Use Docker

There are two Docker approaches, depending on your needs:

##### 4a. Windows Docker Container (Best Compatibility)

Uses native Windows container for perfect compatibility:

```bash
# Requires Docker in Windows containers mode
make build-windows-docker
# or
./build-windows-native-docker.sh
```

**Requirements:**
- Windows host with Docker Desktop in Windows containers mode
- OR use in CI/CD with Windows runner (GitHub Actions, Azure DevOps)

**Pros:**
- Native Windows build environment
- Full compatibility with all dependencies
- No cross-compilation issues

##### 4b. Linux Docker with MinGW (Cross-Compile)

Attempts cross-compilation from Linux container:

```bash
# Works on any Docker host but may have compatibility issues
make build-windows-docker-mingw
# or
./build-windows-docker.sh
```

**⚠️ Warning:** This method may fail with certain dependencies (like go-fitz) that include pre-compiled Windows libraries built with MSVC.

**Requirements:**
- Docker installed and running (any OS)
- May not work with all dependencies

**Output (both methods):**
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
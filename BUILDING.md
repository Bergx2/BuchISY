# Building BuchISY

## Prerequisites

- Go 1.25 or later (see `go.mod`)
- Git
- For Windows builds: MinGW-w64 or Visual Studio (for CGO)
- For Docker builds: Docker installed and running

## Quick Start

```bash
# Clone the repository
git clone https://github.com/Bergx2/BuchISY.git
cd BuchISY

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

There is no dedicated macOS cross-compile script. Use the Docker path instead (see Option 4):

```bash
./build-windows-docker.sh
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

### Slow Windows builds (incremental rebuild takes minutes instead of seconds)

**Symptom:** You change a few `.go` files and `go build` takes many minutes — sometimes 10–20.

**This is almost never the code or the stack.** This project is small (~36k lines); a warm
*incremental* rebuild is normally **a few seconds** (the only fixed cost is the linker pulling in
go-fitz's prebuilt MuPDF static libraries). Some facts that rule out the usual scapegoats:

- `modernc.org/sqlite` is **pure Go** — no C, no compile cost.
- `go-fitz` (`v1.24.15`) ships **prebuilt MuPDF `.a` libraries** — it's a *link* cost, not a
  per-build *compile* cost. It does not recompile MuPDF on every build.
- Fyne's GLFW/OpenGL C bindings compile **once** and are cached — they do **not** recompile on an
  incremental `.go` edit unless something is throwing the cache away.

So a 20-minute *incremental* rebuild means the Windows machine is either **defeating Go's build
cache** or **having every build artifact scanned/locked**. Diagnose, then fix:

**1. Measure and check the cache (PowerShell, in the repo):**

```powershell
go env GOCACHE GOMODCACHE GOFLAGS CGO_ENABLED   # GOFLAGS must NOT contain -a; GOCACHE must not be "off"
# edit one file (add a blank line), then time the rebuild:
Measure-Command { go build -o NUL ./cmd/buchisy }
# is it recompiling dependencies? (any Fyne/glfw/go-fitz/sqlite lines = cache miss):
go build -x -o NUL ./cmd/buchisy 2>&1 | Select-Object -First 40
```

**2. Add antivirus exclusions (the #1 fix on Windows — run elevated PowerShell):**

Windows Defender real-time scanning of the Go cache + linker temp files is the most common cause.
The cgo/link step produces thousands of small files; scanning each one synchronously turns seconds
into minutes.

```powershell
Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\go-build"   # GOCACHE
Add-MpPreference -ExclusionPath "$env:USERPROFILE\go\pkg\mod"  # GOMODCACHE
Add-MpPreference -ExclusionPath "$env:TEMP"                    # linker temp files
Add-MpPreference -ExclusionPath "C:\path\to\BuchISY"           # the repo / build output
Add-MpPreference -ExclusionProcess "go.exe"
Add-MpPreference -ExclusionProcess "gcc.exe"                   # MinGW (cgo)
```

**3. Get the repo and cache off OneDrive / synced folders.** If the repo lives under a synced
`Documents`/`Desktop` (OneDrive), every build output is uploaded/locked mid-build. Move the repo to
a plain local path like `C:\dev\BuchISY`. Optionally pin a fast, non-synced cache:

```powershell
go env -w GOCACHE=C:\gocache GOMODCACHE=C:\gomod   # then add both to the AV exclusions above
```

**4. Use the fast inner-loop build.** `make dev` (or plain `go build -o build\buchisy.exe
./cmd/buchisy`) — avoid `make all`/packaging while iterating, those add tests + bundling.

**Expected result:** an incremental rebuild drops to **a few seconds** (well under 1 minute). If
`go build -x` after step 1 shows it recompiling dependencies on a trivial edit, the cache is being
discarded — steps 2 and 3 fix that. (Note: a slow build is an *environment* problem that follows
the machine, not the language — switching stacks would not fix it; e.g. a Rust/Tauri cold build is
itself multi-minute.)

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
2. **Test invoice extraction**: Process a sample PDF (Claude or Local mode)
3. **Test the booking engine**: Create a booking and verify it posts to the SKR04 accounts
4. **Test a report**: Generate a SuSa/GuV or UStVA for the current period
5. **Test an export**: Run a DATEV/GoBD export and confirm the archive is produced
6. **Test profiles (Mandanten)**: Switch profile and confirm data isolation
7. **Test file dialogs**: Ensure file selection works properly

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

### Automated Builds with GitHub Actions

We have several GitHub Actions workflows configured:

#### 1. Individual Platform Workflows

- **Windows Build** (`.github/workflows/build-windows.yml`)
  - Triggers on version tags (`v*`) or manual dispatch
  - Builds on Windows Server with native toolchain
  - Creates Windows executable with embedded translations

- **macOS Build** (`.github/workflows/build-macos.yml`)
  - Triggers on version tags (`v*`) or manual dispatch
  - Builds on macOS with Fyne packaging
  - Creates both .app bundle and DMG installer

#### 2. Combined Multi-Platform Build

- **Build All Platforms** (`.github/workflows/build-all.yml`)
  - Builds Windows and macOS in parallel
  - Automatically creates GitHub release with all artifacts
  - Most efficient for releases

#### 3. Signed macOS Build (Optional)

- **Build macOS Signed** (`.github/workflows/build-macos-signed.yml`)
  - Manual trigger only
  - Requires Apple Developer certificates in GitHub secrets
  - Signs and notarizes the app for distribution without Gatekeeper warnings

### Triggering Builds

#### Automatic (on version tags):
```bash
git tag v1.3.0
git push origin v1.3.0
```

#### Manual (from GitHub UI):
1. Go to Actions tab
2. Select the workflow
3. Click "Run workflow"

### Required GitHub Secrets (for signed builds only)

For signed macOS builds, add these secrets in repository settings:
- `APPLE_CERTIFICATE_BASE64` - Base64 encoded .p12 certificate
- `APPLE_CERTIFICATE_PASSWORD` - Certificate password
- `APPLE_TEAM_ID` - Apple Developer Team ID
- `APPLE_ID` - Apple ID email
- `APPLE_APP_PASSWORD` - App-specific password
@echo off
REM Windows build script for BuchISY with embedded translations

echo Building BuchISY for Windows with embedded translations...

REM Build the executable with embedded assets
go build -o BuchISY.exe ./cmd/buchisy

if %ERRORLEVEL% NEQ 0 (
    echo Build failed!
    exit /b 1
)

echo.
echo Build complete!
echo.
echo The BuchISY.exe now includes embedded translations and can run standalone.
echo No additional files are needed for distribution.
echo.
echo File: BuchISY.exe
echo Size: ~63MB
echo.
echo You can now distribute just the BuchISY.exe file.
$ErrorActionPreference = "Stop"

Write-Host "====================================" -ForegroundColor Cyan
Write-Host "    SteamTracker Installer Builder   " -ForegroundColor Cyan
Write-Host "====================================" -ForegroundColor Cyan

echo "v1.0.0"

# Step 1: Build the Go executable
Write-Host "`n[1/3] Building Go application (GUI mode)..." -ForegroundColor Yellow
try {
    go build -ldflags "-H=windowsgui" -o SteamTracker.exe .
} catch {
    Write-Host "Error during 'go build': $_" -ForegroundColor Red
    exit 1
}

if (-Not (Test-Path "SteamTracker.exe")) {
    Write-Host "Error: SteamTracker.exe was not created." -ForegroundColor Red
    exit 1
}
Write-Host "Success: SteamTracker.exe built." -ForegroundColor Green


# Step 2: Find Inno Setup compiler
Write-Host "`n[2/3] Looking for Inno Setup Compiler (ISCC)..." -ForegroundColor Yellow
$isccPaths = @(
    "C:\Program Files (x86)\Inno Setup 6\ISCC.exe",
    "C:\Program Files\Inno Setup 6\ISCC.exe",
    "C:\Program Files (x86)\Inno Setup 5\ISCC.exe",
    "C:\Program Files\Inno Setup 5\ISCC.exe"
)

$iscc = $null
foreach ($path in $isccPaths) {
    if (Test-Path $path) {
        $iscc = $path
        break
    }
}

if ($null -eq $iscc) {
    Write-Host "Error: Inno Setup Compiler (iscc.exe) not found." -ForegroundColor Red
    Write-Host "Please install Inno Setup from https://jrsoftware.org/isdl.php" -ForegroundColor Cyan
    exit 1
}
Write-Host "Found ISCC at: $iscc" -ForegroundColor Green


# Step 3: Compile the installer
Write-Host "`n[3/3] Compiling installer with Inno Setup..." -ForegroundColor Yellow
if (-Not (Test-Path "installer.iss")) {
    Write-Host "Error: installer.iss not found in the current directory." -ForegroundColor Red
    exit 1
}

# Run ISCC
$process = Start-Process -FilePath $iscc -ArgumentList "`"installer.iss`"" -Wait -NoNewWindow -PassThru

if ($process.ExitCode -eq 0) {
    Write-Host "`n====================================" -ForegroundColor Cyan
    Write-Host "SUCCESS: Installer generated!" -ForegroundColor Green
    Write-Host "Check the 'dist' folder for SteamTrackerSetup.exe" -ForegroundColor Green
    Write-Host "====================================" -ForegroundColor Cyan
} else {
    Write-Host "`nError: Inno Setup failed to compile the installer." -ForegroundColor Red
    exit 1
}

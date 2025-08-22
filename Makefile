# Makefile for SteamTracker on Windows
# Requires: Go toolchain, PowerShell, and optionally Administrator rights for firewall rules.

EXE=SteamTracker.exe
PKG=.

.PHONY: all build build-gui run run-gui clean install-startup uninstall-startup firewall-allow firewall-remove

all: build-gui

# Standard console build (console window visible)
build:
	go build -o $(EXE) $(PKG)

# GUI-subsystem build (no console window, runs quietly in background)
build-gui:
	go build -ldflags -H=windowsgui -o $(EXE) $(PKG)

run:
	go run $(PKG)

# Build GUI and launch detached
run-gui: build-gui
	powershell -NoProfile -Command "Start-Process -FilePath \"$(EXE)\""

clean:
	powershell -NoProfile -Command "Remove-Item -ErrorAction SilentlyContinue $(EXE)"

# Add to current user startup (registry Run key)
# This will launch the compiled EXE at logon. Make sure you built first.
install-startup: build-gui
	powershell -NoProfile -Command "$$p = (Resolve-Path \"$(EXE)\").Path; $$name = 'SteamTracker'; $$key = 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run'; New-ItemProperty -Path $$key -Name $$name -Value $$p -PropertyType String -Force | Out-Null; Write-Host 'Startup entry added:' $$p"

uninstall-startup:
	powershell -NoProfile -Command "$$name = 'SteamTracker'; $$key = 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Run'; if (Get-ItemProperty -Path $$key -Name $$name -ErrorAction SilentlyContinue) { Remove-ItemProperty -Path $$key -Name $$name -Force; Write-Host 'Startup entry removed.' } else { Write-Host 'Startup entry not found.' }"

# Optional: Pre-authorize firewall rule to avoid prompts if binding changes in the future.
# Not strictly required since we bind to 127.0.0.1, but kept for convenience.
# Note: needs elevated PowerShell (Run as Administrator).
firewall-allow: build-gui
	powershell -NoProfile -Command "$$p = (Resolve-Path \"$(EXE)\").Path; $$r = 'SteamTracker Localhost 8080'; netsh advfirewall firewall delete rule name=\"'$$r'\" > $null 2>&1; netsh advfirewall firewall add rule name=\"'$$r'\" dir=in action=allow program=\"'$$p'\" enable=yes profile=any protocol=TCP localport=8080 localip=127.0.0.1 | Out-Null; Write-Host 'Firewall rule added for' $$p"

firewall-remove:
	powershell -NoProfile -Command "$$r = 'SteamTracker Localhost 8080'; netsh advfirewall firewall delete rule name=\"'$$r'\" | Out-Null; Write-Host 'Firewall rule removed.'" 

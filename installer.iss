[Setup]
AppName=SteamTracker
AppVersion=1.0
DefaultDirName={userpf}\SteamTracker
DefaultGroupName=SteamTracker
UninstallDisplayIcon={app}\SteamTracker.exe
Compression=lzma2
SolidCompression=yes
OutputDir=dist
OutputBaseFilename=SteamTrackerSetup
SetupIconFile=icon.ico
PrivilegesRequired=lowest

[Files]
; L'exécutable et l'icône doivent être dans le même dossier pour que le code les trouve
Source: "SteamTracker.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "icon.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; Raccourci dans le menu démarrer
Name: "{group}\SteamTracker"; Filename: "{app}\SteamTracker.exe"
; Raccourci dans le dossier de démarrage
Name: "{userstartup}\SteamTracker"; Filename: "{app}\SteamTracker.exe"

[Registry]
; Inscription dans la base de registre pour le démarrage automatique
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "SteamTracker"; ValueData: """{app}\SteamTracker.exe"""; Flags: uninsdeletevalue

[Run]
; Option pour lancer l'app immédiatement après l'installation
Filename: "{app}\SteamTracker.exe"; Description: "Lancer SteamTracker maintenant"; Flags: nowait postinstall skipifsilent

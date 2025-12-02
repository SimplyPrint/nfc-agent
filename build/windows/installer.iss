; NFC Agent Installer Script for Inno Setup
; Build with: iscc /DVersion=1.0.0 installer.iss

#ifndef Version
  #define Version "1.0.0"
#endif

[Setup]
AppName=NFC Agent
AppVersion={#Version}
AppVerName=NFC Agent {#Version}
AppPublisher=SimplyPrint
AppPublisherURL=https://simplyprint.io
AppSupportURL=https://github.com/SimplyPrint/nfc-agent/issues
AppUpdatesURL=https://github.com/SimplyPrint/nfc-agent/releases
DefaultDirName={autopf}\NFC Agent
DefaultGroupName=NFC Agent
OutputDir=..\..\dist
OutputBaseFilename=NFC-Agent-Setup-{#Version}
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\nfc-agent.exe
SetupIconFile=..\..\assets\icon.ico
WizardStyle=modern

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "..\..\dist\nfc-agent_windows_amd64_v1\nfc-agent.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\NFC Agent"; Filename: "{app}\nfc-agent.exe"
Name: "{group}\Uninstall NFC Agent"; Filename: "{uninstallexe}"

[Tasks]
Name: "startup"; Description: "Start NFC Agent when Windows starts"; GroupDescription: "Additional options:"

[Registry]
; Add to startup if task selected
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "NFCAgent"; ValueData: """{app}\nfc-agent.exe"" --no-tray"; Flags: uninsdeletevalue; Tasks: startup

[Run]
Filename: "{app}\nfc-agent.exe"; Description: "Launch NFC Agent"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; Stop the running process before uninstall
Filename: "taskkill"; Parameters: "/F /IM nfc-agent.exe"; Flags: runhidden; RunOnceId: "StopNFCAgent"

[Code]
function InitializeSetup(): Boolean;
begin
  Result := True;
end;

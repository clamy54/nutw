; Inno Setup script for Nutw.
;
; The nutw.exe binary is expected in the directory given by BuildDir,
; which defaults to ..\build :
;
;   go build -o build\nutw.exe .\src
;   ISCC.exe /DAppVersion=1.4 installer\nutw.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#ifndef BuildDir
  #define BuildDir "..\build"
#endif

#define AppName "Nutw"
#define AppPublisher "Cyril LAMY"
#define AppURL "https://github.com/clamy54/nutw"
#define AppExeName "nutw.exe"

[Setup]
AppId={{6E1C4A2F-9B7D-4C3E-8A51-2F0D6B8E4C17}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}/releases
VersionInfoVersion={#AppVersion}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
LicenseFile=..\LICENSE.md
OutputDir=..\dist
OutputBaseFilename=nutw-install
Compression=lzma
SolidCompression=yes
WizardStyle=modern
; The service is registered and the machine is shut down by Nutw, both
; need an elevated installer.
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#AppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#BuildDir}\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\LICENSE.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Tasks]
Name: "installservice"; Description: "Install and start Nutw as a Windows service"; GroupDescription: "Service:"

[INI]
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "server"; String: "{code:GetServer}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "port"; String: "{code:GetPort}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "interval"; String: "{code:GetInterval}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "login"; String: "{code:GetLogin}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "password"; String: "{code:GetPassword}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "upsname"; String: "{code:GetUpsName}"
Filename: "{app}\nutw.ini"; Section: "nutd"; Key: "usetls"; String: "{code:GetUseTls}"

[Run]
Filename: "{app}\{#AppExeName}"; Parameters: "--mode=install"; StatusMsg: "Installing the {#AppName} service..."; Flags: runhidden waituntilterminated; Tasks: installservice

[UninstallRun]
Filename: "{app}\{#AppExeName}"; Parameters: "--mode=uninstall"; RunOnceId: "removeservice"; Flags: runhidden waituntilterminated

[UninstallDelete]
Type: files; Name: "{app}\nutw.ini"

[Code]
var
  CfgPage: TInputQueryWizardPage;
  UseTlsBox: TNewCheckBox;

const
  FieldServer = 0;
  FieldPort = 1;
  FieldUpsName = 2;
  FieldLogin = 3;
  FieldPassword = 4;
  FieldInterval = 5;

procedure InitializeWizard;
begin
  CfgPage := CreateInputQueryPage(wpSelectDir,
    'Nut server',
    'Which upsd daemon should Nutw probe ?',
    'These settings are written to nutw.ini in the installation folder, ' +
    'where they can be edited later on.');

  CfgPage.Add('Server (hostname or ip address) :', False);
  CfgPage.Add('Port :', False);
  CfgPage.Add('Ups name, as served by the upsd daemon :', False);
  CfgPage.Add('Login (leave empty if the daemon needs no authentication) :', False);
  CfgPage.Add('Password :', True);
  CfgPage.Add('Amount of time between two probes (seconds) :', False);

  CfgPage.Values[FieldPort] := '3493';
  CfgPage.Values[FieldInterval] := '60';

  UseTlsBox := TNewCheckBox.Create(CfgPage);
  UseTlsBox.Parent := CfgPage.Surface;
  UseTlsBox.Left := CfgPage.Edits[FieldInterval].Left;
  UseTlsBox.Top := CfgPage.Edits[FieldInterval].Top +
    CfgPage.Edits[FieldInterval].Height + ScaleY(14);
  UseTlsBox.Width := CfgPage.SurfaceWidth - UseTlsBox.Left;
  UseTlsBox.Height := ScaleY(17);
  UseTlsBox.Caption := 'Talk to the upsd daemon over a TLS session';
end;

function IsNumeric(Value: String): Boolean;
var
  Index: Integer;
begin
  Result := Length(Value) > 0;

  for Index := 1 to Length(Value) do
    if (Value[Index] < '0') or (Value[Index] > '9') then
      Result := False;
end;

// Load the settings of a previous installation, so an upgrade does not
// silently reset the configuration.
procedure CurPageChanged(CurPageID: Integer);
var
  IniFile: String;
begin
  if CurPageID <> CfgPage.ID then
    Exit;

  IniFile := ExpandConstant('{app}\nutw.ini');

  if not FileExists(IniFile) then
    Exit;

  CfgPage.Values[FieldServer] := GetIniString('nutd', 'server', CfgPage.Values[FieldServer], IniFile);
  CfgPage.Values[FieldPort] := GetIniString('nutd', 'port', CfgPage.Values[FieldPort], IniFile);
  CfgPage.Values[FieldUpsName] := GetIniString('nutd', 'upsname', CfgPage.Values[FieldUpsName], IniFile);
  CfgPage.Values[FieldLogin] := GetIniString('nutd', 'login', CfgPage.Values[FieldLogin], IniFile);
  CfgPage.Values[FieldPassword] := GetIniString('nutd', 'password', CfgPage.Values[FieldPassword], IniFile);
  CfgPage.Values[FieldInterval] := GetIniString('nutd', 'interval', CfgPage.Values[FieldInterval], IniFile);
  UseTlsBox.Checked := GetIniString('nutd', 'usetls', '0', IniFile) = '1';
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;

  if CurPageID <> CfgPage.ID then
    Exit;

  if Trim(CfgPage.Values[FieldServer]) = '' then
  begin
    MsgBox('Please enter the hostname or the ip address of the upsd server.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if Trim(CfgPage.Values[FieldUpsName]) = '' then
  begin
    MsgBox('Please enter the name of the ups, as served by the upsd daemon.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if not IsNumeric(Trim(CfgPage.Values[FieldPort])) then
  begin
    MsgBox('The port must be a number.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if not IsNumeric(Trim(CfgPage.Values[FieldInterval])) then
  begin
    MsgBox('The interval must be a number of seconds.', mbError, MB_OK);
    Result := False;
    Exit;
  end;

  if StrToInt(Trim(CfgPage.Values[FieldInterval])) < 10 then
  begin
    MsgBox('Nutw does not probe more than once every 10 seconds.', mbError, MB_OK);
    Result := False;
    Exit;
  end;
end;

// The running service holds nutw.exe, it has to go before the files are
// replaced.
procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep <> ssInstall then
    Exit;

  if FileExists(ExpandConstant('{app}\{#AppExeName}')) then
    Exec(ExpandConstant('{app}\{#AppExeName}'), '--mode=uninstall', '',
      SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function GetServer(Param: String): String;
begin
  Result := Trim(CfgPage.Values[FieldServer]);
end;

function GetPort(Param: String): String;
begin
  Result := Trim(CfgPage.Values[FieldPort]);
end;

function GetUpsName(Param: String): String;
begin
  Result := Trim(CfgPage.Values[FieldUpsName]);
end;

function GetLogin(Param: String): String;
begin
  Result := Trim(CfgPage.Values[FieldLogin]);
end;

function GetPassword(Param: String): String;
begin
  Result := CfgPage.Values[FieldPassword];
end;

function GetInterval(Param: String): String;
begin
  Result := Trim(CfgPage.Values[FieldInterval]);
end;

function GetUseTls(Param: String): String;
begin
  if UseTlsBox.Checked then
    Result := '1'
  else
    Result := '0';
end;

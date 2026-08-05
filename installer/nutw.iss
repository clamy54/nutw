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
// The settings are split over two pages : a wizard page cannot show more
// than five fields without the last one falling below the bottom of the
// window, which cannot be resized.
var
  SrvPage: TInputQueryWizardPage;
  AuthPage: TInputQueryWizardPage;
  UseTlsBox: TNewCheckBox;

const
  FieldServer = 0;
  FieldPort = 1;
  FieldUpsName = 2;

  FieldLogin = 0;
  FieldPassword = 1;
  FieldInterval = 2;

procedure InitializeWizard;
begin
  SrvPage := CreateInputQueryPage(wpSelectDir,
    'Nut server',
    'Which upsd daemon should Nutw probe ?',
    'These settings are written to nutw.ini in the installation folder, ' +
    'where they can be edited later on.');

  SrvPage.Add('Server (hostname or ip address) :', False);
  SrvPage.Add('Port :', False);
  SrvPage.Add('Ups name, as served by the upsd daemon :', False);

  SrvPage.Values[FieldPort] := '3493';

  UseTlsBox := TNewCheckBox.Create(SrvPage);
  UseTlsBox.Parent := SrvPage.Surface;
  UseTlsBox.Left := SrvPage.Edits[FieldUpsName].Left;
  UseTlsBox.Top := SrvPage.Edits[FieldUpsName].Top +
    SrvPage.Edits[FieldUpsName].Height + ScaleY(16);
  UseTlsBox.Width := SrvPage.SurfaceWidth - UseTlsBox.Left;
  UseTlsBox.Height := ScaleY(17);
  UseTlsBox.Caption := 'Talk to the upsd daemon over a TLS session';

  AuthPage := CreateInputQueryPage(SrvPage.ID,
    'Nut server',
    'How should Nutw log in, and how often should it probe ?',
    'Leave the login and the password empty if the upsd daemon needs no ' +
    'authentication.');

  AuthPage.Add('Login :', False);
  AuthPage.Add('Password :', True);
  AuthPage.Add('Amount of time between two probes (seconds) :', False);

  AuthPage.Values[FieldInterval] := '60';
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
  if CurPageID <> SrvPage.ID then
    Exit;

  IniFile := ExpandConstant('{app}\nutw.ini');

  if not FileExists(IniFile) then
    Exit;

  SrvPage.Values[FieldServer] := GetIniString('nutd', 'server', SrvPage.Values[FieldServer], IniFile);
  SrvPage.Values[FieldPort] := GetIniString('nutd', 'port', SrvPage.Values[FieldPort], IniFile);
  SrvPage.Values[FieldUpsName] := GetIniString('nutd', 'upsname', SrvPage.Values[FieldUpsName], IniFile);
  UseTlsBox.Checked := GetIniString('nutd', 'usetls', '0', IniFile) = '1';

  AuthPage.Values[FieldLogin] := GetIniString('nutd', 'login', AuthPage.Values[FieldLogin], IniFile);
  AuthPage.Values[FieldPassword] := GetIniString('nutd', 'password', AuthPage.Values[FieldPassword], IniFile);
  AuthPage.Values[FieldInterval] := GetIniString('nutd', 'interval', AuthPage.Values[FieldInterval], IniFile);
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;

  if CurPageID = SrvPage.ID then
  begin
    if Trim(SrvPage.Values[FieldServer]) = '' then
    begin
      MsgBox('Please enter the hostname or the ip address of the upsd server.', mbError, MB_OK);
      Result := False;
      Exit;
    end;

    if Trim(SrvPage.Values[FieldUpsName]) = '' then
    begin
      MsgBox('Please enter the name of the ups, as served by the upsd daemon.', mbError, MB_OK);
      Result := False;
      Exit;
    end;

    if not IsNumeric(Trim(SrvPage.Values[FieldPort])) then
    begin
      MsgBox('The port must be a number.', mbError, MB_OK);
      Result := False;
      Exit;
    end;
  end;

  if CurPageID = AuthPage.ID then
  begin
    if not IsNumeric(Trim(AuthPage.Values[FieldInterval])) then
    begin
      MsgBox('The interval must be a number of seconds.', mbError, MB_OK);
      Result := False;
      Exit;
    end;

    if StrToInt(Trim(AuthPage.Values[FieldInterval])) < 10 then
    begin
      MsgBox('Nutw does not probe more than once every 10 seconds.', mbError, MB_OK);
      Result := False;
      Exit;
    end;
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
  Result := Trim(SrvPage.Values[FieldServer]);
end;

function GetPort(Param: String): String;
begin
  Result := Trim(SrvPage.Values[FieldPort]);
end;

function GetUpsName(Param: String): String;
begin
  Result := Trim(SrvPage.Values[FieldUpsName]);
end;

function GetLogin(Param: String): String;
begin
  Result := Trim(AuthPage.Values[FieldLogin]);
end;

function GetPassword(Param: String): String;
begin
  Result := AuthPage.Values[FieldPassword];
end;

function GetInterval(Param: String): String;
begin
  Result := Trim(AuthPage.Values[FieldInterval]);
end;

function GetUseTls(Param: String): String;
begin
  if UseTlsBox.Checked then
    Result := '1'
  else
    Result := '0';
end;

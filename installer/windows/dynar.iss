; Both native Windows binaries are bundled; no credentials are collected.
#ifndef ReleaseTag
  #error ReleaseTag must be supplied
#endif
#ifndef NumericVersion
  #error NumericVersion must be supplied
#endif
#ifndef AssetDir
  #error AssetDir must be supplied
#endif

[Setup]
AppId=fernandocpaz.DynaR
AppName=DynaR
AppVersion={#ReleaseTag}
AppPublisher=fernandocpaz
AppPublisherURL=https://github.com/fernandocpaz/dynar
AppSupportURL=https://github.com/fernandocpaz/dynar/issues
AppUpdatesURL=https://github.com/fernandocpaz/dynar/releases/latest
VersionInfoVersion={#NumericVersion}
DefaultDirName={localappdata}\Programs\DynaR
DisableDirPage=yes
DisableProgramGroupPage=yes
DefaultGroupName=DynaR
PrivilegesRequired=lowest
ArchitecturesAllowed=x64os or arm64
ArchitecturesInstallIn64BitMode=x64os or arm64
MinVersion=10.0
ChangesEnvironment=yes
OutputBaseFilename=DynaR-Setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\dynar.exe
CloseApplications=yes
RestartApplications=no

[Files]
Source: "{#AssetDir}\dynar_{#ReleaseTag}_windows_amd64.exe"; DestDir: "{app}"; DestName: "dynar.exe"; Flags: ignoreversion; Check: not IsArm64
Source: "{#AssetDir}\dynar_{#ReleaseTag}_windows_arm64.exe"; DestDir: "{app}"; DestName: "dynar.exe"; Flags: ignoreversion; Check: IsArm64

[Icons]
Name: "{userprograms}\DynaR"; Filename: "{app}\dynar.exe"; WorkingDir: "{app}"

[Run]
Filename: "{app}\dynar.exe"; Description: "Launch DynaR and configure your Dynatrace connection"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent unchecked

[Code]
const
  EnvKey = 'Environment';
  OwnerKey = 'Software\fernandocpaz\DynaR\Installer';

function SamePath(A, B: String): Boolean;
begin
  A := RemoveQuotes(Trim(A));
  B := RemoveQuotes(Trim(B));
  Result := CompareText(RemoveBackslashUnlessRoot(A), RemoveBackslashUnlessRoot(B)) = 0;
end;

function ContainsPath(Value, Entry: String): Boolean;
var
  P: Integer;
  Part: String;
begin
  Result := False;
  repeat
    P := Pos(';', Value);
    if P = 0 then
      Part := Value
    else
      Part := Copy(Value, 1, P - 1);
    if SamePath(Part, Entry) then
    begin
      Result := True;
      Exit;
    end;
    if P > 0 then
      Delete(Value, 1, P);
  until P = 0;
end;

function RemoveOwnedPath(Value, Entry: String): String;
var
  StartPos, EndPos: Integer;
  Part: String;
begin
  Result := Value;
  StartPos := 1;
  while StartPos <= Length(Value) do
  begin
    EndPos := StartPos;
    while EndPos <= Length(Value) do
    begin
      if Value[EndPos] = ';' then
        Break;
      EndPos := EndPos + 1;
    end;
    Part := Copy(Value, StartPos, EndPos - StartPos);
    if SamePath(Part, Entry) then
    begin
      if EndPos <= Length(Value) then
        Delete(Result, StartPos, EndPos - StartPos + 1)
      else if StartPos > 1 then
        Delete(Result, StartPos - 1, EndPos - StartPos + 1)
      else
        Result := '';
      Exit;
    end;
    StartPos := EndPos + 1;
  end;
end;

procedure AddToUserPath;
var
  Entry, Value, NewValue: String;
begin
  Entry := ExpandConstant('{app}');
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Value) then
    Value := '';
  if ContainsPath(Value, Entry) then
    Exit;
  NewValue := Entry;
  if Value <> '' then
    NewValue := Entry + ';' + Value;
  if not RegWriteStringValue(HKCU, OwnerKey, 'PathAdded', Entry) then
    RaiseException('Could not record DynaR PATH installation.');
  if not RegWriteExpandStringValue(HKCU, EnvKey, 'Path', NewValue) then
  begin
    RegDeleteValue(HKCU, OwnerKey, 'PathAdded');
    RaiseException('Could not update your user PATH. DynaR is installed in ' + Entry);
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    AddToUserPath;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Entry, Value, Owned, Updated: String;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  Entry := ExpandConstant('{app}');
  if RegQueryStringValue(HKCU, OwnerKey, 'PathAdded', Owned) and SamePath(Owned, Entry) then
  begin
    if RegQueryStringValue(HKCU, EnvKey, 'Path', Value) then
    begin
      Updated := RemoveOwnedPath(Value, Entry);
      if Updated <> Value then
        if not RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Updated) then
        begin
          Log('Unable to remove DynaR entry from user PATH.');
          Exit;
        end;
    end;
    RegDeleteValue(HKCU, OwnerKey, 'PathAdded');
    RegDeleteKeyIfEmpty(HKCU, OwnerKey);
  end;
end;

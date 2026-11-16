; WhisperGUI 0.6+ -- Inno Setup installer (CPU and CUDA).
; Run from the repository root, after extracting the CPU or CUDA portable ZIP.
; Example:
;   ISCC.exe /DPackageDir="C:\build\package-cpu" /DVariant=cpu /DAppVersion=0.6.28 installer\WhisperGUI.iss
; No models are included in this installer.

#ifndef AppVersion
  #define AppVersion "0.6.28"
#endif
#ifndef Variant
  #define Variant "cpu"
#endif
#ifndef PackageDir
  #define PackageDir "..\package-cpu"
#endif

#define AppName "WhisperGUI"
#define AppExe "WhisperGUI.exe"

[Setup]
; Deliberately different from the legacy 0.5 AppId.
AppId={{E743AB91-EF79-4F1D-90B5-6F92FA96704F}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=DIT.Lab
DefaultDirName={localappdata}\Programs\WhisperGUI
DefaultGroupName=WhisperGUI
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
OutputDir=Output
OutputBaseFilename=WhisperGUI-{#AppVersion}-windows-x64-{#Variant}-setup
UninstallDisplayIcon={app}\{#AppExe}
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "italian"; MessagesFile: "compiler:Languages\Italian.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#PackageDir}\WhisperGUI.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\bin\*"; DestDir: "{app}\bin"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#PackageDir}\LICENSE-WhisperGUI.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\RUNTIME-VERSIONS.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\THIRD-PARTY-NOTICES.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\WhisperGUI"; Filename: "{app}\{#AppExe}"
Name: "{autodesktop}\WhisperGUI"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,WhisperGUI}"; Flags: nowait postinstall skipifsilent

[Code]
const
  LegacyKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\{1AC27330-D0BA-42B5-A1DE-5FF81CA0CAF0}_is1';
  CRLF = #13#10;
  FileAttributeDirectory = $10;
  FileAttributeReparsePoint = $400;
  GiB = 1073741824;

var
  LegacyDir: String;
  LegacyUninstaller: String;

function UninstallerFromCommand(const Cmd: String): String;
var
  S: String;
  N: Integer;
begin
  Result := '';
  S := Trim(Cmd);
  if S = '' then Exit;
  if S[1] = '"' then
  begin
    Delete(S, 1, 1);
    N := Pos('"', S);
    if N > 0 then Result := Copy(S, 1, N - 1);
  end
  else
  begin
    { Inno's uninstall command normally quotes its executable. }
    N := Pos('.exe', Lowercase(S));
    if N > 0 then Result := Copy(S, 1, N + 3);
  end;
end;

function SafeLegacyPath(const Dir, Uninstaller: String): Boolean;
var
  P, FileName: String;
begin
  Result := False;
  P := RemoveBackslashUnlessRoot(Trim(Dir));
  if (P = '') or (Length(P) <= 3) then Exit;
  if (ExtractFileDir(P) = P) or (ExtractFileDir(P) = '') then Exit;
  if Lowercase(P) = Lowercase(RemoveBackslashUnlessRoot(ExpandConstant('{win}'))) then Exit;
  if Lowercase(P) = Lowercase(RemoveBackslashUnlessRoot(ExpandConstant('{commonpf}'))) then Exit;
  if Lowercase(P) = Lowercase(RemoveBackslashUnlessRoot(ExpandConstant('{userpf}'))) then Exit;
  if Lowercase(P) = Lowercase(RemoveBackslashUnlessRoot(ExpandConstant('{localappdata}'))) then Exit;
  if (GetEnv('USERPROFILE') <> '') and
    (Lowercase(P) = Lowercase(RemoveBackslashUnlessRoot(GetEnv('USERPROFILE')))) then Exit;
  FileName := Lowercase(ExtractFileName(Uninstaller));
  if Pos('unins', FileName) <> 1 then Exit;
  if Lowercase(ExtractFileDir(Uninstaller)) <> Lowercase(P) then Exit;
  if not FileExists(Uninstaller) then Exit;
  Result := True;
end;

function LocateLegacyAtRoot(Root: Integer): Boolean;
var
  Location, Command, Version, Uninstaller: String;
begin
  Result := False;
  if not RegQueryStringValue(Root, LegacyKey, 'UninstallString', Command) then Exit;
  Uninstaller := UninstallerFromCommand(Command);
  if not RegQueryStringValue(Root, LegacyKey, 'InstallLocation', Location) then
    Location := '';
  if Location = '' then Location := ExtractFileDir(Uninstaller);
  Location := RemoveBackslashUnlessRoot(Location);
  if not SafeLegacyPath(Location, Uninstaller) then
  begin
    Log('Legacy uninstall entry found but path validation failed: ' + Location);
    Exit;
  end;
  { Do not confuse a future unrelated entry with the original 0.5. }
  if RegQueryStringValue(Root, LegacyKey, 'DisplayVersion', Version) then
  begin
    if (Version <> '') and (Copy(Version, 1, 3) <> '0.5') then Exit;
  end;
  LegacyDir := Location;
  LegacyUninstaller := Uninstaller;
  Result := True;
end;

function FindLegacy: Boolean;
begin
  Result := LocateLegacyAtRoot(HKCU32);
  if not Result and IsWin64 then Result := LocateLegacyAtRoot(HKCU64);
  if not Result then Result := LocateLegacyAtRoot(HKLM32);
  if not Result and IsWin64 then Result := LocateLegacyAtRoot(HKLM64);
end;

function DirectoryBytes(const Dir: String): Int64;
var
  FindRec: TFindRec;
  Item: String;
begin
  Result := 0;
  if FindFirst(AddBackslash(Dir) + '*', FindRec) then
  begin
    try
      repeat
        if (FindRec.Name <> '.') and (FindRec.Name <> '..') then
        begin
          Item := AddBackslash(Dir) + FindRec.Name;
          { Do not follow junctions or other reparse points. }
          if (FindRec.Attributes and FileAttributeReparsePoint) = 0 then
          begin
            if (FindRec.Attributes and FileAttributeDirectory) <> 0 then
              Result := Result + DirectoryBytes(Item)
            else
              Result := Result + Int64(FindRec.SizeHigh) * 4294967296 + FindRec.SizeLow;
          end;
        end;
      until not FindNext(FindRec);
    finally
      FindClose(FindRec);
    end;
  end;
end;

function DirectoryHasEntries(const Dir: String): Boolean;
var
  R: TFindRec;
begin
  Result := False;
  if FindFirst(AddBackslash(Dir) + '*', R) then
  begin
    try
      repeat
        if (R.Name <> '.') and (R.Name <> '..') then
        begin
          Result := True;
          Break;
        end;
      until not FindNext(R);
    finally
      FindClose(R);
    end;
  end;
end;

function GiBLabel(Bytes: Int64): String;
var
  Whole, Fraction: Int64;
begin
  Whole := Bytes div GiB;
  Fraction := ((Bytes mod GiB) * 100 + (GiB div 2)) div GiB;
  if Fraction = 100 then
  begin
    Whole := Whole + 1;
    Fraction := 0;
  end;
  if Fraction < 10 then
    Result := IntToStr(Whole) + '.0' + IntToStr(Fraction)
  else
    Result := IntToStr(Whole) + '.' + IntToStr(Fraction);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Response, ExitCode, Retry: Integer;
  Text: String;
begin
  Result := '';
  if not FindLegacy then Exit;
  Text := 'E'' stata trovata un''installazione di WhisperGUI 0.5 in:' + CRLF +
    LegacyDir + CRLF + CRLF +
    'E'' caldamente consigliato rimuoverla, per liberare circa ' +
    GiBLabel(DirectoryBytes(LegacyDir)) + ' GiB di spazio su disco.' + CRLF + CRLF +
    'Procedere con la disinstallazione?';
  Response := MsgBox(Text, mbConfirmation, MB_YESNO or MB_DEFBUTTON1);
  if Response <> IDYES then
  begin
    { Prevent mixing the two incompatible application versions. }
    Result := 'La versione 0.5 non e'' stata rimossa. Installazione interrotta.';
    Exit;
  end;

  Log('Uninstalling legacy WhisperGUI: ' + LegacyUninstaller);
  if not Exec(LegacyUninstaller, '/SILENT /NORESTART', LegacyDir,
    SW_SHOWNORMAL, ewWaitUntilTerminated, ExitCode) then
  begin
    Result := 'Impossibile avviare il programma di disinstallazione della 0.5.';
    Exit;
  end;
  if ExitCode <> 0 then
  begin
    Result := 'La disinstallazione della 0.5 non e'' riuscita (codice ' +
      IntToStr(ExitCode) + ').';
    Exit;
  end;

  { The legacy uninstaller can finish removing its folder shortly after exit. }
  for Retry := 1 to 10 do
  begin
    if not DirExists(LegacyDir) or not DirectoryHasEntries(LegacyDir) then Break;
    Sleep(200);
  end;

  if DirExists(LegacyDir) and DirectoryHasEntries(LegacyDir) then
  begin
    Response := MsgBox('La vecchia versione di WhisperGUI e'' stata rimossa, ma la ' +
      'directory di installazione:' + CRLF + CRLF + LegacyDir + CRLF + CRLF +
      'non e'' vuota. Vuoi rimuoverla completamente, ' +
      'incluso tutto il suo contenuto?', mbConfirmation, MB_YESNO or MB_DEFBUTTON2);
    if Response = IDYES then
    begin
      { The directory was validated against the registry and the old
        uninstaller BEFORE the old uninstaller was run. It can now be gone. }
      if (Length(LegacyDir) <= 3) or
        (Lowercase(ExtractFileDir(LegacyUninstaller)) <> Lowercase(LegacyDir)) then
      begin
        Result := 'Percorso della vecchia installazione non sicuro: ' + LegacyDir;
        Exit;
      end;
      { The uninstaller may have removed the directory while the dialog was open. }
      if DirExists(LegacyDir) then
      begin
        if not DelTree(LegacyDir, True, True, True) and DirExists(LegacyDir) then
        begin
          Result := 'Impossibile eliminare completamente la directory: ' + LegacyDir;
          Exit;
        end;
      end;
    end
    else if DirExists(LegacyDir) and DirectoryHasEntries(LegacyDir) and
      (Lowercase(RemoveBackslashUnlessRoot(WizardDirValue)) = Lowercase(LegacyDir)) then
    begin
      Result := 'La cartella scelta per la nuova installazione coincide con quella ' +
        'vecchia e contiene ancora file. Scegli una cartella diversa o consenti la pulizia.';
      Exit;
    end;
  end;
end;
; clawdyX per-user installer (NSIS 3). Installs into
; %LOCALAPPDATA%\Programs\clawdyX for the current user only: no
; administrator prompt, nothing outside the user's profile.
;
;   makensis -DVERSION=1.0.0 -DSRC=bin\windows\clawdyX -DOUT=bin\clawdyX-1.0.0-windows-amd64-setup.exe installer.nsi
;
; Upgrades stop the worker first when the login entry runs this install's
; worker, and start it again afterwards. A worker registered from another
; folder (a .zip copy) is left alone. Silent: /S (the uninstaller keeps the
; settings when silent).

Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma

!ifndef VERSION
  !error "pass -DVERSION=x.y.z"
!endif
!ifndef SRC
  !error "pass -DSRC=<folder with clawdyX.exe and helpers\>"
!endif
!ifndef OUT
  !define OUT "clawdyX-${VERSION}-windows-amd64-setup.exe"
!endif

!define APP "clawdyX"
!define RUNKEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\clawdyX"
; The worker's login entry (clawdyx service start) and the app's own
; "Start with Windows" entry.
!define WORKERVALUE "local.clawdyx"
!define APPVALUE "clawdyx"

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

Name "${APP} ${VERSION}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\${APP}"
InstallDirRegKey HKCU "${UNINSTKEY}" "InstallLocation"
BrandingText "${APP} ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APP}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "FileDescription" "${APP} installer"
VIAddVersionKey "CompanyName" "${APP}"
VIAddVersionKey "LegalCopyright" "PolyForm Noncommercial License 1.0.0"

!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\clawdyX.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open clawdyX"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${SRC}\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Var Ours ; 1 when the worker's login entry runs this install's worker

; Contains: push the text, then the part; pops 1 when the text contains the
; part (ignoring case, like paths on Windows), else 0.
!macro Contains un
Function ${un}Contains
  Exch $R1
  Exch
  Exch $R0
  Push $R2
  Push $R3
  Push $R4
  StrLen $R2 $R1
  StrCpy $R3 0
  loop:
    StrCpy $R4 $R0 $R2 $R3
    StrCmp $R4 "" no
    StrCmp $R4 $R1 yes
    IntOp $R3 $R3 + 1
    Goto loop
  yes:
    StrCpy $R0 1
    Goto done
  no:
    StrCpy $R0 0
  done:
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd
!macroend
!insertmacro Contains ""
!insertmacro Contains "un."

; OursCheck sets $Ours from the worker's login entry.
!macro OursCheck un
  StrCpy $Ours 0
  ReadRegStr $0 HKCU "${RUNKEY}" "${WORKERVALUE}"
  ${If} $0 != ""
    Push $0
    Push "$INSTDIR\helpers\"
    Call ${un}Contains
    Pop $Ours
  ${EndIf}
!macroend

Section "Install"
  SetShellVarContext current
  !insertmacro OursCheck ""
  ${If} $Ours == 1
  ${AndIf} ${FileExists} "$INSTDIR\helpers\clawdyx.exe"
    DetailPrint "Stopping the background worker"
    nsExec::ExecToLog '"$INSTDIR\helpers\clawdyx.exe" service stop'
    Pop $0
  ${EndIf}

  SetOutPath "$INSTDIR"
  File "${SRC}\clawdyX.exe"
  File "${SRC}\LICENSE"
  File "${SRC}\THIRD_PARTY_NOTICES.md"
  File "${SRC}\README.md"
  SetOutPath "$INSTDIR\helpers"
  File "${SRC}\helpers\clawdyx.exe"
  File "${SRC}\helpers\clawdyxw.exe"
  File /nonfatal "${SRC}\helpers\clawdyx-camera.exe"
  SetOutPath "$INSTDIR"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortcut "$SMPROGRAMS\${APP}.lnk" "$INSTDIR\clawdyX.exe"

  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APP}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${APP}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\clawdyX.exe"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"

  ${If} $Ours == 1
    DetailPrint "Starting the background worker"
    nsExec::ExecToLog '"$INSTDIR\helpers\clawdyx.exe" service start'
    Pop $0
  ${EndIf}
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  !insertmacro OursCheck "un."
  ; A worker registered from another folder still uses the settings and
  ; the state, so they are only offered for deletion when there is none.
  StrCpy $2 0
  ReadRegStr $0 HKCU "${RUNKEY}" "${WORKERVALUE}"
  ${If} $0 == ""
  ${OrIf} $Ours == 1
    MessageBox MB_YESNO|MB_ICONQUESTION "Also delete clawdyX's settings, sessions, logs and the stored bot token?$\r$\n$\r$\nClaude Code and your projects are not touched." /SD IDNO IDNO keep
    StrCpy $2 1
  keep:
  ${EndIf}

  ${If} $Ours == 1
    ${If} $2 == 1
      nsExec::ExecToLog '"$INSTDIR\helpers\clawdyx.exe" service uninstall -purge-state'
    ${Else}
      nsExec::ExecToLog '"$INSTDIR\helpers\clawdyx.exe" service uninstall'
    ${EndIf}
    Pop $0
  ${EndIf}
  ${If} $2 == 1
    nsExec::ExecToLog '"$INSTDIR\helpers\clawdyx.exe" token delete'
    Pop $0
    RMDir /r "$APPDATA\clawdyx"
    RMDir /r "$LOCALAPPDATA\clawdyx"
  ${EndIf}

  ; The app's own "Start with Windows" entry, when it points here.
  ReadRegStr $0 HKCU "${RUNKEY}" "${APPVALUE}"
  Push $0
  Push "$INSTDIR\"
  Call un.Contains
  Pop $1
  ${If} $1 == 1
    DeleteRegValue HKCU "${RUNKEY}" "${APPVALUE}"
  ${EndIf}

  Delete "$SMPROGRAMS\${APP}.lnk"
  Delete "$INSTDIR\clawdyX.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.md"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\helpers\clawdyx.exe"
  Delete "$INSTDIR\helpers\clawdyxw.exe"
  Delete "$INSTDIR\helpers\clawdyx-camera.exe"
  RMDir "$INSTDIR\helpers"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "${UNINSTKEY}"
SectionEnd

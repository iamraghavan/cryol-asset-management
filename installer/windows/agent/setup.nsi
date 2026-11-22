Unicode true
ManifestDPIAware true
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"

!ifndef AGENT_EXE
  !error "Pass the absolute path with -DAGENT_EXE=..."
!endif
!ifndef OUTPUT_EXE
  !define OUTPUT_EXE "Cryol-Agent-Setup-x64.exe"
!endif
!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "0.1.0"
!endif
!ifndef FILE_VERSION
  !define FILE_VERSION "0.1.0.0"
!endif
!ifndef DEFAULT_SERVER
  !define DEFAULT_SERVER ""
!endif
!ifndef PRODUCT_PUBLISHER
  !define PRODUCT_PUBLISHER "Cryol Asset Management"
!endif

Name "Cryol Endpoint Inventory"
OutFile "${OUTPUT_EXE}"
Icon "cryol-agent.ico"
UninstallIcon "cryol-agent.ico"
VIProductVersion "${FILE_VERSION}"
VIAddVersionKey /LANG=1033 "ProductName" "Cryol Endpoint Inventory"
VIAddVersionKey /LANG=1033 "CompanyName" "${PRODUCT_PUBLISHER}"
VIAddVersionKey /LANG=1033 "FileDescription" "Cryol Endpoint Inventory Setup"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "OriginalFilename" "Cryol-Agent-Setup-x64.exe"

!define APPKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\CryolEndpointInventory"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_TITLE "Cryol agent setup complete"
!define MUI_FINISHPAGE_TEXT "Cryol Endpoint Inventory runs as a Windows service and starts automatically at boot.$\r$\n$\r$\nOn the server, open Endpoint agents and check Last scan to confirm the first report arrived."

!insertmacro MUI_PAGE_WELCOME
Page custom EnrollmentPage EnrollmentLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Var Server
Var Token
Var CA
Var Interval
Var ServerControl
Var TokenControl
Var CAControl
Var IntervalControl
Var RevealControl

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Cryol Endpoint Inventory requires 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64
  StrCpy $Server "${DEFAULT_SERVER}"
  StrCpy $Interval "5m"
FunctionEnd

Function EnrollmentPage
  !insertmacro MUI_HEADER_TEXT "Connect to Cryol" "Configure the server and enroll this Windows computer."
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 12u "Cryol server address (HTTPS)"
  Pop $0
  ${NSD_CreateText} 0 13u 100% 13u "$Server"
  Pop $ServerControl
  ${NSD_CreateLabel} 0 34u 100% 12u "One-use token from Endpoint agents > New enrollment"
  Pop $0
  ${NSD_CreatePassword} 0 47u 100% 13u "$Token"
  Pop $TokenControl
  ${NSD_CreateCheckbox} 0 63u 100% 12u "Show token"
  Pop $RevealControl
  ${NSD_OnClick} $RevealControl ToggleToken
  ${NSD_CreateLabel} 0 81u 100% 12u "Public CA certificate (optional, for a private server certificate)"
  Pop $0
  ${NSD_CreateFileRequest} 0 94u 79% 13u "$CA"
  Pop $CAControl
  ${NSD_CreateButton} 81% 94u 19% 13u "Browse..."
  Pop $0
  ${NSD_OnClick} $0 ChooseCA
  ${NSD_CreateLabel} 0 114u 100% 12u "Inventory interval, for example 5m or 1h"
  Pop $0
  ${NSD_CreateText} 0 127u 55% 13u "$Interval"
  Pop $IntervalControl
  nsDialogs::Show
FunctionEnd

Function ToggleToken
  Pop $0
  ${NSD_GetState} $RevealControl $0
  ${If} $0 == ${BST_CHECKED}
    SendMessage $TokenControl 0x00CC 0 0
  ${Else}
    SendMessage $TokenControl 0x00CC 42 0
  ${EndIf}
  StrCpy $0 $TokenControl
  System::Call 'user32::InvalidateRect(p r0, p 0, i 1)'
FunctionEnd

Function ChooseCA
  Pop $0
  nsDialogs::SelectFileDialog open "" "Certificates|*.pem;*.cer;*.crt|All files|*.*"
  Pop $0
  ${If} $0 != ""
    ${NSD_SetText} $CAControl "$0"
  ${EndIf}
FunctionEnd

Function EnrollmentLeave
  ${NSD_GetText} $ServerControl $Server
  ${NSD_GetText} $TokenControl $Token
  ${NSD_GetText} $CAControl $CA
  ${NSD_GetText} $IntervalControl $Interval
  StrCpy $0 $Server 8
  StrLen $1 $Token
  ${If} $0 != "https://"
  ${OrIf} $1 != 43
  ${OrIf} $Interval == ""
    MessageBox MB_ICONEXCLAMATION "Enter the HTTPS server address, the 43-character token, and a scan interval."
    Abort
  ${EndIf}
  ${If} $CA != ""
    IfFileExists "$CA" +3
    MessageBox MB_ICONEXCLAMATION "The selected certificate could not be found."
    Abort
  ${EndIf}
FunctionEnd

Section "Install Cryol Endpoint Inventory"
  SetOutPath "$PLUGINSDIR"
  File /oname=cryol-agent.exe "${AGENT_EXE}"

  ; Use the wizard's process environment for private configuration. NSIS keeps
  ; these values out of the process command line and service registration.
  StrCpy $3 $Server
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_SERVER", w r3) i.r0'
  ${If} $0 == 0
    Goto failed
  ${EndIf}
  StrCpy $3 $CA
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_CA", w r3) i.r0'
  ${If} $0 == 0
    Goto failed
  ${EndIf}
  StrCpy $3 $Interval
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_INTERVAL", w r3) i.r0'
  ${If} $0 == 0
    Goto failed
  ${EndIf}
  StrCpy $3 $Token
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_ENROLLMENT_TOKEN", w r3) i.r0'
  StrCpy $3 ""
  StrCpy $Token ""
  ${If} $0 == 0
    Goto failed
  ${EndIf}
  DetailPrint "Connecting and installing the Cryol service..."
  nsExec::ExecToStack /TIMEOUT=90000 '"$PLUGINSDIR\cryol-agent.exe" install-service -installer-input'
  Pop $1
  Pop $2
  Call ClearSetupEnvironment
  ${If} $1 != "0"
    MessageBox MB_ICONSTOP "Cryol setup could not finish.$\r$\n$\r$\n$2"
    SetErrorLevel 1
    Abort
  ${EndIf}

  StrCpy $INSTDIR "$PROGRAMFILES64\Cryol Agent"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${APPKEY}" "DisplayName" "Cryol Endpoint Inventory"
  WriteRegStr HKLM "${APPKEY}" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr HKLM "${APPKEY}" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKLM "${APPKEY}" "DisplayIcon" "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "${APPKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${APPKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKLM "${APPKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${APPKEY}" "NoRepair" 1
  Goto done

  failed:
    Call ClearSetupEnvironment
    MessageBox MB_ICONSTOP "Cryol setup could not initialize enrollment."
    SetErrorLevel 1
    Abort
  done:
SectionEnd

Function ClearSetupEnvironment
  StrCpy $Token ""
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_ENROLLMENT_TOKEN", p 0)'
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_SERVER", p 0)'
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_CA", p 0)'
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_INTERVAL", p 0)'
FunctionEnd

Section "Uninstall"
  SetRegView 64
  nsExec::ExecToStack /TIMEOUT=45000 '"$INSTDIR\cryol-agent.exe" uninstall-service'
  Pop $0
  Pop $1
  ${If} $0 != "0"
    MessageBox MB_ICONSTOP "Could not remove the Cryol service.$\r$\n$\r$\n$1"
    SetErrorLevel 1
    Abort
  ${EndIf}
  DeleteRegKey HKLM "${APPKEY}"
  Delete "$INSTDIR\Uninstall.exe"
  DetailPrint "Agent state and server history were preserved."
SectionEnd

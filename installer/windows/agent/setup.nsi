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
  !define PRODUCT_VERSION "0.3.2"
!endif
!ifndef FILE_VERSION
  !define FILE_VERSION "0.3.2.0"
!endif
!ifndef DEFAULT_SERVER
  !define DEFAULT_SERVER ""
!endif
!ifndef PRODUCT_PUBLISHER
  !define PRODUCT_PUBLISHER "Raghavan - Cryol"
!endif

Name "Cryol Endpoint Inventory"
OutFile "${OUTPUT_EXE}"
Icon "cryol-agent.ico"
UninstallIcon "cryol-agent.ico"
VIProductVersion "${FILE_VERSION}"
VIAddVersionKey /LANG=1033 "ProductName" "Cryol Endpoint Inventory"
VIAddVersionKey /LANG=1033 "CompanyName" "${PRODUCT_PUBLISHER}"
VIAddVersionKey /LANG=1033 "FileDescription" "Cryol Endpoint Inventory Setup"
VIAddVersionKey /LANG=1033 "Comments" "Developer: ${PRODUCT_PUBLISHER}"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "OriginalFilename" "Cryol-Agent-Setup-x64.exe"

!define APPKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\CryolEndpointInventory"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Cryol Endpoint Inventory"
!define MUI_WELCOMEPAGE_TEXT "Developer: ${PRODUCT_PUBLISHER}$\r$\n$\r$\nSet up an endpoint or repair an existing Cryol service. Cryol collects authorized hardware, installed-software and network inventory."
!define MUI_FINISHPAGE_TITLE "Cryol agent setup complete"
!define MUI_FINISHPAGE_TEXT "Cryol Endpoint Inventory runs as a Windows service and starts automatically at boot.$\r$\n$\r$\nCryol accepted an inventory report. Open Endpoint agents and check Last scan to view the result."

!insertmacro MUI_PAGE_WELCOME
Page custom OperationPage OperationLeave
Page custom EnrollmentPage EnrollmentLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Var Server
Var Mode
Var HasService
Var HasPreviousFiles
Var ArchivePrevious
Var ArchiveControl
Var InstallRadio
Var RepairRadio
Var Token
Var CA
Var Interval
Var ServerControl
Var TokenControl
Var CAControl
Var IntervalControl
Var RevealControl
Var BundledCA

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Cryol Endpoint Inventory requires 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64
  StrCpy $Server "${DEFAULT_SERVER}"
  StrCpy $Interval "5m"
  StrCpy $BundledCA "0"
  StrCpy $Mode "install"
  StrCpy $HasService "0"
  ClearErrors
  ReadRegStr $0 HKLM "SYSTEM\CurrentControlSet\Services\CryolAgent" "ImagePath"
  ${If} $0 != ""
    StrCpy $HasService "1"
    StrCpy $Mode "repair"
  ${EndIf}
  StrCpy $HasPreviousFiles "0"
  StrCpy $ArchivePrevious "0"
  StrCpy $3 "$PROGRAMFILES64\Cryol Agent"
  System::Call 'Kernel32::GetFileAttributesW(w r3) i.r0'
  ${If} $HasService == "0"
  ${AndIf} $0 != -1
    StrCpy $HasPreviousFiles "1"
  ${EndIf}
  ClearErrors
FunctionEnd

Function OperationPage
  !insertmacro MUI_HEADER_TEXT "Choose a setup action" "Install a new endpoint or repair the Windows service."
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateRadioButton} 0 4u 100% 14u "Install and enroll this endpoint"
  Pop $InstallRadio
  ${NSD_CreateLabel} 12u 22u 95% 22u "Create the Cryol service and connect it to your organization's HTTPS server."
  Pop $0
  ${NSD_CreateRadioButton} 0 51u 100% 14u "Repair existing Cryol service"
  Pop $RepairRadio
  ${NSD_CreateLabel} 12u 70u 95% 34u "Update the executable, restore Automatic startup, and confirm inventory upload. The existing enrollment is kept."
  Pop $0
  ${If} $HasService == "0"
    EnableWindow $RepairRadio 0
    ${If} $HasPreviousFiles == "1"
      ${NSD_CreateCheckbox} 0 108u 100% 14u "Archive previous local files and enroll"
      Pop $ArchiveControl
      ${If} $ArchivePrevious == "1"
        ${NSD_Check} $ArchiveControl
      ${EndIf}
      ${NSD_CreateLabel} 12u 124u 95% 13u "The existing folder will be moved to a protected backup."
      Pop $0
    ${EndIf}
  ${Else}
    EnableWindow $InstallRadio 0
    ${NSD_CreateButton} 12u 108u 55% 15u "Check service status"
    Pop $0
    ${NSD_OnClick} $0 CheckInstalledService
  ${EndIf}
  ${If} $Mode == "repair"
    ${NSD_Check} $RepairRadio
  ${Else}
    ${NSD_Check} $InstallRadio
  ${EndIf}
  nsDialogs::Show
FunctionEnd

Function CheckInstalledService
  Pop $0
  Call StageSetupFiles
  nsExec::ExecToStack /TIMEOUT=15000 '"$PLUGINSDIR\cryol-agent.exe" status-service'
  Pop $1
  Pop $2
  ${If} $1 == "0"
    MessageBox MB_ICONINFORMATION "$2"
  ${Else}
    MessageBox MB_ICONEXCLAMATION "Windows service check failed.$\r$\n$\r$\n$2"
  ${EndIf}
FunctionEnd

Function OperationLeave
  ${NSD_GetState} $RepairRadio $0
  ${If} $0 == ${BST_CHECKED}
    StrCpy $Mode "repair"
    StrCpy $Token ""
  ${Else}
    StrCpy $Mode "install"
    ${If} $HasPreviousFiles == "1"
      ${NSD_GetState} $ArchiveControl $0
      ${If} $0 != ${BST_CHECKED}
        MessageBox MB_ICONEXCLAMATION "Cryol found files from a previous installation. Select the archive option to preserve them and continue."
        Abort
      ${EndIf}
      StrCpy $ArchivePrevious "1"
    ${EndIf}
  ${EndIf}
FunctionEnd

Function EnrollmentPage
  ${If} $Mode == "repair"
    Abort
  ${EndIf}
  !insertmacro MUI_HEADER_TEXT "Connect to Cryol" "Configure the server and enroll this Windows computer."
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 12u "Cryol server address (HTTPS)"
  Pop $0
  ${NSD_CreateText} 0 13u 70% 13u "$Server"
  Pop $ServerControl
  ${NSD_CreateButton} 72% 13u 28% 13u "Test connection"
  Pop $0
  ${NSD_OnClick} $0 TestConnection
  ${NSD_CreateLabel} 0 34u 100% 12u "One-use token from Endpoint agents > Generate token"
  Pop $0
  ${NSD_CreatePassword} 0 47u 100% 13u "$Token"
  Pop $TokenControl
  ${NSD_CreateCheckbox} 0 63u 100% 12u "Show token"
  Pop $RevealControl
  ${NSD_OnClick} $RevealControl ToggleToken
  !ifdef PUBLIC_CA_PEM
    StrCpy $CA "$PLUGINSDIR\cryol-ca.pem"
    StrCpy $BundledCA "1"
    ${NSD_CreateLabel} 0 86u 100% 22u "This setup includes the Cryol public CA. Windows trust settings stay the same."
    Pop $0
  !else
    ${NSD_CreateLabel} 0 81u 100% 12u "Public CA certificate (PEM, only for private HTTPS)"
    Pop $0
    ${NSD_CreateFileRequest} 0 94u 79% 13u "$CA"
    Pop $CAControl
    ${NSD_CreateButton} 81% 94u 19% 13u "Browse..."
    Pop $0
    ${NSD_OnClick} $0 ChooseCA
  !endif
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

Function StageSetupFiles
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=cryol-agent.exe "${AGENT_EXE}"
  !ifdef PUBLIC_CA_PEM
    File /oname=cryol-ca.pem "${PUBLIC_CA_PEM}"
  !endif
FunctionEnd

Function TestConnection
  Pop $0
  ${NSD_GetText} $ServerControl $Server
  ${If} $BundledCA == "0"
    ${NSD_GetText} $CAControl $CA
  ${EndIf}
  Call StageSetupFiles
  StrCpy $3 $Server
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_SERVER", w r3) i.r0'
  StrCpy $3 $CA
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_CA", w r3) i.r0'
  nsExec::ExecToStack /TIMEOUT=15000 '"$PLUGINSDIR\cryol-agent.exe" check-connection -wizard-input'
  Pop $1
  Pop $2
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_SERVER", p 0)'
  System::Call 'Kernel32::SetEnvironmentVariableW(w "CRYOL_SETUP_CA", p 0)'
  ${If} $1 == "0"
    MessageBox MB_ICONINFORMATION "$2"
  ${Else}
    MessageBox MB_ICONEXCLAMATION "Cryol connection failed.$\r$\n$\r$\n$2"
  ${EndIf}
FunctionEnd

Function EnrollmentLeave
  ${NSD_GetText} $ServerControl $Server
  ${NSD_GetText} $TokenControl $Token
  ${If} $BundledCA == "0"
    ${NSD_GetText} $CAControl $CA
  ${EndIf}
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
  ${AndIf} $BundledCA == "0"
    IfFileExists "$CA" +3
    MessageBox MB_ICONEXCLAMATION "The selected certificate could not be found."
    Abort
  ${EndIf}
FunctionEnd

Section "Install Cryol Endpoint Inventory"
  Call StageSetupFiles
  ${If} $Mode == "repair"
    DetailPrint "Checking the existing service and restoring Automatic startup..."
    nsExec::ExecToStack /TIMEOUT=110000 '"$PLUGINSDIR\cryol-agent.exe" repair-service'
    Pop $1
    Pop $2
    Goto checkresult
  ${EndIf}

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
  ${If} $ArchivePrevious == "1"
    nsExec::ExecToStack /TIMEOUT=150000 '"$PLUGINSDIR\cryol-agent.exe" install-service -installer-input -archive-existing'
  ${Else}
    nsExec::ExecToStack /TIMEOUT=150000 '"$PLUGINSDIR\cryol-agent.exe" install-service -installer-input'
  ${EndIf}
  Pop $1
  Pop $2
  Call ClearSetupEnvironment
  checkresult:
  ${If} $1 != "0"
    MessageBox MB_ICONSTOP "Cryol setup could not finish.$\r$\n$\r$\n$2"
    SetErrorLevel 1
    Abort
  ${EndIf}

  DetailPrint "$2"
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

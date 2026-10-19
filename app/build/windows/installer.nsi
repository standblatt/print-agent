; Windows installer of Standblatt Print Agent: per user without admin rights, start menu and desktop shortcut,
; autostart. Built by `make app-installer`, which defines VERSION, EXE, BOOTSTRAPPER and OUT.

Unicode true
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "LogicLib.nsh"

!define NAME "Standblatt Print Agent"
!define APP_EXE "standblatt-print-agent.exe"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
; Wails' autostart value name (the slug of the app name); the app finds and switches it by the exe path.
!define RUN_VALUE "standblatt-print-agent"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\StandblattPrintAgent"
; The WebView2 runtime is installed when one of these keys has a version.
!define WEBVIEW2_KEY "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Name "${NAME}"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\${NAME}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=0 "ProductName" "${NAME}"
VIAddVersionKey /LANG=0 "CompanyName" "Standblatt"
VIAddVersionKey /LANG=0 "FileDescription" "${NAME} Setup"
VIAddVersionKey /LANG=0 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "LegalCopyright" "AGPL-3.0"

!define MUI_ICON "icon.ico"
!define MUI_UNICON "icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "$(StartApp)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; NSIS picks the language of Windows, English otherwise.
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "German"
!insertmacro MUI_LANGUAGE "French"
!insertmacro MUI_LANGUAGE "Italian"

LangString StartApp ${LANG_ENGLISH} "Start ${NAME}"
LangString StartApp ${LANG_GERMAN} "${NAME} starten"
LangString StartApp ${LANG_FRENCH} "Démarrer ${NAME}"
LangString StartApp ${LANG_ITALIAN} "Avvia ${NAME}"
LangString InstallWebView2 ${LANG_ENGLISH} "Installing the WebView2 runtime"
LangString InstallWebView2 ${LANG_GERMAN} "WebView2-Laufzeit wird installiert"
LangString InstallWebView2 ${LANG_FRENCH} "Installation du runtime WebView2"
LangString InstallWebView2 ${LANG_ITALIAN} "Installazione del runtime WebView2"

; Stops the app, installed or started from a download, so its exe can be replaced or removed.
!macro StopApp
	nsExec::Exec 'taskkill /F /IM ${APP_EXE}'
	Pop $0
	nsExec::Exec 'taskkill /F /IM standblatt-print-agent-app-windows-amd64.exe'
	Pop $0
	Sleep 1000
!macroend

Section "Install"
	!insertmacro StopApp

	SetRegView 64
	ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\${WEBVIEW2_KEY}" "pv"
	${If} $0 == ""
		ReadRegStr $0 HKCU "Software\${WEBVIEW2_KEY}" "pv"
	${EndIf}
	${If} $0 == ""
		DetailPrint "$(InstallWebView2)"
		InitPluginsDir
		File "/oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" "${BOOTSTRAPPER}"
		ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
	${EndIf}

	SetOutPath "$INSTDIR"
	File "/oname=${APP_EXE}" "${EXE}"
	WriteUninstaller "$INSTDIR\uninstall.exe"

	CreateShortcut "$SMPROGRAMS\${NAME}.lnk" "$INSTDIR\${APP_EXE}"
	CreateShortcut "$DESKTOP\${NAME}.lnk" "$INSTDIR\${APP_EXE}"

	WriteRegStr HKCU "${RUN_KEY}" "${RUN_VALUE}" '"$INSTDIR\${APP_EXE}" -hidden'

	WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${NAME}"
	WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
	WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "Standblatt"
	WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
	WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
	WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
	WriteRegStr HKCU "${UNINSTALL_KEY}" "URLInfoAbout" "https://github.com/standblatt/print-agent"
	WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
	WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
	${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
	WriteRegDWORD HKCU "${UNINSTALL_KEY}" "EstimatedSize" $0
SectionEnd

; Removes the program, its shortcuts and its autostart; the config (with the tokens) and the log stay.
Section "Uninstall"
	!insertmacro StopApp
	Delete "$INSTDIR\${APP_EXE}"
	Delete "$INSTDIR\uninstall.exe"
	RMDir "$INSTDIR"
	Delete "$SMPROGRAMS\${NAME}.lnk"
	Delete "$DESKTOP\${NAME}.lnk"
	DeleteRegValue HKCU "${RUN_KEY}" "${RUN_VALUE}"
	DeleteRegKey HKCU "${UNINSTALL_KEY}"
SectionEnd

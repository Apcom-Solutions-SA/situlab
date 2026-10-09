; Installeur SituLab (Windows) — NSIS
; Compilation : makensis build\installer.nsi  (depuis la racine du dépôt, après `go build`)
; Produit : build\SituLab-Setup.exe

!define APP "SituLab"
!define VERSION "1.8.2"
!define PUBLISHER "APCOM Solutions SA"
!define EXE "situlab.exe"

Name "${APP} ${VERSION}"
OutFile "SituLab-Setup.exe"
InstallDir "$PROGRAMFILES64\${APP}"
InstallDirRegKey HKLM "Software\${APP}" "InstallDir"
RequestExecutionLevel admin
Unicode true
SetCompressor /SOLID lzma

!include "MUI2.nsh"
!define MUI_ICON "windows\icon.ico"
!define MUI_UNICON "windows\icon.ico"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "French"

Section "SituLab"
  SetOutPath "$INSTDIR"
  File "bin\${EXE}"
  File "windows\icon.ico"

  ; Raccourcis
  CreateDirectory "$SMPROGRAMS\${APP}"
  CreateShortcut "$SMPROGRAMS\${APP}\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\icon.ico"
  CreateShortcut "$DESKTOP\${APP}.lnk" "$INSTDIR\${EXE}" "" "$INSTDIR\icon.ico"

  ; Désinstalleur + entrée "Programmes et fonctionnalités"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\${APP}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}" "DisplayName" "${APP}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}" "DisplayIcon" "$INSTDIR\icon.ico"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}" "UninstallString" "$INSTDIR\Uninstall.exe"

  ; Demande au shell de rafraîchir ses icônes (SHCNE_ASSOCCHANGED) : évite qu'une (ré)installation
  ; au même emplacement laisse une vignette de raccourci périmée dans le cache d'icônes de Windows.
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, i 0, i 0)'
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APP}\${APP}.lnk"
  RMDir "$SMPROGRAMS\${APP}"
  Delete "$DESKTOP\${APP}.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}"
  DeleteRegKey HKLM "Software\${APP}"

  ; Désinstallation avancée : par défaut, on CONSERVE les données utilisateur (comme tout
  ; installeur Windows), pour qu'une réinstallation retrouve les réglages. On propose toutefois
  ; de les effacer — utile pour céder le poste ou repartir à blanc.
  ; SetShellVarContext current : viser le Roaming de l'utilisateur courant (poste personnel).
  ; Note : la suppression du jeton résiduel du coffre Windows se fait, elle, via « situlab purge »
  ; (le Go sait énumérer le coffre, pas NSIS).
  SetShellVarContext current
  ; MB_DEFBUTTON2 : le bouton par défaut est NON (conserver) ; /SD IDNO : en mode silencieux aussi,
  ; on CONSERVE. Effacer les données reste un choix explicite de l'utilisateur.
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Supprimer aussi vos données SituLab (connexions, trousseau chiffré, réglages, cache) ?$\n$\nNon = les conserver pour une réinstallation ultérieure." /SD IDNO IDNO skipUserData
    RMDir /r "$APPDATA\${APP}"
    RMDir /r "$APPDATA\${EXE}"
  skipUserData:
SectionEnd

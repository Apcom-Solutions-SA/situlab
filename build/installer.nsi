; Installeur SituLab (Windows) — NSIS
; Compilation : makensis build\installer.nsi  (depuis la racine du dépôt, après `go build`)
; Produit : build\SituLab-Setup.exe

!define APP "SituLab"
!define VERSION "1.15.0"
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
!include "WinMessages.nsh"
!include "LogicLib.nsh"
!include "WordFunc.nsh"
!include "nsDialogs.nsh"
!insertmacro un.WordReplace

; Primo-paramètres de modules (v1.13) : cases cochées par DÉFAUT ; l'utilisateur peut en réduire.
; Les choix sont écrits dans $INSTDIR\situlab-modules.json, consommé par l'app au 1er lancement.
Var MOD_AGENTS
Var MOD_GITADV
Var MOD_SPOKES
Var MOD_CORPUS
Var MOD_BACKUP

!define MUI_ICON "windows\icon.ico"
!define MUI_UNICON "windows\icon.ico"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
Page custom ModulesPageCreate ModulesPageLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "French"

; --- Page « Modules » : quelles fonctions activer (tout coché = plein contrôle) ---
Function ModulesPageCreate
  !insertmacro MUI_HEADER_TEXT "Modules" "Choisissez les fonctions à activer (tout est coché par défaut)."
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 24u "Vous pourrez ajuster ces modules plus tard dans Réglages > Modules."
  Pop $1
  ${NSD_CreateCheckbox} 0 28u 100% 11u "Accès IA (agents) — Assistant IA, bibliotheque de prompts, CLI/MCP"
  Pop $MOD_AGENTS
  ${NSD_Check} $MOD_AGENTS
  ${NSD_CreateCheckbox} 0 42u 100% 11u "Git avance — etapes separees, matrice des branches, outils git locaux"
  Pop $MOD_GITADV
  ${NSD_Check} $MOD_GITADV
  ${NSD_CreateCheckbox} 0 56u 100% 11u "SPOKES — projections de mission"
  Pop $MOD_SPOKES
  ${NSD_Check} $MOD_SPOKES
  ${NSD_CreateCheckbox} 0 70u 100% 11u "CORPUS — clouds de fichiers souverains"
  Pop $MOD_CORPUS
  ${NSD_Check} $MOD_CORPUS
  ${NSD_CreateCheckbox} 0 84u 100% 11u "BACKUP — sauvegardes git vers cloud"
  Pop $MOD_BACKUP
  ${NSD_Check} $MOD_BACKUP
  nsDialogs::Show
FunctionEnd

Function ModulesPageLeave
  ${NSD_GetState} $MOD_AGENTS $MOD_AGENTS
  ${NSD_GetState} $MOD_GITADV $MOD_GITADV
  ${NSD_GetState} $MOD_SPOKES $MOD_SPOKES
  ${NSD_GetState} $MOD_CORPUS $MOD_CORPUS
  ${NSD_GetState} $MOD_BACKUP $MOD_BACKUP
FunctionEnd

; BoolStr : pile [etat 0/1] -> remplace par "true"/"false"
Function BoolStr
  Exch $0
  ${If} $0 == 0
    StrCpy $0 "false"
  ${Else}
    StrCpy $0 "true"
  ${EndIf}
  Exch $0
FunctionEnd

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

  ; PATH : rendre `situlab` appelable depuis n'importe quel terminal. On vise le PATH UTILISATEUR
  ; (HKCU, court) et non le PATH système : NSIS a une longueur de chaîne bornée (~1024) et réécrire
  ; un PATH système long le tronquerait. Ajout idempotent.
  Push "$INSTDIR"
  Call AddToUserPath

  ; Primo-paramètres de modules → seed JSON consommé par l'app au 1er lancement.
  Push $MOD_AGENTS
  Call BoolStr
  Pop $MOD_AGENTS
  Push $MOD_GITADV
  Call BoolStr
  Pop $MOD_GITADV
  Push $MOD_SPOKES
  Call BoolStr
  Pop $MOD_SPOKES
  Push $MOD_CORPUS
  Call BoolStr
  Pop $MOD_CORPUS
  Push $MOD_BACKUP
  Call BoolStr
  Pop $MOD_BACKUP
  FileOpen $9 "$INSTDIR\situlab-modules.json" w
  ${If} $9 != ""
    FileWrite $9 '{"agents":$MOD_AGENTS,"git_advanced":$MOD_GITADV,"spokes":$MOD_SPOKES,"corpus":$MOD_CORPUS,"backup":$MOD_BACKUP}'
    FileClose $9
  ${EndIf}

  ; Demande au shell de rafraîchir ses icônes (SHCNE_ASSOCCHANGED) : évite qu'une (ré)installation
  ; au même emplacement laisse une vignette de raccourci périmée dans le cache d'icônes de Windows.
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, i 0, i 0)'
SectionEnd

; AddToUserPath : ajoute le dossier [sommet de pile] au PATH de l'utilisateur (HKCU\Environment),
; seulement s'il n'y est pas déjà. Broadcast WM_WININICHANGE pour que les nouveaux process le voient.
Function AddToUserPath
  Exch $0  ; dossier à ajouter
  Push $1
  Push $2
  ReadRegStr $1 HKCU "Environment" "Path"
  ; Garde anti-troncature : un PATH utilisateur proche de la limite NSIS n'est pas réécrit.
  StrLen $2 $1
  ${If} $2 > 1000
    DetailPrint "PATH utilisateur trop long : ajout de $0 ignoré (à ajouter à la main)."
    Goto done
  ${EndIf}
  ; Déjà présent ? recherche de «;$0;» dans «;$1;» (insensible à la casse via StrCmp).
  Push ";$1;"
  Push ";$0;"
  Call StrContainsCI
  Pop $2
  StrCmp $2 "" 0 done
  ${If} $1 == ""
    WriteRegExpandStr HKCU "Environment" "Path" "$0"
  ${Else}
    WriteRegExpandStr HKCU "Environment" "Path" "$1;$0"
  ${EndIf}
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
  DetailPrint "Ajouté au PATH utilisateur : $0"
done:
  Pop $2
  Pop $1
  Pop $0
FunctionEnd

; StrContainsCI : pile [haystack, needle] -> pile [needle si trouvé (casse ignorée), sinon ""].
Function StrContainsCI
  Exch $R0 ; needle
  Exch
  Exch $R1 ; haystack
  Push $R2
  Push $R3
  Push $R4
  StrLen $R2 $R0
  StrCpy $R4 0
  loop:
    StrCpy $R3 $R1 $R2 $R4
    StrCmp $R3 "" notfound
    StrCmp $R3 $R0 found
    IntOp $R4 $R4 + 1
    Goto loop
  found:
    Goto done
  notfound:
    StrCpy $R0 ""
  done:
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Exch $R0
FunctionEnd

Section "Uninstall"
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\situlab-modules.json"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APP}\${APP}.lnk"
  RMDir "$SMPROGRAMS\${APP}"
  Delete "$DESKTOP\${APP}.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP}"
  DeleteRegKey HKLM "Software\${APP}"

  ; Retrait du PATH utilisateur (toutes les formes : «;dir», «dir;», «dir» seul).
  ReadRegStr $1 HKCU "Environment" "Path"
  ${un.WordReplace} "$1" ";$INSTDIR" "" "+" $1
  ${un.WordReplace} "$1" "$INSTDIR;" "" "+" $1
  ${un.WordReplace} "$1" "$INSTDIR" "" "+" $1
  WriteRegExpandStr HKCU "Environment" "Path" "$1"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000

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

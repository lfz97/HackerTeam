# AD / Windows Infra Services — Scanner Cheatsheet

## Kerberos — 88/tcp+udp
```bash
nmap -p88 --script=krb5-enum-users --script-args krb5-enum-users.realm="<DOMAIN>",userdb=users.txt <IP>
kerbrute userenum -d <DOMAIN> --dc <IP> users.txt       # user enum (pre-auth)
kerbrute bruteuser -d <DOMAIN> --dc <IP> passwords.txt <user>   # authorized
GetNPUsers.py <DOMAIN>/ -dc-ip <IP> -request            # AS-REP roast (pre-auth, safe)
GetUserSPNs.py <DOMAIN>/<user>:<pass> -dc-ip <IP> -request  # kerberoast (needs 1 valid cred)
```
Findings: users without pre-auth (AS-REP roastable), SPN accounts (kerberoastable),
MS14-068 precondition (old DC). Harvested hashes go to cracking, flag to Exploit.

## LDAP — 389/636/3268/3269/tcp
```bash
nmap -n -sV --script "ldap* and not brute" <IP>          # anonymous metadata
ldapsearch -x -H ldap://<IP> -D '' -w '' -b "DC=<dom>,DC=<tld>"    # null bind test
ldapsearch -x -H ldap://<IP> -b "" -s base "(objectclass=*)" "*" + # rootDSE
ldapdomaindump <IP> -u '<DOM>\<user>' -p '<pass>'       # full dump w/ creds
# netexec null-bind query:
netexec ldap <DC> -u '' -p '' --query "(sAMAccountName=*)"
```
Findings: **null/anonymous bind** (user & group dump), creds with broad read. Flag AD attacks
(kerberoast, ASREP) to Exploit. ldapsearch w/ TLS SNI bypass: `-H ldaps://host:636/`.

## MSRPC / Endpoint Mapper — 135/tcp, 593
```bash
rpcdump.py <IP> -p 135                                  # impacket: list RPC interfaces
nmap --script msrpc-enum -p 135 <IP>
msf> use auxiliary/scanner/dcerpc/endpoint_mapper
```
Notable interfaces: LSA (user enum), SAMR (user/group/policy enum), SRVSVC (shares/sessions),
DCOM. Password guessing via SAMR can trigger lockout — authorized + rate-limited only.

## NetBIOS — 137/udp,138/udp,139/tcp
```bash
nmblookup -A <IP>
nbtscan <IP>/30
sudo nmap -sU -sV -T4 --script nbstat.nse -p137 -Pn -n <IP>
```
Findings: NetBIOS name/domain/user list disclosure.

## IPMI — 623/udp
```bash
nmap -sU --script ipmi-version -p 623 <IP>
msf> use auxiliary/scanner/ipmi/ipmi_version
msf> use auxiliary/scanner/ipmi/ipmi_cipher_zero          # cipher 0 auth bypass
msf> use auxiliary/scanner/ipmi/ipmi_dumphashes           # RAKP hash retrieval
ipmitool -I lanplus -H <IP> -U '' -P '' user list         # anonymous auth check
```
Findings: **cipher-0 bypass**, **anonymous auth**, RAKP hash leak (crackable), Supermicro
cleartext password exposure. Default creds per vendor (iDRAC calvin, IMM PASSW0RD, etc).

## WinRM / OMI — see remote-access.md

## General AD notes
- Time sync matters for Kerberos (KRB_AP_ERR_SKEW if skewed).
- When NTLM disabled, use `-k` kerberos auth with impacket/netexec.
- impacket examples dir: /usr/share/doc/python3-impacket/examples/

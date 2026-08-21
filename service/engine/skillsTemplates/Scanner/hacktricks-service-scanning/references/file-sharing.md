# File-Sharing Services — Scanner Cheatsheet

## FTP — 21/tcp (22 implicit SFTP)
```bash
nc -vn <IP> 21
nmap -sV -p21 -sC <IP>                            # includes ftp-anon + ftp-bounce checks
nmap --script ftp-* -p 21 <IP>
```
- **Anonymous login** (top finding): user `anonymous` / pass `anonymous` (or blank, or
  `ftp:ftp`). Then `ls -a`, `binary`, download interesting files.
  `wget -m ftp://anonymous:anonymous@<IP>` pulls everything.
- Default creds: `anonymous:anonymous`, `ftp:ftp`, `admin:admin`.
- **FTP bounce** (port-scan internal hosts via vulnerable FTP):
  `nmap -p21 --script ftp-bounce <IP>` then
  `nmap -Pn -p 21,80 -b ftp:ftp@<FTP_IP> 127.0.0.1` / `-b ftp:ftp@<FTP_IP> <internal>/24`.
- Cleartext creds — flag weak-crypto; `openssl s_client -starttls ftp` if TLS.
- Known: vsftpd 2.3.4 backdoor (`:)` smiley) — version precondition, flag to Exploit.
- Wing FTP web client RCE via `\0`-truncated username Lua injection — if Wing FTP identified.

## SMB — 139/445/tcp
```bash
nbtscan -r 192.168.0.1/24                        # NetBIOS host discovery
enum4linux-ng -A <IP>                            # full enum (users, shares, policy, RID)
enum4linux-ng -A -u "<user>" -p "<pass>" <IP>
nmap --script "safe or smb-enum-*" -p 445 <IP>
rpcclient -U "" -N <IP>                          # null session; then: querydispinfo, enumdomusers, srvinfo
# impacket (installed):
/usr/share/doc/python3-impacket/examples/samrdump.py <IP>
/usr/share/doc/python3-impacket/examples/rpcdump.py <IP>
# nxc / crackmapexec if installed:
nxc smb <IP> --shares ; nxc smb <IP> --users ; nxc smb <IP> --pass-pol
smbclient -L //<IP> -N                            # list shares anonymously
smbmap -H <IP>                                    # share permissions
```
Findings:
- **Null/anonymous session** → user + share enumeration.
- **Guest access** or `IPC$` readable.
- **Writable shares** (esp. `NETLOGON`, public, profiles).
- Common creds: `guest:(blank)`, `Administrator:(blank)/password/admin`.
- Version-specific preconditions to flag: **EternalBlue MS17-010** (`nmap --script smb-vuln-ms17-010`),
  SMBGhost, SambaCry (CVE-2017-7494), ksmbd CVEs. `smb-protocols`/`smb-os-discovery` for version.

## NFS — 2049/tcp (+ 111 rpcbind, mountd)
```bash
nmap -sV --script nfs-ls,nfs-showmount,nfs-statfs -p 2049 <IP>
showmount -e <IP>                                 # list exports & allowed hosts
rpcinfo -p <IP> | grep -i nfs
# mount an open export read-only:
sudo mkdir /mnt/t && sudo mount -t nfs <IP>:/<export> /mnt/t -o ro
```
Findings: exports with `no_root_squash` (root privesc path) or `*` allowed hosts. Flag high.

## Rsync — 873/tcp
```bash
nmap -sV --script rsync-list-modules -p 873 <IP>
nc -vn <IP> 873 ; printf "\n" | nc -vn <IP> 873   # list modules banner
rsync --list-only rsync://<IP>/                   # anonymous module list
```
Findings: anonymous readable/writable modules. `rsync rsync://<IP>/<module>/ file` to read.

## AFP — 548/tcp (Apple Filing)
```bash
nmap -sV --script afp-ls,afp-path-vuln,afp-serverinfo,afp-showmount -p 548 <IP>
nmap -p 548 --script afp-brute <IP>              # authorized only
```
Findings: CVE-2018-1160 (Netatalk <3.1.12 pre-auth RCE) — version precondition, flag to Exploit.

## TFTP — 69/udp
```bash
nmap -n -Pn -sU -p69 -sV --script tftp-enum <IP>
nmap -n -Pn -sU -p69 --script tftp-enum --script-args tftp-enum.filelist=./tftp-files.txt <IP>
```
No auth by design; look for config/IOS image downloads. Path traversal possible on some daemons.

## iSCSI — 3260/tcp
```bash
nmap -sV --script=iscsi-info -p 3260 <IP>
nmap -sV --script iscsi-brute --script-args userdb=u.txt,passdb=p.txt -p 3260 <IP>
iscsiadm -m discovery -t sendtargets -p <IP>:3260   # enumerate targets (unauth?)
```
Findings: targets discoverable/login without CHAP auth = full disk access.

## GlusterFS — 24007-24009/49152/tcp
```bash
gluster --remote-host <IP> peer status
gluster --remote-host <IP> volume info all
sudo mount -t glusterfs <IP>:/<vol> /mnt/gluster   # if no auth
```
Flag CVE-2018-1088 / CVE-2022-48340 preconditions if version known.

## WebDAV — HTTP PUT/PROPFIND (see also web-tech-stack.md)
```bash
davtest -url http://<IP>                          # test upload+move to executable exts
davtest -auth user:pass -sendbd auto -url http://<IP>
cadaver http://<IP>                               # interactive
curl -T 'shell.txt' "http://<IP>/"                # raw PUT test
curl -X MOVE --header "Destination: http://<IP>/shell.php" "http://<IP>/shell.txt"
```
Findings: PUT allowed (especially to executable webroot), IIS5/6 WebDAV unicode bypass precondition.

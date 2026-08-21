---
name: hacktricks-service-scanning
description: >-
  Per-service network scanning playbook distilled from HackTricks. Use whenever you see
  an open port or service banner and need to know HOW to scan it — enumeration commands,
  anonymous/unauthenticated access checks, default credentials to test, nmap NSE scripts,
  and known-vulnerable version indicators. Covers 90+ services: SSH, FTP, SMB, Kerberos,
  LDAP, MSSQL, MySQL, PostgreSQL, Oracle, Redis, MongoDB, Memcached, Elasticsearch,
  CouchDB, SMTP, POP, IMAP, VNC, RDP, WinRM, SNMP, DNS, NTP, Docker, MQTT, AMQP,
  FastCGI, JDWP, IPMI, SIP/VoIP, RTSP, printers, ICS/Modbus, CMS/web tech stacks
  (WordPress, Tomcat, Spring, Grafana, Zabbix, SharePoint...), wfuzz/ffuf web fuzzing,
  login bypass payloads and per-service brute-force commands. ALWAYS consult this skill
  when a scan target exposes network services — even common ones like 22 or 3306 —
  instead of improvising commands from memory. Complements the pentest-tools skill:
  that one tells you WHERE tools are installed; this one tells you WHAT to run against
  each service.
---

# HackTricks Service Scanning Playbook — Scanner Agent

Distilled from a careful study of the full HackTricks `network-services-pentesting`
section (190+ pages), plus its brute-force cheatsheet, fuzzing methodology, WFuzz guide,
login-bypass lists, and nmap reference. This skill answers one question: **"Port X is
open — what do I run, in what order, and what findings am I looking for?"**

## How to use this skill

1. Match the open port/service against the routing table below.
2. Open the matching reference file and execute the enumeration steps top-to-bottom:
   **banner grab → safe nmap scripts → anonymous/unauthenticated checks → version &
   CVE preconditions → (only if Captain authorized) weak-credential test**.
3. Record raw output verbatim into the task's raw output directory. You scan; you do
   not verify or exploit. When a reference says "hand to Exploit", stop there and flag it.

## Scanner safety rules (non-negotiable)

- Rate-limit everything: hydra `-t 4..16`, ffuf/wfuzz `-rate`/`-t` throttles, nmap `-T3`
  max unless Captain says otherwise. Never `--risk=3`, never `--os-shell`, never
  intrusive/dos/exploit NSE categories, never destructive printer/PJL commands
  (`FSINIT`, `FSDELETE`, `FSDOWNLOAD` writes), never `FLUSH*` on Redis, never write
  operations on databases.
- Credential brute-forcing only with explicit Captain authorization; small wordlists;
  watch for lockout responses.
- Local tool reality: `crackmapexec` is NOT installed — use `nxc`/netexec if present,
  else impacket scripts (`/usr/share/doc/python3-impacket/examples/`) which ARE installed.
  Check the `pentest-tools` skill for exact paths before running anything exotic.

## Routing table — port/service → reference file

| Ports / service | Read |
|---|---|
| 21 FTP · 139/445 SMB · 2049 NFS · 873 rsync · 548 AFP · 69 TFTP · 3260 iSCSI · 24007 GlusterFS · WebDAV PUT | `references/file-sharing.md` |
| 22 SSH · 23 Telnet · 3389 RDP · 5900-5901 VNC · 5985/5986 WinRM/OMI · 6000 X11 · 512/513/514 rexec/rlogin/rsh · JDWP · remote gdbserver · 5555 ADB | `references/remote-access.md` |
| 3306 MySQL · 1433 MSSQL · 5432 PostgreSQL · 1521 Oracle · 6379 Redis · 27017 MongoDB · 11211 Memcached · 9042 Cassandra · 5984 CouchDB · 8086 InfluxDB · 9200 Elasticsearch · 5601 Kibana · 9001 HSQLDB · 50070/8088 Hadoop/YARN · 5439 Redshift | `references/databases.md` |
| 25/465/587 SMTP · 110/995 POP · 143/993 IMAP · 5060 SIP/VoIP | `references/mail-and-voip.md` |
| 88 Kerberos · 389/636/3268 LDAP · 135/593 MSRPC · 137-139 NetBIOS · 623 IPMI | `references/ad-and-windows.md` |
| 80/443/8080... web apps & CMS (WordPress, Drupal, Joomla, Moodle, Tomcat, JBoss, Spring, Grafana, Zabbix, SharePoint, AEM, Werkzeug/Flask, Django, Laravel, Git leak, Apache/Nginx misconfigs, PHP tricks) · 401/403 bypass | `references/web-tech-stack.md` |
| Web fuzzing tools: wfuzz/ffuf/gobuster/dirsearch recipes, login-bypass payloads, rate-limit & captcha bypass, directory BF methodology | `references/web-fuzzing.md` |
| 53 DNS · 123 NTP · 161 SNMP · 194/6667 IRC · 79 finger · 113 ident · 111 rpcbind · 1080 SOCKS · 2375/5000 Docker · 1883 MQTT · 5672/15672 AMQP/RabbitMQ · 8089 Splunk · 9000 FastCGI · 8333 Bitcoin · 502 Modbus · 47808 BACnet · 500/4500 IKE/IPsec · 1723 PPTP · 9100/631/515 printers · 1099 Java RMI · 4369 epmd · 3632 distcc · 3690 SVN · 5353 mDNS · 3128 Squid · 554 RTSP · 10000 ndmp · 44134 Tiller · misc | `references/misc-services.md` |
| Any brute-force job: per-service hydra/ncrack/medusa/legba/nmap-brute syntax, default-credential sources, wordlist generation | `references/brute-force.md` |

## Universal first-touch (any TCP service)

```bash
nc -vn <IP> <PORT>                          # raw banner
nmap -sV -sC -Pn -p <PORT> <IP>             # version + default safe scripts
nmap --script-help "<service>*"             # discover available NSE scripts
```

## Finding taxonomy to report

When writing findings, tag each with what the scanner actually showed:
`banner/version`, `anon-or-unauth-access`, `default-credentials` (only after authorized
test), `info-disclosure` (users, shares, config), `known-cve-precondition` (version in
vulnerable range — flag, do NOT exploit), `misconfiguration` (open relay, writable share,
exposed console), `weak-crypto` (export ciphers, no TLS). Everything else is Exploit's job.

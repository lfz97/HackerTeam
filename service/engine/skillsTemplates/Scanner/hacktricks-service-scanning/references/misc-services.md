# Misc/Other Services — Scanner Cheatsheet

## DNS — 53/tcp+udp
```bash
dig version.bind CHAOS TXT @<IP>                 # BIND version banner
dig any <domain> @<IP>
dig axfr @<IP>                                    # zone transfer attempt (top finding)
dig axfr <domain> @<IP> ; fierce --domain <domain> --dns-servers <IP>
dnsrecon -d <domain> -a -n <IP>                   # zone transfer + enum
dnsrecon -r 127.0.0.0/24 -n <IP>                  # reverse PTR brute-force (internal host discovery!)
dnsenum --dnsserver <IP> --enum -p 0 -s 0 -o subs.txt -f wordlist <domain>
dig -t _ldap._tcp.<dom> ; _kerberos._tcp.<dom> ; _gc._tcp.<dom>   # AD DC discovery
nmap -n --script "(default and *dns*) or fcrdns or dns-srv-enum" <IP>
```
Findings: zone transfer open (high), recursion open (amplification), version disclosure.

## NTP — 123/udp
```bash
ntpq -c rv <IP> ; ntpq -c peers <IP> ; ntpq -c readvar <IP>
ntpdc -c monlist <IP>                             # mode-7: last-600-clients list (info + amplification)
nmap -sU -p123 --script ntp-monlist <IP>
nmap -sU -sV --script "ntp* and (discovery or vuln) and not (dos or brute)" -p 123 <IP>
chronyc -a -n tracking -h <IP>                    # chrony targets
```
Findings: monlist enabled (info disclosure + DDoS amplifier), version vulns (CVE-2014-9293 pre-4.2.8 family).

## SNMP — 161/udp
```bash
onesixtyone -c community-strings.txt <IP>         # community brute (fast, quiet-ish)
nmap -sU --script snmp-brute <IP> [--script-args snmp-brute.communitiesdb=wl]
snmp-check <IP> -c public                         # full readable dump
snmpwalk -v2c -c public <IP> .1                   # everything
snmpwalk -v1 -c private <IP>
snmpbulkwalk -c public -v2c <IP> .
braa public@<IP>:.1.3.6.*                         # mass OID sweep
nmap -sU --script snmp-info,snmp-interfaces,snmp-processes,snmp-sysdescr,snmp-netstat <IP>
```
Key OIDs: sysDescr `1.3.6.1.2.1.1.1.0` (banner), users via `hrSWRunName` / Windows users
`1.3.6.1.4.1.77.1.2.25`. Findings: default community (`public/private`) = massive info disclosure;
writable community = config tamper (flag to Exploit).

## IRC — 194/6660-7000
```bash
nc -vn <IP> 6667 ; nmap -sV --script irc-botnet-channels,irc-info -p 6667 <IP>
msf> use auxiliary/scanner/irc/irc_login          # anon check
```
Look for: anonymous access, botnet C2 channels, old UnrealIRCd backdoor (3.2.8.1 — flag).

## Finger — 79/tcp
```bash
finger @<IP>                                      # user list
finger root@<IP> ; finger-user-enum.pl -U users.txt -t <IP>
printf '\r\n' | nc -vn <IP> 79                    # logged-in users
```
Some daemons allow `finger "|/bin/id@host"` command execution + relay (user@host@victim).

## Ident — 113/tcp
```bash
ident-user-enum <IP> 22 113 139 445               # service-owner usernames
nmap -p 22,113,445 --script auth-owners <IP>
```

## Portmapper/RPC — 111/tcp+udp
```bash
rpcinfo -p <IP>
nmap -sSUC -p111 <IP>
showmount -e <IP>                                   # NFS exports via rpcbind
```
NIS exposure: `ypcat -d <domain> -h <IP> passwd` may dump password hashes (flag).

## SOCKS — 1080/tcp
```bash
nmap -p 1080 --script socks-auth-info <IP>
nmap --script socks-brute -p 1080 <IP>              # authorized
nmap -sV --script socks-methods,socks-open-proxy -p 1080 <IP>
printf '\x05\x01\x00' | nc -nv <IP> 1080            # no-auth handshake?
curl --socks5-hostname <IP>:1080 https://ifconfig.me   # egress/open-proxy test
```

## Docker API — 2375/2376/tcp
```bash
curl http://<IP>:2375/version ; curl http://<IP>:2375/containers/json ; curl http://<IP>:2375/images/json
docker -H tcp://<IP>:2375 ps
```
Unauthenticated Docker API = **critical** finding (trivial container escape to host). FLAG to Exploit.

## Docker Registry — 5000/tcp
```bash
curl http://<IP>:5000/v2/_catalog
curl http://<IP>:5000/v2/<image>/tags/list
curl http://<IP>:5000/v2/<image>/manifests/<tag>
```
Findings: anonymous catalog = image/source/secret leak.

## MQTT — 1883/tcp (8883 tls)
```bash
mosquitto_sub -h <IP> -t '#' -v                   # anonymous subscribe to all topics
mosquitto_pub -h <IP> -t 'test' -m 'ping'         # ONLY if write-test authorized
nmap -p 1883 <IP> -sV
```
Findings: anonymous access to telemetry/commands. Topics may include credentials.

## AMQP/RabbitMQ — 5671/5672 ; Management — 15672
```bash
curl -u guest:guest http://<IP>:15672/api/overview    # DEFAULT CREDS guest:guest
curl -u guest:guest http://<IP>:15672/api/users
legba amqp --target <IP>:5672 --username guest --password wl.txt
```
Findings: guest:guest on mgmt UI (vhost `/` full control).

## Splunkd — 8089/tcp (+web 8000)
```bash
curl -k https://<IP>:8089/services/server/info      # version (auth?)
# web UI 8000 default creds admin:changeme (flag!)
```
With admin creds: RCE via custom app — hand to Exploit.

## FastCGI — 9000/tcp
```bash
nmap -sV -p 9000 <IP>                               # fastcgi?
# unauthenticated php-fpm = file read / code exec primitive:
# cgi-fcgi -connect <IP>:9000 (probe only) — FLAG to Exploit (SCRIPT_FILENAME technique)
```

## Bitcoin nodes — 8333/18333/38333/18444
```bash
nmap -p 8333 --script bitcoin-info,bitcoin-getaddr <IP>
```

## Modbus — 502/tcp (ICS!)
```bash
nmap -sV --script modbus-discover -p 502 <IP>       # safe mode only; NO aggressive write
msf> use auxiliary/scanner/scada/modbusdetect       # UID discovery
# pymodbus read_device_information — safe read
```
ICS caution: enumeration only; never write coils/registers without explicit scope.
No auth by default — flag the exposure.

## BACnet — 47808/udp (ICS building automation)
```bash
nmap --script bacnet-info -sU -p 47808 <IP>
```
## EthernetIP 44818, OPC-UA 4840: nmap enip-info / opc-ua-info style scripts; read-only.

## IKE/IPsec VPN — 500/udp, 4500/udp
```bash
nmap -sU -p 500 --script ike-version <IP>
ike-scan -M <IP>                                    # discover valid transforms
ike-scan -M --showbackoff <IP>                      # vendor fingerprint via backoff pattern
ike-scan -A <IP>                                    # aggressive mode? → PSK hash capture possible
ikeforce.py <IP> -e -w groupnames.dic               # group ID brute (authorized)
```
Findings: aggressive mode enabled (PSK crackable offline — hashcat -m 5300/5400), group ID
discovered. XAuth brute via ikeforce (authorized only).

## PPTP — 1723/tcp (+GRE proto 47)
```bash
nmap -Pn -sV --script pptp-version -p1723 <IP>
# MS-CHAPv2 handshake capture → offline crack (hashcat -m 5500, asleap, chapcrack)
```
Legacy path; MS-CHAPv2 crackable → flag.

## Printers
### PJL / raw — 9100/tcp
```bash
nc -vn <IP> 9100 ; @PJL INFO STATUS ; @PJL INFO ID ; @PJL INFO VARIABLES ; @PJL FSDIRLIST NAME="0:\" ENTRY=1 COUNT=65535
nmap -sV --script pjl-ready-message -p 9100 <IP>
# PRET toolkit: python2 pret.py <IP> pjl  (interactive)
```
Findings: filesystem access, env vars (may contain credentials), Canon TTF-VM CVE preconditions.
NEVER run FSINIT/FSDELETE/FSDOWNLOAD (destructive).
### IPP/CUPS — 631/tcp+udp
```bash
nmap -sV -p631 --script=cups-info,cups-queue-info <IP>
ippfind --timeout 3 --txt -v "@local and port=631"
```
Flag: cups-browsed UDP/631 chain (CVE-2024-47176/47076/47175/47177) — remote RCE precondition.
### LPD — 515/tcp
```bash
# PRET lpdtest.py hostname get /etc/passwd  — file read / cmd injection primitives (flag)
```

## Java RMI — 1099/tcp (+1050/1098)
```bash
nmap -p 1099 --script rmi-dumpregistry <IP>
msf> use auxiliary/scanner/misc/java_rmi_server     # unauth check
# barmie for registry enumeration
```
Unauthenticated RMI registry = deserialization RCE precondition — flag high/critical.

## Erlang epmd — 4369/tcp
```bash
nmap -p 4369 --script epmd-info <IP>
```
Exposed nodes + Erlang cookie leak = RCE (related: CouchDB, RabbitMQ clustering).

## distcc — 3632/tcp
```bash
nmap -sV -p 3632 <IP> ; nmap -p 3632 --script distcc-exec <IP>   # SAFE check only? — this NSE executes `id`; flag version instead
```
distcc = remote compile; CVE-2004-2687 command exec precondition. Flag.

## Subversion — 3690/tcp
```bash
svn info svn://<IP>/ ; svn list svn://<IP>/
```

## mDNS — 5353/udp
```bash
nmap -sU -p 5353 --script dns-service-discovery,dns-srv-enum <IP>
```

## Squid proxy — 3128/tcp
```bash
nmap -Pn -sV -p 3128 --script http-open-proxy <IP>
curl -x http://<IP>:3128 https://ifconfig.me       # open proxy egress check
curl http://<IP>:3128/squid-internal-mgr/info      # cache manager leak
curl http://<IP>:3128/squid-internal-mgr/config
# internal reachability via proxy:
curl -x http://<IP>:3128 http://127.0.0.1:8080/ ; curl -x http://<IP>:3128 http://169.254.169.254/latest/meta-data/
```
Findings: open proxy, cache manager config leak (ACLs/passwords), SSRF into internal.

## RTSP — 554/8554/tcp
```bash
nmap -sV --script "rtsp-*" -p 554 <IP>             # includes rtsp-methods, rtsp-url-brute
ffplay -rtsp_transport tcp rtsp://<IP>/<path>      # view stream if unauth
# camera default creds: admin:admin, admin:1234, admin:12345, root:root
```

## NDMP — 10000/tcp
```bash
nmap -sV -p 10000 --script ndmp-version <IP>       # backup protocol, often unauth
```

## Helm Tiller — 44134/tcp
```bash
helm --tiller-namespace kube-system version        # if reachable: unauth tiller = cluster RCE
```
Flag: exposed tiller (k8s pre-1.16) = cluster takeover path.

## rusersd — 1026/udp
```bash
rusers -l <IP>                                      # logged-in users list!
nmap -sU -p 1026 --script rusers <IP>
```

## WS-Discovery — 3702/udp
```bash
nmap -sU -p 3702 --script wsdd-discover <IP>
```
Leaks ONVIF camera/device URLs — pivot to camera default creds.

## AJP — 8009/tcp
```bash
nmap -p 8009 --script ajp-auth,ajp-headers <IP>
# Ghostcat CVE-2020-1938 (Tomcat AJP file read/incl) — flag version precondition to Exploit
nmap --script ajp-brute -p 8009 <IP>               # authorized
```

## SAP — 3299 (SAPRouter) / 3200-3299 range
```bash
nmap -sV -p 3299 <IP>                               # saprouter ACL probing (nmap sapdb-info)
```
SAP default creds: SAP*/06071992, SAP*/PASS, DDIC/19920706 (flag only with authorization).

## IBM MQ — 1414/tcp
```bash
nmap -sV -p 1414 <IP> ; mq client channel discovery (punchq/ibmwebsphere tools)
```

## Check Point — 264/tcp (FW-1) + SmartConsole
Flag CVE preconditions: CVE-2024-24919 (Quantum info disclosure), CVE-2026-16232 (SmartConsole
auth bypass). Version fingerprint via 264 topology queries.

## Compaq/HP Insight — 2301/2381
Web management; check default creds (administrator:administrator variants).

## Cameras — 32100/udp (P2P cameras), RTSP defaults
Vendor default creds (admin/admin, admin/12345...) — authorized brute only.

## Echo 7 / Whois 43 / TACACS+ 49 / EPP 700 / Cisco SD-WAN 12346
Low value; banner-grab + version note only. TACACS+ secret brute via thc-tacacs (authorized).

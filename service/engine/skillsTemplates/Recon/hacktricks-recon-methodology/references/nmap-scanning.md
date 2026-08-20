# Nmap / Masscan Scanning Reference

Fast host discovery → full port scan → service/OS fingerprint. Save outputs with `-oA`.

## Host discovery (from outside)

ICMP is often filtered; combine probes:

```bash
ping -c 1 199.66.11.4
fping -g 199.66.11.0/24
nmap -PE -PM -PP -sn -n 199.66.11.0/24    # echo, timestamp, subnet-mask requests
```

TCP discovery with masscan (top nmap ports of a /24 in <5 min):

```bash
masscan -p20,21-23,25,53,80,110,111,135,139,143,443,445,993,995,1723,3306,3389,5900,8080 199.66.11.0/24
```

HTTP-focused: `masscan -p80,443,8000-8100,8443 <range>`

UDP discovery (slow with nmap; use intensity 0 for most-probable probes):

```bash
nmap -sU -sV --version-intensity 0 -F -n 199.66.11.53/24
# faster: udp-proto-scanner.pl 199.66.11.53/24
```

SCTP: `nmap -T4 -sY -n --open -Pn <IP/range>`

## Port scanning

```bash
nmap -sV -sC -O -T4 -n -Pn -oA fastscan <IP>          # top 1000 TCP
nmap -sV -sC -O -T4 -n -Pn -p- -oA fullfastscan <IP>  # all ports, fast
nmap -sV -sC -O -p- -n -Pn -oA fullscan <IP>          # all ports, default timing
syn.scan 192.168.1.0/24 1 10000                       # bettercap
```

UDP options:

```bash
udp-proto-scanner.pl <IP>
nmap -sU -sV --version-intensity 0 -n -F -T4 <IP>     # top ~100 UDP services
nmap -sU -sV -sC -n -F -T4 <IP>                       # + default scripts
nmap -sU -sV --version-intensity 0 -n -T4 <IP>        # top 1000 UDP
```

SCTP scans: `nmap -T4 -sY -n -oA SCTFastScan <IP>`; all ports: add `-p- -sV -sC`.

## Scan technique flags

- `-sS` SYN stealth (default w/ root) — no full connection, less logging.
- `-sT` connect scan (default unprivileged) — leaves logs.
- `-sU` UDP. `-sY/-sZ` SCTP init/cookie-echo.
- `-sN -sX -sF` null/xmas/fin — bypass some firewalls; open|filtered=no response, closed=RST.
  Unreliable on Windows/Cisco/BSDI.
- `-sM` Maimon (FIN+ACK, BSD). `-sA/-sW` ACK/Window — map firewall rulesets (filtered vs unfiltered).
- `-sI <zombie>` idle scan (IPID-based anonymity); find zombies with ipidseq script.
- `--badsum` — firewalls may answer, hosts drop → firewall detection.
- `-sO` IP protocol scan. `-b <ftp server>` FTP bounce (mostly dead).

## Target & port selection

- Targets: `<ip>/<net>` directly, `-iL file`, `-iR <num>` random, `--exclude`, `--excludefile`.
- `-p-` all 65535; `-F` top 100; `--top-ports <n>`; `-r` disable random order;
  `U:53,T:21-25,80` protocol grouping; `--port-ratio <0-1>`.

## Version / OS / scripts

- `-sV` version; `--version-intensity 0-9` (default 7); `--version-light` (=2) for first pass on
  large/UDP ranges; `--version-all` (=9) for stubborn services; `--allports` to include excluded
  ports like 9100 (careful: may print on printers).
- `-O` OS; `--osscan-limit` skip if no open+closed port; `--osscan-guess` try harder.
- `-sC` or `--script=default`. Categories: auth, broadcast, default, discovery, dos, exploit,
  external, fuzzer, intrusive, malware, safe, version, vuln, all.
  - Search: `nmap --script-help="http-*"`, `"not intrusive"`, `"default or safe"`,
    `"(default or safe or intrusive) and not http-*"`.
  - Args: `--script-args n1=v1,n2={n3=v3}`; `--script-args-file`; `--script-trace`; `--script-updatedb`.
  - `safe=1` runs only safe scripts.
- vulscan NSE (offline CVE/OSVDB/EDB DBs) or maintained `vulners`:
  `nmap -sV --script vulners --script-args mincvss=7.0 <IP>` — validate hits, depends on -sV accuracy.

## Timing

`-T0..5` (paranoid..insane). T4 = `--max-rtt-timeout 1250ms --initial-rtt-timeout 500ms
--max-retries 6 --max-scan-delay 10ms`. T5 adds `--host-timeout 15m`.
Fine control: `--min/max-hostgroup`, `--min/max-parallelism`, `--min/max/initial-rtt-timeout`,
`--max-retries`, `--host-timeout`, `--scan-delay`/`--max-scan-delay`, `--min-rate`/`--max-rate`,
`--defeat-rst-ratelimit` (skip closed/filtered wait).

## Firewall / IDS evasion (detection context)

- `-f` fragment (8B default; `--mtu <mult of 8>`); version scan/scripts don't support fragmentation.
- `-D decoy1,decoy2,ME` decoys (put ME after 5-6 decoys; `RND:<n>` random). Use live IPs inside a LAN.
- `-S <IP>` spoof source IP; `-e <iface>` interface; `-g/--source-port <port>` exploit trust rules
  (e.g. 53, 20, 67): `nmap --source-port 53 <IP>`.
- `--data <hex>`, `--data-string "text"`, `--data-length <n>` pad packets; `--ip-options`;
  `--ttl <v>`; `--randomize-hosts`; `--spoof-mac <mac|vendor|0>`; `--proxies <urls>` (reduce
  parallelism if proxies drop connections).
- IDS/IPS evasion concepts: TTL manipulation (`--ttl`), random data (`--data-length`),
  fragmentation (`-f`), invalid checksums, uncommon IP/TCP options, fragment/stream overlapping
  (BSD/Linux/First/Last reassembly policies), IPv6 extension-header chains & tiny first fragments
  (RFC 7112). Tools: sniffjoke, scapy.

## Output

`-oN` normal (supports `--resume`), `-oX` XML (best for parsing/new features), `-oG` greppable
(deprecated), `-oA` all, `--webxml` portable stylesheet, `-v/-d` levels, `--reason`,
`--stats-every`, `--packet-trace`, `--open`. Runtime keys: v/V, d/D, p/P, ?.

```bash
nmap -sV -oX - 10.10.10.0/24          # XML to stdout
nmap -sV --webxml -oX scan.xml <IP>   # portable HTML-friendly XML
```

## Speedups & special builds

- Speed service scan ~16x: in `/usr/share/nmap/nmap-service-probes` set all `totalwaitms` to 300
  and `tcpwrappedms` to 200 (or compile with changed service_scan.h defaults).
- Nmap ≥7.94: `-sU`+`-sV` share service-probes (UDP scan responses feed version matching);
  `-sV` probes DTLS-wrapped UDP. 7.95: new fingerprints (grpc, mysqlx, tuya) + ICS NSE
  (hartip-info, iec61850-mms). 7.96: parallel forward DNS.
- Static nmap for restricted hosts: build in Docker (static OpenSSL 1.1.1w + PCRE2 +
  `--with-libpcap=included --with-libdnet=included -static`), bundle scripts/nselib + data files;
  verify with `file`; AppArmor/seccomp/SELinux and egress may still block execution.

## Misc

- `-6` IPv6; `-A` = `-O -sV -sC --traceroute`; `-n` no DNS; `-R` always DNS;
  `--system-dns` use OS resolver (split-DNS/hosts compatibility); `--dns-servers <srv>` force resolvers.
- Default discovery probes: `-PA80 -PS443 -PE -PP`. `-sn` ping scan only; `-Pn` skip discovery;
  `-PR` ARP ping (default on local net; `--send-ip` to disable); `-PS/-PA/-PU/-PY/-PO` probe types;
  `-sL` list targets (DNS only) to verify scope ownership.

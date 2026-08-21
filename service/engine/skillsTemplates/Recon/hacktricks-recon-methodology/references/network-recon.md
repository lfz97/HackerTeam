# Network Recon (internal / LAN perspective)

Applies once you are inside a network (or assessing one). Passive first, active second.

## Discovering hosts

### Passive

```bash
netdiscover -p
p0f -i eth0 -p -o /tmp/p0f.log        # passive OS fingerprint
# bettercap:
net.recon on        # read local ARP cache periodically
net.show
set net.show.meta true
```

### Active (L2)

```bash
nmap -sn <Network>                    # ARP requests on local net
netdiscover -r <Network>
nbtscan -r 192.168.0.1/24             # NetBIOS name discovery
# bettercap probes (ARP, mDNS, NBNS, UPNP, WSD):
net.probe on
set net.probe.mdns true; set net.probe.nbns true; set net.probe.upnp true; set net.probe.wsd true
alive6 <IFACE>                        # IPv6 multicast ping
```

### Active ICMP tricks

- Broadcast ping reaches every host: `ping -b 10.10.5.255`; `ping -b 255.255.255.255` may even
  reveal hosts in other subnets.
- `nmap -PE -PM -PP -sn -vvv -n 10.12.5.0/24` (echo/timestamp/subnet-mask).
- Misconfigured devices sometimes reply from private source IPs on public interfaces — catch them:
  `tcpdump -nt -i eth2 src net 10 or 172.16/12 or 192.168/16`

### mDNS / DNS-SD / SSDP / WSD discovery

- mDNS: UDP 5353, multicast 224.0.0.251 / ff02::fb; resolves `*.local`; TTL=0 releases a name.
  Tools: avahi-browse --all, bettercap net.probe.mdns, Responder.
- DNS-SD: query `_printer._tcp.local` etc.; service list at dns-sd.org/ServiceTypes.html.
- SSDP: UDP 1900, multicast 239.255.255.250 (UPnP discovery).
- WSD/WS-Discovery: UDP 3702 probes/hello.

## Sniffing

```bash
sudo tcpdump -i <IFACE> udp port 53      # what are hosts resolving?
tcpdump -i <IFACE> icmp
# rolling web captures:
sudo nohup tcpdump -i eth0 -G 300 -w "/tmp/dump-%m-%d-%H-%M-%S-%s.pcap" -W 50 'tcp and (port 80 or port 443)' &
# remote capture piped to local Wireshark:
ssh user@<TARGET> tcpdump -i ens160 -U -s0 -w - 'port not 22' | sudo wireshark -k -i -
# bettercap:
net.sniff on; net.sniff stats
set net.sniff.output sniffed.pcap; set net.sniff.filter "not arp"
```

Credential extraction from pcaps: PCredz (github.com/lgandx/PCredz).

## LLMNR / NBT-NS / mDNS / WPAD exposure (recon view)

These unauthenticated UDP broadcast protocols reveal what hosts are looking for and can answer
them. Detection/recon first: run Responder in Analyze/listen mode or Dementor in analysis mode
to see the traffic without attacking:

```bash
responder -I <Interface>                 # default poisoning (active — scope check!)
Dementor -I <interface> -A               # analysis mode
```

Notes: Responder logs to /usr/share/responder/logs; Dementor adds CUPS and finer config.
Inveigh is the Windows equivalent (PowerShell Invoke-Inveigh or Inveigh.exe).
Also relevant: WSUS over HTTP 8530 (NTLM), which can be enumerated via nmap -p 8530,8531 and
registry/GPO checks (WUServer/WUStatusServer). Relay feasibility auditing tool: RelayKing
(`--gen-relay-list` for ntlmrelayx targets; checks SMB signing, EPA/CBT, coercion vectors).

## IPv6 recon & attacks (link-local, very stealthy)

Theory: prefix(48) + subnet(16) + IID(64); FE80::/10 link-local, FC00::/7 ULA, 2000::/3 global,
ff02::1 all-nodes, ff02::2 all-routers. Derive link-local from MAC `12:34:56:78:9a:bc` →
`fe80::1034:56ff:fe78:9abc` (insert fffe, flip 7th bit).

Discovery:

```bash
ping6 -I <IFACE> ff02::1    # then: ip -6 neigh
alive6 eth0
# find IPv6 via DNS: AXFR/AAAA/ANY queries; search engine: site:ipv6.*
```

Passive NDP/DHCPv6 sniffing maps MAC↔IPv6 without sending anything (Scapy sniff of RS/RA/NS/NA,
DHCP6 Solicit/Advertise on UDP 547). Capture legit RAs to decide attack vector:

```bash
sudo tcpdump -vvv -i eth0 'icmp6 && ip6[40] == 134'   # Router Advertisements; check flags M/O
```

- M=1/O=1 → DHCPv6 spoofing viable (mitm6: `sudo mitm6 -i eth0 --no-ra -d corp.local`,
  pair with `ntlmrelayx.py -6 -t ldaps://dc -wh wpad`).
- M=0/O=0 (pure SLAAC) → rogue RA / RDNSS spoofing instead (RFC 8106).
- RA-Guard bypass variants: `atk6-fake_router6 -H/-F/-D eth0 <prefix>` (hop-by-hop/frag evasion);
  `atk6-flood_router26 -F -m eth0`.
- Rogue DHCPv6: `atk6-fake_dhcps6 <IFACE> <PREFIX>/<LEN> <DNSv6>`; starvation: `atk6-flood_dhcpc6`;
  info dump: `atk6-dump_dhcp6 <IFACE>`; capture: `tcpdump 'udp port 546 or udp port 547'`.
- DHCPv6 basics: client UDP 546, server 547; ff02::1:2 relay-agents multicast; DUIDs correlate
  hosts; IA_NA addresses, IA_PD prefix delegation; Reconfigure needs OPTION_RECONF_ACCEPT.

Defences to note: RA Guard/DHCPv6 Guard/ND Inspection, port ACLs for RA source MAC, alerts on
high-rate RA or RDNSS changes.

## VLAN / switching recon

- CDP leaks device model/IOS version — sniff with tcpdump/wireshark/yersinia.
- Identify your switch port via CDP or `show mac address-table | include <MAC>`.
- Enumerate VLANs: `show vlan brief`; DTP state: yersinia or dtpscan.py; once trunk forms,
  enumerate visible VLAN IDs from STP/tagged traffic.
- 802.1Q sub-interfaces to reach enumerated VLANs:

```bash
sudo modprobe 8021q
sudo ip link add link eth0 name eth0.10 type vlan id 10
sudo ip link set eth0.10 up
sudo dhclient -v eth0.10        # or static: ip addr add 10.10.10.66/24 dev eth0.10
```

- Voice VLAN hop: LLDP-MED/CDP reveals VVID; voiphopper (`voiphopper -i eth0 -z` auto-hop).
- Tools: yersinia (DTP/VTP/STP/CDP), VLANPWN, frogger (nccgroup vlan-hopping), dtp-spoof.
- Private VLAN / client isolation L3 bypass: send packet with victim IP + router MAC.

## Routing protocol recon

- EIGRP: IP proto 88, multicast 224.0.0.10 (v6: FF02::A). Sniff: `tcpdump 'ip proto 88 or ip6 proto 88'`;
  enumerate: `nmap --script broadcast-eigrp-discovery`. Extract AS, K-values, holdtime, auth from
  captured HELLO PARAMETER TLV before any injection.
- HSRP: v1 multicast 224.0.0.2, v2 224.0.0.102 (v6 FF02::66), UDP 1985/2029; GLBP UDP 3222 (v6 FF02::66).
  Capture hellos; MD5 auth crackable via hsrp2john + John. Loki parses/injects both.
- OSPF: Keyed-MD5 or HMAC-SHA auth; capture authenticators for offline cracking (legacy Loki).
- RIP: UDP 520 (RIPng 521 IPv6 multicast); RIPv2 MD5.

## DHCP recon

```bash
nmap --script broadcast-dhcp-discover    # reveals DHCP server, offered IP, DNS, domain
```

Rogue DHCP values of interest: rogue DNS/WPAD beats rogue gateway. Responder DHCP.py options:
`-i` attacker IP as GW, `-d` domain, `-r` real router, `-p` rogue DNS, `-w` WPAD URL, `-S` spoof GW.

## STP / VTP notes

If you can't see BPDUs, STP attacks won't work. yersinia stp attacks 0-6 (DoS, TCN CAM flush,
root bridge takeover → MitM between two switches; bridge with ettercap -T -B). VTP: higher
revision advertisement overwrites VLAN DB (needs trunk).

## Bluetooth L2CAP/GATT surface (Android Fluoride)

PSMs: SDP 0x0001, RFCOMM 0x0003, BNEP 0x000F, AVCTP 0x0017/0x001B, AVDTP 0x0019, ATT/GATT 0x001F.
BlueBlue (Scapy-based, on BlueBorne l2cap_infra) for L2CAP/ATT probing.

## Telecom core exposure (if in scope)

- GTP-C listeners: `masscan <range> -pU:2123 --rate 50000`; GRX/IPX often allows ICMP.
- Default OSS creds: hydra with vendor-default wordlists.
- 5G NAS pre-security window: capture InitialUEMessage (ngap.procedure_code==15,
  nas_5g.message_type==65) — check for plaintext SUPI/IMSI, NEA0/NIA0 downgrades, replay.
  Tools: Open5GS lab, 5GReplay, Sni5Gect.
- Milesight UR-series routers: unauthenticated SMS API `/cgi` (query_inbox/query_outbox JSON-RPC)
  and CVE-2023-43261 log exposure `/lang/log/httpd.log` with AES-recoverable admin passwords.

## Local gateway discovery

```bash
arp-scan -l | tee hosts.txt
./gateway-finder.py -f hosts.txt -i <INTERNET_IP>   # which hosts forward IPv4
```

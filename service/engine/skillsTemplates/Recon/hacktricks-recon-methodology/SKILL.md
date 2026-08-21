---
name: hacktricks-recon-methodology
description: >-
  Complete reconnaissance methodology distilled from HackTricks: external asset discovery
  (acquisitions, ASN/BGP, reverse whois, trackers, favicon hash), subdomain enumeration
  (OSINT tools, DNS brute force, permutations, vhosts, CORS discovery, CT logs), passive intel
  (Shodan/Censys/FOFA dorks, crt.sh, SecurityTrails, GitHub/code leaks, database breach search,
  Google dorks), port/service scanning with nmap and masscan, internal host discovery, IPv6/DHCPv6
  and LLMNR/mDNS reconnaissance, WiFi survey and EAP identity harvesting, and phishing-domain /
  typosquat detection. USE THIS SKILL whenever you need to enumerate domains/subdomains, map an
  organization's internet footprint, find leaked secrets or credentials, choose nmap flags,
  discover live hosts on a network, fingerprint WAFs, correlate assets by favicon/cert/tracker,
  or assess WiFi from a recon perspective — even if the task does not say "recon" explicitly.
  Complements pentest-tools (tool paths) and quake-api (Quake queries): this skill tells you the
  methodology and exact commands to run.
---

# HackTricks Recon Methodology

Distilled from a full read of the HackTricks wiki sections: External Recon Methodology,
Pentesting Methodology, Pentesting Network, Pentesting WiFi, Phishing Methodology, Threat
Modeling and Fuzzing. Source: `/mnt/d/code/my/hacktricks/src/generic-methodologies-and-resources/`.

## When to read which reference

| Situation | Read |
|-----------|------|
| Enumerate a company's domains/subdomains/IPs from the outside | `references/external-recon.md` |
| Hunt leaked secrets/creds: GitHub dorks, breach DBs, code search, Google dorks | `references/passive-intel-leaks.md` |
| Choose nmap/masscan flags, port-scan hosts, service/OS fingerprint | `references/nmap-scanning.md` |
| Inside a network: find live hosts, sniff, IPv6/DHCPv6/LLMNR recon, VLAN/routing recon | `references/network-recon.md` |
| WiFi survey: monitor mode, AP enumeration, WPS/EAP recon, Android/NexMon | `references/wifi-recon.md` |
| Find typosquats/phishing domains of a brand; email harvesting; phishing infra detection | `references/phishing-recon.md` |

## Core workflow (external recon)

Build a validated asset list BEFORE scanning. The order matters because each phase feeds the
next (e.g., reverse-whois on every new domain, favicon pivots on every new web server):

1. **Companies & scope** — acquisitions (CrunchBase, Wikipedia, OpenCorporates, GLEIF LEI).
2. **ASN / IP ranges** — bgp.he.net, bgpview.io, ipinfo.io, `amass intel -org <name>`; BBOT auto-summarizes ASNs.
3. **Domains** — reverse DNS on IP ranges (`dnsrecon -r`), reverse whois loop, tracker/copyright/favicon/CT correlation, Shodan `org:`/`ssl:` pivots.
4. **Subdomains** — OSINT tools (subfinder, Amass, BBOT, ...), DNS brute force, permutations, vhost fuzzing, CT monitoring.
5. **IPs** — resolve everything, pull historical IPs (SecurityTrails) to bypass CDNs.
6. **Web servers** — httprobe/httpx probing, screenshots (EyeWitness/gowitness) to prioritize.
7. **Cloud assets** — bucket/keyword permutations (cloud_enum, S3Scanner).
8. **Emails & leaks** — theHarvester/hunter.io + breach DBs + GitHub/paste secret hunting.
9. Re-run the pivots whenever a new domain appears — each result can expose more certificates, passive-DNS records and favicon matches.

Modern pipeline shortcut (JSONL between phases makes re-runs after new creds easy):

```bash
bbot -t company.com -p subdomain-enum cloud-enum code-enum email-enum spider
httpx -l hosts.txt -sc -title -td -favicon -jarm -asn -jsonl -o httpx.jsonl
katana -list live_hosts.txt -jc -js-crawl -kf all -xhr -fx -jsonl -o katana.jsonl
naabu -list live_hosts.txt -top-ports 1000 -exclude-cdn -json -o naabu.jsonl
```

## Decision notes & conflict handling

- Prefer **passive** techniques first (OSINT, CT, passive DNS, search engines) — they are
  stealthier and often faster; go active (DNS brute force, port scans) only after passive is
  exhausted, and keep rate low to avoid IDS triggers.
- **Favicon/tracker/copyright matches are leads, not proof.** Always validate with title,
  body hash, TLS SANs and ports before reporting an asset as in-scope.
- **Historical IPs** of a domain that no longer belong to the target are usually out of scope —
  verify ownership before scanning.
- Where HackTricks lists several tools for the same job, prefer the maintained one
  (e.g., BBOT/subfinder over archived tools like gitrob; Titus over archived Nosey Parker).
- Recon boundary: discover and record; do NOT exploit findings (no takeover, no injection).

## Threat modeling (for scoping)

When asked to model the attack surface of a target: draw DFDs with trust boundaries and apply
STRIDE (Spoofing, Tampering, Repudiation, Information disclosure, DoS, Elevation of privilege);
score with DREAD; tools: OWASP Threat Dragon, Microsoft Threat Modeling Tool. Methodologies:
PASTA (risk-centric, 7 stages), Trike, VAST, OCTAVE.

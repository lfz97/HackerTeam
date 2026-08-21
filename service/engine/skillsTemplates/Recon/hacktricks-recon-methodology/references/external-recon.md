# External Recon Methodology (full workflow)

Goal: obtain all companies owned by the target, then all their assets (IP ranges, domains,
subdomains, cloud assets, emails, leaked creds). Repeat each pivot whenever a new domain is
found — every new result can expose certificates, passive-DNS links and favicon matches that
were invisible from the seed.

## 1. Companies (scope expansion)

- CrunchBase → company page → "acquisitions"; Wikipedia "acquisitions" section.
- Public companies: SEC/EDGAR filings, investor-relations pages, Companies House (UK).
- Corporate trees: OpenCorporates (https://opencorporates.com/), GLEIF LEI DB (https://www.gleif.org/).

## 2. ASN & IP ranges

ASN = number assigned to an autonomous system; finding a company's ASN gives its IP ranges.

- Search by company name / IP / domain: https://bgp.he.net/, https://bgpview.io/, https://ipinfo.io/
- Regional registries: AFRINIC, ARIN, APNIC, LACNIC, RIPE NCC.
- Automation:

```bash
amass intel -org tesla
amass intel -asn 8911,50313,394161
bbot -t tesla.com -f subdomain-enum   # ASN module auto-summarizes at the end
```

- http://asnlookup.com (free API) for org IP ranges; http://ipv4info.com for domain→IP/ASN.

## 3. Domains

### Reverse DNS on IP ranges

Works only if PTR records are set. Use victim's DNS or public resolvers:

```bash
dnsrecon -r <DNS Range> -n <IP_DNS>
dnsrecon -d facebook.com -r 157.240.221.35/24
dnsrecon -r 157.240.221.35/24 -n 1.1.1.1
dnsrecon -r 157.240.221.35/24 -n 8.8.8.8
```

Online: http://ptrarchive.com. At scale: massdns, dnsx.

### Reverse whois (loop)

Whois fields (org name, email, phone) → search other registrations sharing them; recurse.

Free: ip.thc.org (web+API), viewdns.info/reversewhois, domaineye.com/reverse-whois,
reversewhois.io, whoix.com (web free). Paid: DomainTools, whoisxmlapi, domainiq,
SecurityTrails, whoisfreaks.
Automation: DomLink (needs whoxy API key); `amass intel -d tesla.com -whois`.

### Trackers

Same Google Analytics / AdSense ID across sites ⇒ same operator. Tools: Udon (github.com/dhn/udon),
BuiltWith, Sitesleuth, Publicwww, SpyOnWeb, Webscout (Sc0ut).

### Favicon hash pivots

MMH3 over base64-encoded favicon bytes; pivot in Shodan/FOFA/Censys:

```python
import mmh3, requests, codecs
def fav_hash(url):
    r = requests.get(url, timeout=10)
    print(f"{url} : {mmh3.hash(codecs.encode(r.content, 'base64'))}")
```

```bash
shodan search org:"Target" http.favicon.hash:116323821 --fields ip_str,port --separator " " | awk '{print $1":"$2}'
# FOFA: icon_hash="116323821"
# Batch: httpx -l targets.txt -favicon ; favihash.py -f https://target/favicon.ico -t targets.txt -s
```

Caveats: MMH3 collisions/spoofing possible; probe beyond /favicon.ico (manifest files,
browserconfig.xml, apple-touch-icon*, `<link rel=icon>`); static assets may remain reachable
behind WAF/SSO — check ETag/Last-Modified/redirects; validate matches against title, body hash,
headers, TLS SANs and ports; cluster by HTML hash; a hash appearing across unrelated
signatures/products may be a honeypot; CDNs/SNI-only surfaces are missing from IP-centric datasets.

### Copyright / unique strings

Find strings shared across the org's sites (copyright notices); search them in Google or
`shodan search http.html:"Copyright string"`.

### Certificate-transparency correlation

Cron-based bulk renewal (`certbot renew` for all certs at once) means certificate timestamps /
CT log positions correlate related domains. Direct CT search: crt.sh, certspotter.com,
search.censys.io, chaos.projectdiscovery.io (+ chaos-client).

### DMARC correlation

Domains/subdomains sharing the same DMARC record: dmarc.live/info/<domain>,
github.com/Tedixx/dmarc-subdomains, spoofcheck, dmarcian.

### Shodan / ssl / assetfinder

```bash
shodan search org:"Tesla, Inc."     # check TLS certs of hits for new domains
# take the TLS 'Organisation' of the main site, then:
shodan search ssl:"Tesla Motors"    # or use sslsearch tool
assetfinder --subs-only <domain>
```

### Passive DNS / historical

securitytrails.com, community.riskiq.com (PassiveTotal), DomainTools Iris, Farsight DNSDB.
Also check domain takeover: domains still used but whose registration expired.

## 4. Subdomains

### DNS enumeration + zone transfer attempt

```bash
dnsrecon -a -d tesla.com
```

### OSINT tools (configure API keys for best results)

```bash
bbot -t tesla.com -f subdomain-enum               # or -rf passive for passive-only
bbot -t tesla.com -f subdomain-enum -m naabu gowitness -n my_scan -o .
amass enum [-active] [-ip] -d tesla.com
subfinder -d tesla.com [-silent]
findomain -t tesla.com [--quiet]
python3 oneforall.py --target tesla.com [--dns False] [--req False] [--brute False] run
assetfinder --subs-only <domain>
sudomy -d tesla.com          # needs sudomy.api with keys
vita -d tesla.com
theHarvester -d tesla.com -b "crtsh, dnsdumpster, virustotal, securityTrails, ..."
```

### Free APIs & scrapers

```bash
curl https://ip.thc.org/tesla.com
curl https://sonar.omnisint.io/subdomains/tesla.com | jq -r ".[]"     # Crobat API
curl https://jldc.me/anubis/subdomains/tesla.com | jq -r ".[]"
rapiddns(){ curl -s "https://rapiddns.io/subdomain/$1?full=1" | grep -oE "[\.a-zA-Z0-9-]+\.$1" | sort -u; }
crt(){ curl -s "https://crt.sh/?q=%25.$1" | grep -oE "[\.a-zA-Z0-9-]+\.$1" | sort -u; }
gau --subs tesla.com | cut -d "/" -f 3 | sort -u
python3 SubDomainizer.py -u https://tesla.com | grep tesla.com     # JS scraping
python subscraper.py -u tesla.com | grep tesla.com | cut -d " " -f
shodan domain <domain>; shodan search "http.html:help.domain.com"
python3 censys-subdomain-finder.py tesla.com   # needs CENSYS_API_ID/SECRET
python3 DomainTrail.py -d example.com
```

Also: securitytrails free API, chaos.projectdiscovery.io (free bug-bounty subdomain data,
chaospy client, chaos-public-program-list).

Tool comparison: https://blog.blacklanternsecurity.com/p/subdomain-enumeration-tool-face-off

### DNS brute force

Wordlists: jhaddix all-in-one gist, assetnote best-dns-wordlist, localdomain.pw, commonspeak,
SecLists/Discovery/DNS. Resolvers: dnsvalidator-filtered list or trickest resolvers-trusted.txt.

```bash
# massdns (fast, false-positive prone)
sed 's/$/.domain.com/' subdomains.txt > bf-subdomains.txt
./massdns -r resolvers.txt -w /tmp/results.txt bf-subdomains.txt
grep -E "tesla.com. [0-9]+ IN A .+" /tmp/results.txt

gobuster dns -d mysite.com -t 50 -w subdomains.txt
shuffledns -d example.com -list example-subdomains.txt -r resolvers.txt
puredns bruteforce all.txt domain.com
aiodnsbrute -r resolvers -w wordlist.txt -vv -t 1024 domain.com
```

### Second-round permutations

```bash
cat subdomains.txt | dnsgen -
goaltdns -l subdomains.txt -w words-permutations.txt -o final-words.txt
gotator -sub subdomains.txt -silent [-perm words-permutations.txt]
altdns -i subdomains.txt -w words-permutations.txt -o out
cat subdomains.txt | dmut -d words-permutations.txt -w 100 --dns-errorLimit 10 --use-pb --verbose -s resolvers-trusted.txt
# alterx: pattern-based candidate generation
```

Smart permutation: **regulator** learns regex-like patterns from found subdomains:

```bash
python3 main.py adobe.com adobe adobe.rules
make_brute_list.sh adobe.rules adobe.brute
puredns resolve adobe.brute --write adobe.valid
```

**subzuf**: DNS-response-guided brute-force fuzzer: `echo www | subzuf facebook.com`

### VHosts on an IP

OSINT: HostHunter. Brute force the Host header (ffuf `-ac` auto-calibration filters defaults):

```bash
ffuf -u http://10.10.10.10 -H "Host: FUZZ.example.com" -w subdomains-top1million-20000.txt -ac
ffuf -c -w wordlist -u http://victim.com -H "Host: FUZZ.victim.com"
gobuster vhost -u https://mysite.com -t 50 -w subdomains.txt
wfuzz -c -w SecLists/.../subdomains-top1million-20000.txt --hc 400,404,403 -H "Host: FUZZ.example.com" -u http://example.com -t 100
vhostbrute.py --url="example.com" --remoteip="10.1.1.15" --base="www.example.com" --vhosts="vhosts_full.list"
VHostScan -t example.com
```

This may expose internal/hidden endpoints.

### CORS-based subdomain discovery

Pages that echo `Access-Control-Allow-Origin` only for valid subdomains in `Origin`:

```bash
ffuf -w subdomains-top1million-5000.txt -u http://10.10.10.208 -H 'Origin: http://FUZZ.crossfit.htb' -mr "Access-Control-Allow-Origin" -ignore-body
```

### Buckets, monitoring, takeovers

- While enumerating, watch CNAMEs pointing to buckets (S3/GCP/Azure) — brute-force bucket names too.
- Monitor new certs: sublert (CT-log watcher).
- Check subdomain takeover candidates; subdomains resolving to IPs outside known asset ranges
  deserve their own port scan (mind scope: they may be third-party hosted).

## 5. IPs

Collect all IPs from ranges + DNS resolutions. Historical IPs (SecurityTrails free API) may still
be owned by the client → possible CloudFlare bypass. hakip2host: domains pointing at a given IP.
Port-scan everything that isn't a CDN.

## 6. Web server hunting

```bash
masscan -p80,443,8000-8100,8443 <ranges>          # fast web-port discovery
cat /tmp/domains.txt | httprobe                    # 80+443 probing
cat /tmp/domains.txt | httprobe -p http:8080 -p https:8443
# fprobe, httpx equivalent
```

Screenshots to triage: EyeWitness, HttpScreenshot, Aquatone, Gowitness, webscreenshot; then
eyeballer to rank likely-vulnerable pages.

## 7. Public cloud assets

Keywords (company terms, domain/subdomain names) + bucket wordlists (goaltdns words.txt,
altdns words.txt, AWSBucketDump BucketNames.txt) → permutations → tools:

cloud_enum, CloudScraper, cloudlist, S3Scanner. Look beyond S3 buckets (functions, registries).

## 8. Emails

theHarvester (free, many sources), hunter.io / snov.io / minelead.io free APIs, phonebook.cz,
maildb.io, anymailfinder. Verify/enumerate more via SMTP VRFY/RCPT brute force against the
target's mail server. Emails feed brute force, phishing and OSINT on the people.

## 9. Credential & secret leaks

Breach DBs: leak-lookup.com, dehashed.com (see passive-intel-leaks.md for the full list).
Secrets: Leakos (gitleaks over all org+dev repos and URLs), Pastos (80+ paste sites),
Google dorks via Gorks (full google-hacking-database; plain-browser tools get blocked fast).

## 10. Automated full-recon frameworks

rengine, Osmedeus, reconftw, EchoPwn (old).

## Recap checklist

Companies → assets/ASNs → domains → subdomains (+takeover check) → IPs (CDN vs not) → web
servers + screenshots → cloud assets → emails/cred leaks/secret leaks → test the webs.

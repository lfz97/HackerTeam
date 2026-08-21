# Phishing-Infrastructure Recon & Detection

Recon side of phishing: harvesting emails, finding typosquats/lookalike domains the attacker
(or defender) cares about, and detecting phishing infrastructure (gophish, AiTM, clones).

## Email discovery

theHarvester (free), phonebook.cz (free), maildb.io, hunter.io, anymailfinder.com, snov.io,
minelead.io (free tiers). Also: SMTP username enumeration/brute force against the target's mail
server; webmail login username brute force.

## Domain-variation generation (offensive naming or defensive monitoring)

Techniques: keyword domains (victim.com-management.com), hyphenated subdomain (www-victim.com),
new TLD, homoglyph (zelfser.com), transposition, singular/plural, omission, repetition,
replacement, subdomained (vi.ctim.com), insertion, missing dot (victimcom.com), bitsquatting
(bit-flip domains — e.g. windows.com → windnws.com).

Tools: **dnstwist**, **urlcrazy**; sites: dnstwist.it, dnstwister.report, typo-generator tools.
Buy expired trusted domains: expireddomains.net; verify reputation in FortiGuard/Palo Alto URL
filtering before use.

## Homograph / homoglyph specifics

Unicode lookalikes: Greek Η (U+0397)→H, ρ (U+03C1)→p; Cyrillic а (U+0430)→a, е (U+0435)→e;
Armenian օ (U+0585)→o; Cherokee Ꭲ (U+13A2)→T. Detection: mixed-script inspection
(`unicodedata.name(ch)` block check per display name/subject/URL); Punycode normalization with
`idna.encode(hostname)` before allow-list comparison; dnstwist `--fuzzers homoglyph` / urlcrazy
for permutations.

## Hunting suspicious/lookalike domains (defensive)

1. Generate candidates (dnstwist/urlcrazy incl. bit-flip + homoglyphs) and resolve them; feed
   NXDOMAIN lookups from internal DNS logs (users hitting typos before registration).
2. Basic checks per candidate: open ports (HTTP/HTTPS + **3333 for gophish admin panels**),
   login forms similar to the victim's, domain age (younger = riskier), screenshots.
3. Advanced: continuous monitoring; spider candidate sites and compare login forms to victim's
   with **ssdeep**; submit junk creds and check for redirect to the victim's domain.

### Favicon & fingerprint hunting

Phishing kits reuse brand favicons → Shodan mmh3 favicon pivot:

```python
import base64, requests, mmh3
b64 = base64.encodebytes(requests.get("https://brand.com/favicon.ico", timeout=10).content)
print(mmh3.hash(b64))   # then: shodan search http.favicon.hash:<value>
```

FavFreak for hashing + dork generation. Validate matches (reuse is common).

### urlscan.io telemetry

```
page.domain:(/.*yourbrand.*/ AND NOT yourbrand.com AND NOT www.yourbrand.com)
domain:yourbrand.com AND NOT page.domain:yourbrand.com    # hotlinked assets
+ date:>now-7d
```

API pivot on `page.tlsIssuer/tlsValidFrom/tlsAgeDays`, `task.source:certstream-suspicious`.

### Domain age via RDAP

```bash
curl -s https://rdap.verisign.com/com/v1/domain/<d>.com | jq -r '.events[] | select(.eventAction=="registration") | .eventDate'
curl -s https://www.rdap.net/domain/<d>.com | jq
```

Bucket NRDs (<7d / <30d) for triage priority. Suspicious TLDs: .zip/.mov (filename confusion).

### Certificate Transparency

Search Subject/SAN for brand keywords (crt.sh, Censys; filter by date/CA, e.g. Let's Encrypt);
CertStream + phishing_catcher scores suspicious cert names in near-real-time. Prioritize NRDs,
privacy WHOIS, fresh NotBefore.

### AiTM infra detection

Evilginx-style proxies: JA3/JA4/JA4S/JA4H fingerprinting at egress (weak signal, confirm with
content/domain intel); record TLS cert metadata (issuer, SAN count, wildcard, validity) for
lookalikes and correlate with DNS age/geo.

## Phishing infrastructure setup (for authorized campaigns)

### GoPhish

Install release to /opt/gophish; admin UI port 3333 (tunnel: `ssh -L 3333:127.0.0.1:3333 user@ip`).
TLS: certbot standalone → copy privkey/fullchain to /opt/gophish/ssl_keys; config.json:
admin_server 127.0.0.1:3333 TLS, phish_server 0.0.0.0:443 with cert paths.

Mail: postfix (`myhostname`/`mydestination`), /etc/hostname + /etc/mailname = domain,
A record mail.domain + MX → mail.domain. Records to set:
- rDNS PTR for VPS IP → domain.
- SPF TXT: `v=spf1 mx a ip4:<VPS_IP> ?all` (spfwizard.net).
- DMARC TXT on `_dmarc.<domain>`: `v=DMARC1; p=none`.
- DKIM (opendkim; concatenate both B64 key parts in the TXT record).

Testing: mail-tester.com (send via `mail -s`), `check-auth@verifier.port25.com` (read
/var/mail/root), Gmail header check for dkim=pass. Delist: spamhaus.org/lookup, sender.office.com.
Wait ≥1 week for domain age before the campaign; send test emails to 10min-mail addresses.

Campaign pieces: Sending profile (noreply/support/servicedesk names; Ignore Certificate Errors),
email template with {{.FirstName}} variables + tracking image, landing page (clone with
`wget --mirror --page-requisites --convert-links --adjust-extension <URL>` / goclone / SET),
launch & monitor opens/clicks/submits.

### Modern identity-flow phishing (recon-relevant)

- Helpdesk impersonation → MFA reset/re-enrollment; monitor deleteMFA+addMFA from same IP.
- OAuth device-code phishing (victim enters attacker code at microsoft.com/devicelogin) → token theft.
- QR lures; SEO poisoning + ClickFix (fake CAPTCHA → "paste this into Win+R"); TDS handoff
  (visible href is real, first click hijacked); LLM-runtime-generated stealers (eval of LLM
  response); mobile-gated phishing (`detect_device.js` → POST /detect → 500 for non-mobile,
  hunt: urlscan `filename:"detect_device.js" AND page.status:500`).
- WhatsApp QR device-linking hijack; Discord invite hijacking (expired temporary / deleted
  lowercase permanent / lost-boost vanity codes re-registered by Level-3-boost servers).
- AI agent-mode phishing: hosted agent browser "take over" handoff; OCR/navigation prompt
  injection in agentic browsers (hidden low-contrast text; trust-zone primitives
  INJECTION/CTX_IN/REV_CTX_IN/CTX_OUT).
- Repo-local AI tooling abuse: `.claude/settings.json` hooks, `.mcp.json`, `CODEX_HOME` .env
  redirect (Codex CLI CVE-2025-61260) — treat dot-directories as executable inputs.

### Phishing document/loader awareness (for artifacts you find)

Office macros (.docm/renamed .docx/.rtf still execute; remote template trick), includePicture
external load (`Insert → Quick Parts → Field → includePicture http://<ip>/x`), HTA via mshta,
LNK+ZIP marker-carved PowerShell stages, stego-delimited payloads in images (<<sudo_png>>...),
JS/VBS→Base64 PowerShell staging, ClickFix clipboard chains (ClearFake, Scarlet Goldfinch,
SyncAppvPublishingServer.vbs LOLBAS). NTLM coercion via invisible images/UNC paths in emails.

## Defenses to verify on the target (recon output)

SPF/DMARC/DKIM posture (`dig TXT`, dmarc.live), display-name≠sender-domain policies, Unicode
sanitization in SEG/SIEM, user awareness gaps.

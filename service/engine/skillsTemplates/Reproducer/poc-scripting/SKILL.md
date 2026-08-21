---
name: poc-scripting
description: >-
  Standalone Python reproduction-script patterns for converting verified vulnerability
  structured blocks into runnable PoC/Exploit scripts. USE THIS SKILL whenever you need to
  write a python3 script that detects or exploits a confirmed vulnerability: map
  entry_point/payload fields to requests, implement success/failure signature checks from
  verification fields, choose protocol libraries (requests/socket/impacket/scapy), and
  handle timeouts/retries/TLS/encoding. Covers SQL injection, XSS, SSTI, RCE, file upload,
  SSRF, open redirect, auth bypass, and non-HTTP protocols (Redis/Docker API/SMB/MySQL).
  The Reproducer prompt owns the script structure and output spec; this skill supplies the
  per-vulnerability-type coding patterns.
---

# PoC/Exploit Scripting Patterns

Convert a structured block (`entry_point`, `payload`, `verification`) into a runnable script.

## Request Construction

- `entry_point.method`/`path`/`params` → `requests.request(method, base_url + path, ...)`
- `entry_point.headers` map 1:1; always copy `Cookie`/`Authorization` verbatim
- `payload.injection_point` names the exact field to replace with `payload.raw` (query param, body key, header, path segment)
- Build the URL with `urljoin`-safe concatenation; keep query params explicit, never f-string a full URL
- If the block gives `base_url` with a trailing slash, strip it before joining

## Verification Logic (PoC mode)

- `verification.success_indicators`/`failure_indicators` are **response signatures**, not vague goals
- Implement as a decision tree over (status_code, headers, body regex, timing):
  ```python
  def vulnerable(resp):
      if resp.status_code != 200:
          return False
      if re.search(r"error in your SQL syntax", resp.text, re.I):
          return True
      return "admin" in resp.text and "uid" in resp.text
  ```
- Distinguish *baseline* vs *attack* responses: send the same request with a benign payload first; the vulnerability is the **difference**, not the absolute response
- Timing-based checks (blind SQLi): measure 5 runs of baseline vs 5 runs of payload, compare means; require ≥2x gap before calling it vulnerable
- Exit codes: 0 = vulnerable, 1 = not vulnerable, 2 = error/insufficient info (never mix them up)

## Script Hygiene

- `requests` with `timeout=10`, wrapped in `try/except (requests.exceptions.RequestException, TimeoutError)`
- TLS: `verify=False` + `urllib3.disable_warnings()` only when the target uses self-signed certs (the norm in pentest labs)
- Retry with backoff for transient failures (max 2 retries); never retry attack payloads blindly — a retried exploit can double-fire
- PoC mode must be **non-destructive and idempotent**: no writes to the target, no sleeps > 30s, no resource-exhaustion payloads
- Exploit mode: confirm prerequisites (session cookie, port reachable) before firing; print a clear "PRE-FLIGHT FAILED" and exit 2 if they are missing
- Print results as stable single-line markers (`[+] VULNERABLE`, `[-] NOT VULNERABLE`, `[!] ERROR: ...`) — the report pipeline greps for these
- `pip install` commands in the header comment, minimal deps (`requests` covers 80% of cases; `impacket`/`scapy` only for protocol work)

## Protocol Selection

| Transport | Library |
|---|---|
| HTTP/HTTPS | `requests` |
| Raw TCP/UDP | `socket` (timeouts mandatory) |
| Redis | `redis` or raw RESP via socket |
| Docker/containerd API | `requests` against unix socket or HTTP API |
| SMB/Windows | `impacket` (`SMBConnection`, `smbclient`) |
| MySQL/MSSQL | `pymysql`/`pyodbc` (or raw protocol for odd cases) |
| ICMP/broadcast | `scapy` (needs root) |

Per-type request/verification skeletons: see `references/`.

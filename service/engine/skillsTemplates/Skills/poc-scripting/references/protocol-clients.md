# Non-HTTP Protocol Reproduction Patterns

## Redis (unauth / weak auth)

```python
import socket

def redis_cmd(host, port, *parts, password=None, timeout=5):
    s = socket.create_connection((host, port), timeout=timeout)
    if password:
        s.sendall(b"*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n" % (len(password), password.encode()))
        s.recv(64)
    payload = ("*%d\r\n" % len(parts)) + "".join(
        "$%d\r\n%s\r\n" % (len(p.encode()), p.encode()) for p in parts)
    s.sendall(payload.encode())
    resp = s.recv(4096)
    s.close()
    return resp

vuln = b"+OK" in redis_cmd(host, port, "PING")
if vuln:                                  # info leak check (PoC-safe read only)
    info = redis_cmd(host, port, "INFO", "server")
    vuln = b"redis_version" in info
```

- Unauthorized detection = `PING` → `+PONG` without auth. Weak-auth detection = `AUTH <password from block>` succeeds
- NEVER issue `CONFIG SET dir`, `SLAVEOF`, or `FLUSHALL` in PoC mode — RCE-via-cron/slaveof belongs in exploit mode with explicit prerequisites (writable dir, restart expectations)

## Docker API (unauthenticated socket/port)

```python
# /var/run/docker.sock or exposed 2375
def docker_version(base="http://127.0.0.1:2375"):
    r = requests.get(base + "/version", timeout=5)
    return r.json() if r.status_code == 200 else None

v = docker_version()
vuln = v is not None and "ApiVersion" in v
if vuln:                                   # container listing = read-only proof
    c = requests.get(base + "/containers/json?all=1", timeout=5)
    count = len(c.json()) if c.status_code == 200 else -1
```

- Unix socket: `requests_unixsocket` (pip) or `http.client` over `urllib.parse.urlsplit` with `socket.AF_UNIX`
- NEVER start/exec containers in PoC mode; `POST /containers/create` + `exec` belongs in exploit mode only

## SMB / Windows (impacket)

```python
from impacket.smbconnection import SMBConnection

conn = SMBConnection(host, host, timeout=10)
try:
    conn.login("", "")                     # null session
    shares = conn.listShares()
    vuln = len(shares) > 0 and any("ADMIN$" not in s["shi1_netname"] for s in shares)
except Exception:
    vuln = False
```

- Null-session detection: `login("","")` succeeding. Guest: `login("guest","")`. Credential reuse: `login(user, pass)`
- Read-only check = `listShares` / `listPath(share, "")`; never write files (`putFile`) in PoC mode
- `impacket` version pin: header comment must list `impacket>=0.11.0`

## MySQL / MSSQL

```python
import pymysql
try:
    conn = pymysql.connect(host=host, port=3306, user=user, password=pwd,
                           connect_timeout=8, read_timeout=8)
    with conn.cursor() as cur:
        cur.execute("SELECT version()")
        vuln = cur.fetchone() is not None
    conn.close()
except (pymysql.err.OperationalError, TimeoutError):
    vuln = False
```

- Weak/default creds (`root`/empty, `sa`/`password`) — try exactly the credential pairs from the structured block, do not brute force
- MSSQL: `pyodbc` needs a driver; prefer `pymssql` in scripts to avoid ODBC driver installs

## Raw TCP Services

- Banner grab: connect, `recv(1024)`, compare against the block's `verification` signature (e.g. `SSH-2.0-OpenSSH` version string, `220` FTP greeting)
- Protocol specifics always come from the structured block's evidence — the script verifies the exact signature, it does not guess services

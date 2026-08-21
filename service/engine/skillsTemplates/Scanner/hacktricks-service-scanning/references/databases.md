# Database Services — Scanner Cheatsheet

General: for every DB, first check **no-auth / default-creds / empty-password** with nmap
scripts, then (authorized only) brute-force via brute-force.md. Never run write/destructive SQL.

## MySQL — 3306/tcp
```bash
nmap -sV -p 3306 --script mysql-audit,mysql-databases,mysql-dump-hashes,mysql-empty-password,mysql-enum,mysql-info,mysql-users,mysql-variables,mysql-vuln-cve2012-2122 <IP>
mysql -h <IP> -u root                             # try empty password
mysql -h <IP> -u root --password=                 # explicit empty
```
Findings: empty root password (mysql-empty-password), CVE-2012-2122 auth-bypass precondition
(version <5.5.24-ish — flag only). Version banner for exploit search.
msf: `auxiliary/scanner/mysql/mysql_version`, `mysql_login` (auth'd), `mysql_hashdump`.

## MSSQL — 1433/tcp (+ UDP 1434 browser)
```bash
nmap --script ms-sql-info,ms-sql-empty-password,ms-sql-xp-cmdshell,ms-sql-config,ms-sql-ntlm-info,ms-sql-tables,ms-sql-hasdbaccess,ms-sql-dac,ms-sql-dump-hashes --script-args mssql.instance-port=1433,mssql.username=sa,mssql.password=,mssql.instance-name=MSSQLSERVER -sV -p 1433 <IP>
msf> use auxiliary/scanner/mssql/mssql_ping       # instance discovery
```
Findings: empty `sa` password (high), xp_cmdshell enabled flag, DAC exposed.
With creds: `mssqlclient.py <dom>/<user>:<pass>@<IP>` (impacket), enumerate links
(`EXEC sp_linkedservers`), hashdump. NTLM steal via responder + `mssql_ntlm_stealer` (post-scan phase).

## PostgreSQL — 5432/tcp
```bash
nmap -sV --script pgsql-info,pgsql-brute -p 5432 <IP>   # brute authorized only
psql -h <IP> -U postgres -W                       # try common passwords
msf> use auxiliary/scanner/postgres/postgres_version
```
With creds: `psql` then `\du` (superusers), `SELECT usename,passwd FROM pg_shadow;`,
`COPY ... FROM PROGRAM` RCE precondition if superuser (flag to Exploit).
Flag: `trust` auth in pg_hba (no password) — detect via successful login with any pass.

## Oracle TNS — 1521-1529/tcp
Requires `odat` (install if needed) or `tnscmd10g`/nmap.
```bash
nmap -sV --script oracle-tns-version -p 1521 <IP>
nmap -p 1521 --script oracle-sid-brute <IP>       # SID discovery
nmap -p 1521 --script oracle-brute --script-args oracle-brute.sid=<SID> <IP>  # authorized
# odat (if installed):
python3 odat.py all -s <IP> -p 1521
python3 odat.py sidguess -s <IP>
python3 odat.py passwordguesser -s <IP> -d <SID>
```
Findings: valid SID, weak/known creds (SCOTT/TIGER, SYSTEM/MANAGER), UTL_FILE/Java exec
primitives (flag to Exploit).

## Redis — 6379/tcp
```bash
nmap --script redis-info -sV -p 6379 <IP>         # unauth info dump
redis-cli -h <IP> INFO                             # NOAUTH? if output = needs creds
redis-cli -h <IP> CONFIG GET "*"                   # only if unauth/creds
```
Findings: **unauthenticated access** = critical-ish (RCE via master/slave or webshell/ssh-key
write primitives — FLAG to Exploit, do not perform). `master/slave` RCE, `MODULE LOAD`,
Lua eval preconditions. If `NOAUTH`, brute-force authorized (brute-force.md).

## MongoDB — 27017/tcp (28017 web stats)
```bash
nmap -sV --script mongodb-info,mongodb-databases,mongodb-brute -p 27017 <IP>
mongosh --host <IP> --eval "db.adminCommand('listDatabases')"   # unauth?
# legacy: mongo <IP> ; show dbs
```
Findings: no auth (all dbs readable), version preconditions (CVE-2019-23902 pre-3.6 auth bypass).

## Memcached — 11211/tcp (udp too)
```bash
nmap -p 11211 --script memcached-info <IP>
printf "stats\r\n" | nc -vn <IP> 11211             # unauth stats
printf "version\r\n" | nc -vn <IP> 11211
```
Findings: unauth stats/key dump, UDP amplification (msf `memcached_amp` check only).

## Cassandra — 9042/9160/tcp
```bash
nmap -sV --script cassandra-info -p 9042 <IP>
cqlsh <IP> 9042                                     # unauth? (default no-auth common)
```
With access: `DESCRIBE KEYSPACES;`, `SELECT * FROM system_auth.credentials;` (hashes).

## CouchDB — 5984/tcp (6984 ssl)
```bash
curl http://<IP>:5984/                              # version/welcome
curl http://<IP>:5984/_all_dbs                      # unauth db list = big finding
curl http://<IP>:5984/_users/_all_docs              # user docs
nmap -sV --script couchdb-databases,couchdb-stats -p 5984 <IP>
```
Findings: CVE-2017-12635 privilege-escalation (JSON `roles` array duplication) precondition —
flag to Exploit if version <1.7/<2.1. Unauth `_all_dbs` = data exposure.

## Elasticsearch — 9200/tcp
```bash
curl http://<IP>:9200/                              # version, cluster name
curl http://<IP>:9200/_cat/indices?v                # index list (unauth?)
curl "http://<IP>:9200/_search?pretty"              # dump default
nmap -p 9200 --script elasticsearch-info <IP>
```
Findings: no auth → full data access; kibana linkage. CVE-2014-3120 (RCE via scripting) &
CVE-2015-1427 preconditions for old versions.

## InfluxDB — 8086/tcp
```bash
curl http://<IP>:8086/ping -v                       # version header
curl -G http://<IP>:8086/query --data-urlencode "q=SHOW DATABASES"   # auth disabled?
```
Findings: **CVE-2019-20933** auth bypass when shared-secret empty (versions <1.7.6) — flag to
Exploit; unauth query API.

## Others (brief)
- **HSQLDB 9001**: default user `sa` empty password; `java -jar hsqldb.jar` GUI. Flag if reachable.
- **Hadoop YARN 8088**: unauth app submission = RCE precondition; check `curl http://<IP>:8088/ws/v1/cluster/info`.
- **Redshift 5439**: psql-compatible; needs AWS creds typically.

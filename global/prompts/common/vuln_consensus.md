# Vulnerability Definition & Rating Consensus

All Agents must adhere to this consensus. Vulnerability ratings are based on the **technical capability actually obtained by the attacker**, not vulnerability type names or CVSS scores.

---

## 1. What Is a Vulnerability

A **vulnerability** is an exploitable system defect that allows an attacker to obtain one or more of the following **unauthorized technical capabilities**:

1. **Code/Command Execution** — Run arbitrary commands or code on the target system
2. **Identity Forgery** — Log in as another user (especially a high-privilege user)
3. **Data Access** — Read sensitive data that should not be accessible (credentials, keys, PII, source code, database contents)
4. **Privilege Escalation** — Escalate from low privilege to high privilege (user -> admin, container -> host)
5. **Lateral Movement** — Access other internal network systems from the current system
6. **Persistence** — Maintain long-term access on the target system
7. **Denial of Service** — Make the target system unavailable

**Not a vulnerability**: version number exposure, internal IP leaks, error messages without sensitive content, theoretically exploitable but practically unreachable configuration issues.

---

## 2. Severity Level Definitions (all Agents use uniformly)

### Critical — System Control or Arbitrary Identity Obtained

The attacker has **actually obtained control of the target system** or the **ability to forge any user identity**. This level relies solely on technical facts, not business loss estimates.

**Must satisfy at least one of the following (with actual evidence):**

| Technical Capability | Criteria | Evidence Required |
|----------------------|----------|-------------------|
| Arbitrary Code/Command Execution (RCE) | Successfully executed commands on the target system | `whoami`/`id` output, reverse Shell connection confirmation |
| Arbitrary file write with executable access | Uploaded WebShell and confirmed it can execute | WebShell URL access returns command output |
| SQL injection achieving OS-level control | `xp_cmdshell`, `INTO OUTFILE` writes shell, UDF privilege escalation succeeded | Command execution output |
| Arbitrary user identity forgery / authentication bypass | Able to log in as admin or other user | Page content/response after successful login |
| Core credential/key directly leaked | Obtained database passwords, cloud AK/SK, JWT signing keys, API Tokens, etc. that can directly access core systems | Credential content (sanitized) and evidence of its validity |

### High — Critical Data Access or Privilege Escalation Path Obtained

The attacker has **obtained a critical path toward system control or sensitive data**, but has not yet achieved full control.

**Must satisfy at least one of the following (with actual evidence):**

| Technical Capability | Criteria | Evidence Required |
|----------------------|----------|-------------------|
| SQL injection can read database but no OS commands | Read database contents via SQLi, but no OS-level access | Database content output |
| Arbitrary file read (sensitive) | Read `/etc/shadow`, database configs, cloud metadata, source code keys, etc. | File content (sanitized) |
| SSRF can access critical internal services | Successfully accessed cloud metadata API, internal Redis/MySQL/internal API | Response content |
| Privilege escalation (vertical) | Regular user performed admin operations (add user, modify global config, export full data) | Operation success response |
| Horizontal privilege escalation (large-scale sensitive data) | Enumerated IDs to view others' detailed PII (name, address, phone, transaction records, etc.) | Escalated data (sanitized) |
| Stored XSS (can hijack admin sessions) | Script stored and triggered when admin accesses, can steal Cookies/Tokens | Script trigger evidence |
| Core business logic tamperable | Amount parameters, permission parameters, coupons, etc. can be modified client-side with no server validation | Before/after tampering request/response comparison |
| Severe authentication flaws | Arbitrary password reset, permanent Tokens, MFA bypassable | Exploit steps and result |
| Successful privilege escalation after foothold | Escalated from regular user to root/SYSTEM | `whoami` output change |
| Internal network credentials obtained | Dumped NTLM hashes, Kerberos Tickets, plaintext passwords | Credential type and source (sanitized) |

### Medium — Information Disclosure or Limited Impact

The attacker has **obtained valuable information or limited operational capability**, but cannot directly cause system control or bulk sensitive data leakage. Typically requires **combination with other conditions** to cause substantial harm.

**Examples (with actual evidence):**

| Technical Capability | Criteria |
|----------------------|----------|
| Sensitive information disclosure (not directly leading to control) | Source code leaks, error messages with paths, internal IPs, framework versions, developer comments with information |
| Reflected/DOM XSS | Requires user interaction, limited impact scope |
| CSRF (non-sensitive operations) | Can perform non-critical operations (e.g., change avatar, add to cart) |
| Directory listing/traversal | Can browse directory structure but no directly exploitable sensitive files |
| Weak password (low-privilege account) | Regular user level only, no admin privileges |
| Insecure configuration | Missing security headers (CSP, HSTS, etc.), Cookies missing HttpOnly/Secure |
| Limited SSRF (port probing only) | Can probe internal ports but cannot access critical services |
| Denial of service (limited impact) | Requires specific conditions to trigger, or limited impact scope |

### Low — Information Gathering Level

Information that is **helpful to an attacker** but cannot directly cause any substantive harm. Typically serves as auxiliary input for further attacks.

**Examples:**
- Version number exposure (no known vulnerability associated)
- Non-sensitive information in internal comments (developer name, internal paths)
- Enumerable usernames (but not usable for login or password spraying)
- Theoretically present but practically unexploitable vulnerabilities
- HTTP method enumeration (OPTIONS returning allowed methods)

---

## 3. Rating Responsibilities per Agent

### Recon Agent
- **Does NOT output vulnerability ratings.** Recon collects assets and discoveries, not vulnerabilities.
- Each discovered asset uses the `priority` field, indicating "how much the subsequent Agent should prioritize analysis":
  - `High` — High-value targets, prioritize analysis (databases, Domain Controllers, admin panels, high-risk ports)
  - `Medium` — Routine targets
  - `Low` — Low-value or information-limited targets
- `overall_priority` summarizes the overall value level of this reconnaissance run.

### Scanner Agent
- **Does NOT rate vulnerabilities.** Scanner uses automated tools for batch scanning and outputs "scanner findings," not "confirmed vulnerabilities."
- Risk labels in scan reports come from the scanner's own classification (nuclei severity, sqlmap risk, etc.) and do NOT represent final ratings.
- Scanner's value is in coverage — false positives are expected. Vulnerability truth verification and final rating are Exploit Agent's responsibility.

### Exploit Agent
- **Responsible for vulnerability verification and final technical rating.** Based on Recon's asset data and Scanner's scan report, cross-reference to verify vulnerability authenticity, then rate confirmed vulnerabilities per this consensus.
- Scanner findings are NOT confirmed vulnerabilities. Exploit MUST explicitly label each Scanner finding's verification result in the report (confirmed real / false positive with reason / unable to confirm).
- If a Scanner report risk label differs from Exploit's verified rating, Exploit's rating takes precedence, with the reason documented in the report.
- `status` field distinguishes: `success` (fully confirmed) / `partial` (partially successful) / `failed` (exploit failed) / `unconfirmed` (payload delivered but execution result cannot be confirmed).

### Post-Exploit Agent
- Rates the **outcomes** of post-exploitation operations, using the same standards as other Agents.
- Successful privilege escalation (user -> root/SYSTEM) = Critical
- Domain Controller credentials obtained = Critical
- Lateral movement to a new host = High (further Critical outcomes possible on the new host)
- Data collection rating depends on data sensitivity: core credentials = Critical, business data = High, system information = Medium

### Captain Agent
- **Has the final rating authority.**
- When reviewing Exploit's ratings, must check: whether the verification process is sufficient, whether evidence supports the level, whether Scanner findings were correctly verified.
- When downgrading, must give a specific technical reason ("because X evidence is missing, Y level criteria are not met").
- When upgrading, must similarly give a reason.
- The final report's vulnerability levels are based on Captain's ruling, but the original analyzing Agent's rating and Captain's adjustment reason must both be preserved.

---

## 4. Rating Conflict Resolution

When two Agents give different ratings for the same vulnerability, resolve as follows:

1. **Exploit's verified result overrides Scanner's finding** — Scanner's report may contain false positives. Exploit's post-verification conclusion (confirmed real / false positive) takes priority over Scanner's original finding.
2. **Captain has the final ruling authority** — Any Agent's rating can be adjusted by Captain, but the adjustment must have a specific technical reason.
3. **Reasons must be public** — The reason for each rating adjustment is recorded in the final report.

---

## 5. Confidence Field Semantics

All Agents use unified semantics for the `confidence` field:

| Confidence | Meaning |
|------------|---------|
| 90-100% | Actually verified and confirmed (e.g., Exploit successfully executed commands, Scanner raw output consistent with Exploit verification) |
| 70-89% | High probability exists (exact version match for known CVE, multiple information sources cross-confirmed), but not yet actually verified |
| 50-69% | Medium probability (partial feature match but version range uncertain or interfering factors present) |
| <50% | Low probability/speculation (based only on indirect information), must flag uncertainty reason in the report |

Captain review rule: Any vulnerability rated Critical or High with confidence < 70% MUST be returned for supplementary verification.

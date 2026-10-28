# Vulnerability Definition & Rating Consensus

**Operator and Worker** must adhere to this consensus. Vulnerability ratings are based on the **technical capability actually obtained by the attacker**, not vulnerability type names or CVSS scores.

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

**Confirmed means reproduced.** A vulnerability is confirmed only when the reproduction is recorded and its output exists in a raw file. An anomaly that has not been reproduced is an observation: record it as unverified with `verification: pending_verification`, never as a confirmed vulnerability or a rated row. This changes nothing in the severity criteria below.

---

## 2. Severity Level Definitions (both roles use them uniformly)

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

## 3. Rating Responsibilities per Role

Two roles exist: **Operator** and **Worker**. Recon, scanning, verification and post-exploitation are task types, not roles. A Worker executes exactly one task type per dispatch.

### Worker
- **Executes one task type.** Recon, scan, verify, or post-exploit. Returns findings with evidence, a **proposed** severity, and `confidence`.
- **No rating authority.** The proposed severity is input to the Operator's ruling, never a rating. Never place a Worker proposal in the report as the vulnerability level.
- **Never present a tool label as a rating.** nuclei severity, sqlmap risk and similar are scanner output, not findings. Quote the label verbatim, state that it is unverified, and in a scan task report a numeric `confidence` no higher than 50 with the verification field set to `pending_verification`.
- **No label substitutes for evidence.** Every finding carries the raw file path plus the command, request/response, or line that proves it.
- **Recon findings are not vulnerabilities.** Rate them by `priority` — `High` (databases, Domain Controllers, admin panels, high-risk ports), `Medium` (routine targets), `Low` (low-value or information-limited) — and summarize the run in `overall_priority`.
- **A verify-task proposal requires reproduction.** State confirmed real / false positive with reason / unable to confirm first, then propose a level. Without reproduction there is no proposal, only "unconfirmed".
- **Post-exploit proposals use the same standards as every other task type.** Privilege escalation to root/SYSTEM = Critical; Domain Controller credentials obtained = Critical; lateral movement to a new host = High (further Critical outcomes possible on the new host); collected data by sensitivity — core credentials = Critical, business data = High, system information = Medium.

### Operator
- **Owns every final rating.** No other role assigns one.
- **Reviews every claim, not only exploitation claims.** A Worker returning "done" proves nothing: read the report file in full, check each claim against a raw output file that exists, check that scanner labels stayed labels, check that `pending_verification` did not survive a verification task.
- **Downgrades or upgrades only with a specific technical reason** naming the evidence that is missing or present and the section 2 criteria that therefore fail or hold. "Too high" is not a reason.
- **Preserves the trail.** Keep the Worker's proposed severity, its `confidence`, and the reason for every change.
- **Gates on confidence.** Any Critical or High proposal below 70% `confidence` goes back for verification before it can stand; until it returns with evidence, it is reported as unverified, not at the proposed level.

### Who Verifies
- The Operator decides that verification is required and what evidence would settle it. It does not rate on suspicion, and it does not accept a Worker's word in place of a raw output file.
- The Worker assigned a verification task performs the reproduction and returns the evidence.
- The Operator re-rates on the returned evidence. A verification task that returns nothing new changes no level.

---

## 4. Rating Conflict Resolution

Conflicts resolve in one direction — tool label, then verified finding, then Operator ruling. Each stage supersedes the one before it.

1. **Tool label versus verified finding — the finding wins.** A scanner label is a coverage artifact and is expected to contain false positives. A verification task's conclusion (confirmed real / false positive with reason / unable to confirm) replaces the label, whether it raises or lowers the level. Both are recorded.
2. **Verified finding versus Operator ruling — the ruling wins.** The Operator may set any level the evidence supports, above or below both the label and the Worker's proposal, with a specific technical reason.
3. **Reasons must be public.** Every change of level is recorded in the final report: from what level, to what level, why, and on which evidence.
4. **Unresolved conflicts do not resolve upward.** If the evidence cannot settle the disagreement, the finding goes back for verification and is reported as unverified — never at the tool's label and never at the higher of the two.

---

## 5. Confidence Field Semantics

Both roles use the same semantics for the `confidence` field:

| Confidence | Meaning |
|------------|---------|
| 90-100% | Actually verified and confirmed (e.g., a verification task executed the command and captured its output, tool raw output consistent with the reproduction) |
| 70-89% | High probability exists (exact version match for known CVE, multiple information sources cross-confirmed), but not yet actually verified |
| 50-69% | Medium probability (partial feature match but version range uncertain or interfering factors present) |
| <50% | Low probability/speculation (based only on indirect information), must flag uncertainty reason in the report |

Operator review rule: Any vulnerability rated Critical or High with confidence < 70% MUST be returned for supplementary verification.

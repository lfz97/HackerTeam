# Passive Intel & Leak Hunting

Passive-only hunting for credentials, secrets and code leaks. Combine breach DBs, code search
platforms, GitHub dorks, paste sites and Google dorks. If you find valid leaked credentials or
API tokens, that is an immediate high-value finding — record it, do not use it.

## 1. Data breach search engines

- GreyNoise Visualizer (viz.greynoise.io) — IP/CIDR lookup, scanner activity by tag/CVE.
- DeHashed — usernames, emails, IPs; API available.
- Have I Been Pwned — email breach/paste checks; API.
- Intelligence X (intelx.io) — emails, domains, URLs, IPs, CIDRs.
- SpyCloud — business email/domain exposure incl. infostealer identities and stolen session cookies.
- WeLeakInfo, LeakCheck, LeakRadar, Leak-Lookup, Scylla.so, Leaked.domains, BreachDirectory.
- Library of Leaks — public documents/companies/people incl. leak datasets.
- InfoStealers (infostealers.info) — infostealer logs from infected devices.
- WhiteIntel — dark-web/credential/infostealer monitoring.
- PSBDMP (psbdmp.ws) — pastebin dump search/monitoring.
- Findemail.io — company email discovery.
- ScamSearch — scammer records by photo/email/username/phone/crypto address.
- CLI aggregator: Leaker (github.com/vflame6/leaker) — searches multiple leak sources by
  email/username/domain/keyword/phone.

## 2. GitHub secret hunting

### Tools

- **TruffleHog v3** — verifies credentials live; scans orgs, issues/PRs, gists, wikis:
  ```bash
  export GITHUB_TOKEN=<token>
  trufflehog github --org Target --results=verified \
    --include-wikis --issue-comments --pr-comments --gist-comments
  ```
- **Gitleaks** — repos/dirs/archives:
  ```bash
  gitleaks git -v --log-opts="--all" <repo>        # full history
  gitleaks dir -v <path>                            # directory
  gitleaks dir -v --max-archive-depth 1 <path>      # archives
  # org-wide:
  gh repo list Target --limit 1000 --json nameWithOwner,url \
  | jq -r '.[].url' | while read -r r; do
    tmp=$(mktemp -d); git clone --depth 1 "$r" "$tmp" && gitleaks dir -v "$tmp" || true; rm -rf "$tmp"
  done
  ```
- **ggshield** (GitGuardian): `ggshield secret scan repo <path-or-url>`, `ggshield secret scan path -r .`
- **Nosey Parker** (archived → replaced by Titus; existing installs):
  `noseyparker scan --datastore np.db <path|repo>` then `noseyparker report --datastore np.db`
- Others: detect-secrets, gitGraber, github-dorks, git-secrets, gittyleaks, GitDorker, RExpository.
- Carlopsolop wrappers: **Leakos** (download all public repos of org + devs, run gitleaks; also
  scans text of URLs), **Pastos** (80+ paste sites), **Gorks** (runs full google-hacking-database).

### Where secrets hide in GitHub

- GitHub Code Search indexes **default branch only** — clone for branches/history.
- Full git history, branches, tags (`git log -p --all` catches removed secrets).
- Issues, PRs, comments, descriptions (TruffleHog flags cover these).
- Actions workflow logs/artifacts (read access suffices; redaction not guaranteed).
- Wikis, release assets, gists.
- GitHub search skips large files; for thoroughness clone + scan locally.
- UI code search supports regex; REST/`gh search code` uses legacy engine without regex.

### Modern token patterns

- GitHub: `ghp_` `gho_` `ghu_` `ghs_` `ghr_` `github_pat_`
- Slack: `xoxb-` `xoxp-` `xoxa-` `xoxs-` `xoxc-` `xoxe-`
- Cloud: `AWS_ACCESS_KEY_ID` `AWS_SECRET_ACCESS_KEY` `aws_session_token`, `GOOGLE_API_KEY`,
  `AZURE_TENANT_ID` `AZURE_CLIENT_SECRET`, `OPENAI_API_KEY` `ANTHROPIC_API_KEY`
- Regex bundle for local ripgrep:
  ```
  (AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,255}|github_pat_[A-Za-z0-9_]{20,255}|AIza[0-9A-Za-z\-_]{35}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY)
  ```

### GitHub dorks (representative selection)

Token/env dorks: `"api_key"` `"api_secret"` `"access_token"` `"auth_token"` `"aws_access_key_id"`
`"aws_secret"` `"client_secret"` `"db_password"` `"database_password"` `"encryption_key"`
`"master_key"` `"private_key"` `"secret_key"` `"slack_token"` `"xoxb "` `"xoxp"`
`"firebase"` `"sendgrid"` `"mailgun"` `"stripe"` `"herokuapp"` `HEROKU_API_KEY language:json`
`shodan_api_key language:python` `PT_TOKEN language:bash` `WORDPRESS_DB_PASSWORD=`
`SECRET_KEY_BASE=` `xoxp OR xoxb OR xoxa` `org:Target "AWS_ACCESS_KEY_ID"` `org:Target "list_aws_accounts"`.

Filename dorks: `filename:.env DB_USERNAME NOT homestead` `filename:.env MAIL_HOST=smtp.gmail.com`
`filename:.npmrc _auth` `filename:.dockercfg auth` `filename:.netrc password` `filename:.ftpconfig`
`filename:.git-credentials` `filename:.htpasswd` `filename:.s3cfg` `filename:.pgpass`
`filename:.bash_history` `filename:id_rsa` `filename:id_rsa or filename:id_dsa`
`filename:wp-config.php` `filename:config.php dbpasswd` `filename:configuration.php JConfig password`
`filename:settings.py SECRET_KEY` `filename:credentials aws_access_key_id`
`filename:secrets.yml password` `filename:shadow path:etc` `filename:logins.json`
`filename:dbeaver-data-sources.xml` `filename:sftp-config.json password`
`[WFClient] Password= extension:ica` `extension:pem private` `extension:ppk private`
`extension:sql mysql dump password` `extension:json mongolab.com` `extension:json googleusercontent client_secret`.

## 3. Wide source-code search platforms

Cross-repo code search for leaks, vulnerable patterns, tech/infra mapping:

- **Sourcebot** (self-hosted, regex/symbol; multi-branch via `rev:` when configured).
- **Sourcegraph** — regex, boolean, symbol, repo/file/language, branch/commit, diff, commit-msg;
  `type:diff` / `type:commit` recover deleted strings without cloning.
- **GitHub Code Search** — regex + qualifiers (`repo:` `org:` `path:` `language:` `symbol:`); default branch only.
- **GitLab Exact Code Search** (Zoekt; default branch, files <1 MB, <20k trigrams) + Advanced
  Search fallback (comments, commits, MRs, wikis).
- **SearchCode**, **grep.app** (1M GitHub repos).

High-signal query ideas (adapt syntax per platform):

```text
org:target path:.github/workflows ("pull_request_target" OR "workflow_run" OR "ACTIONS_STEP_DEBUG")
org:target (path:terraform OR path:helm OR language:HCL OR language:YAML) ("role_arn" OR "assume_role" OR "client_secret" OR "access_key")
org:target ("BEGIN PRIVATE KEY" OR "ghp_" OR "github_pat_" OR "AIza" OR "xoxb-")
org:target (path:.env OR path:values.yaml OR path:application-prod OR path:credentials)
org:target path:.github/workflows ("workflow_call" OR "secrets: inherit" OR "id-token: write" OR "self-hosted")
org:target path:.github/workflows ("uses:" AND NOT /@[0-9a-f]{40}/)
org:target (path:.devcontainer OR path:devcontainer.json) ("remoteEnv" OR "containerEnv" OR "initializeCommand" OR "postCreateCommand" OR "mounts")
org:target ("internal" OR "corp" OR "staging") ("https://" OR "ssh://") NOT path:test
```

Prioritized files: `.github/workflows/*.yml` (pull_request_target/workflow_run triggers, `uses:`
pinned to tags not SHAs), `.devcontainer/*`, `.gitlab-ci.yml`, `azure-pipelines.yml`,
`cloudbuild.yaml`, `Jenkinsfile`, `buildkite*`, `atlantis.yaml`, `terragrunt.hcl`,
`helmfile.yaml`, `skaffold.yaml`, `argocd*`.

### Mass local search when indexed search is insufficient

```bash
gh repo list TARGET_ORG --limit 1000 --json nameWithOwner,sshUrl | jq -r '.[].sshUrl' \
| while read -r repo; do git clone --depth 1 "$repo" "repos/$(basename "$repo" .git)" 2>/dev/null || true; done

rg -n --pcre2 -g '!{.git,node_modules,vendor,dist,build,coverage}' \
  '(AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,255}|github_pat_[A-Za-z0-9_]{20,255}|AIza[0-9A-Za-z\-_]{35}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY)' repos/
```

Search all refs/history of a clone:

```bash
REPO_DIR=repos/some-repo
git -C "$REPO_DIR" fetch --all --tags --prune
git -C "$REPO_DIR" for-each-ref --format='%(refname:short)' refs/remotes/origin refs/tags \
| while read -r ref; do git -C "$REPO_DIR" grep -nI -E 'pull_request_target|workflow_call|id-token: write|secrets: inherit' "$ref" || true; done
git -C "$REPO_DIR" log --all -p -G 'gh[pousr]_|github_pat_|BEGIN [A-Z ]+PRIVATE KEY' -- .
```

Blind spots: default-branch-only indexing; large/vendored/generated files skipped; comments,
issues, PRs, gists, wikis often out of scope; devcontainer configs can be branch-specific.

## 4. Google dorks

Use the google-hacking-database (exploit-db) with a tool like Gorks (thousands of queries;
plain-browser automation gets blocked). Hand-picked classics:
`intitle:"index of" passwords`, `inurl:admin`, `filetype:sql "INSERT INTO"`,
`inurl:wp-config.php`, `site:target.com -www`, `intitle:"Dashboard" inurl:admin`,
`ext:log password`.

## 5. Public code vulnerability review

If the org has open-source code, review it with SAST tools (per language) and free services
like Snyk (app.snyk.io) for public repos.

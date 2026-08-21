# Mail & VoIP Services — Scanner Cheatsheet

## SMTP — 25/465/587/tcp
```bash
nc -vn <IP> 25
openssl s_client -starttls smtp -connect <IP>:587    # STARTTLS banner
openssl s_client -crlf -connect <IP>:465             # implicit TLS
nmap -p25 --script smtp-commands <IP>                 # EHLO feature list
nmap -p25 --script smtp-open-relay <IP>               # open relay check (top finding)
nmap -p25 --script smtp-ntlm-info <IP>                # NTLM info disclosure (Windows version)
```
User enumeration (run against authorized targets):
```bash
smtp-user-enum -M VRFY -U users.txt -t <IP>
smtp-user-enum -M RCPT -U users.txt -t <IP>
smtp-user-enum -M EXPN -U users.txt -t <IP>
nmap --script smtp-enum-users <IP>
msf> use auxiliary/scanner/smtp/smtp_enum
```
Findings: **open relay**, VRFY/EXPN/RCPT user enum, NTLM info disclosure, SMTP smuggling
precondition (bare-LF / BDAT — flag modern CVE-2023-51766 Exim / CVE-2023-51765 Sendmail).

## POP3 — 110/995/tcp
```bash
nc -nv <IP> 110
openssl s_client -connect <IP>:995 -crlf -quiet
nmap --script "pop3-capabilities or pop3-ntlm-info" -sV -p 110 <IP>
```
Commands after login (authorized): USER/PASS, STAT, LIST, RETR n. NTLM info disclosure via
`pop3-ntlm-info`. Brute-force: brute-force.md.

## IMAP — 143/993/tcp
```bash
nc -nv <IP> 143
openssl s_client -starttls imap -connect <IP>:143 -crlf -quiet
nmap --script "imap-capabilities or imap-ntlm-info" -sV -p 143 <IP>
```
With creds: `A1 LOGIN user pass`, `A1 LIST "" *`, `A1 SELECT INBOX`, `A1 SEARCH ALL`,
`A1 FETCH n BODY[]`. curl IMAP: `curl -u user:pass 'imap://<IP>/INBOX'`.

## SIP / VoIP — 5060/udp+tcp (5061 tls)
```bash
sudo nmap --script=sip-methods -sU -p 5060 <IP>       # allowed methods
sippts scan -i <IP>/32 -p all -r 5060-5080            # sippts discovery
sippts enumerate -i <IP>                              # methods enum
svwar <IP> -p5060 -e100-300 -m REGISTER               # extension enum (svmap/svwar)
sipsak / sipvicious for fingerprinting
msf> use auxiliary/scanner/sip/options ; auxiliary/scanner/sip/enumerator
```
Findings: unauthenticated REGISTER/INVITE (toll fraud path), extension enumeration success,
weak SIP digest (capture via svcrack - authorized only). Hand exploitation to Exploit.

## General mail notes
- All mail protocols cleartext by default → flag weak-crypto when TLS not enforced.
- NTLM auth on SMTP/POP/IMAP leaks Windows version/hostname → info-disclosure finding.

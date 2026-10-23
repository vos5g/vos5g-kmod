#!/usr/bin/env bash
# Is it safe to build CONFIG_<NAME> as a module for the running kernel?
#
#   ./tools/check-option.sh NF_TABLES XT_MATCH_OWNER ...   (after ./build.sh has fetched the tree)
#
# A module is only safe if turning the option on doesn't change anything the
# already-built kernel shares with it: struct layouts, inline helpers, netns
# members. This lists every header that tests the option; any hit needs reading
# (e.g. CONFIG_NF_TABLES adds `struct netns_nftables nft` to struct net, so
# nftables can't be a module here). Also run ./build.sh afterwards: it refuses
# to build if a built-in (=y) option would change.
set -euo pipefail
K=$(cd "$(dirname "$0")/.." && pwd)/build/kmod/msm-4.14
[ -d "$K/include" ] || { echo "run ./build.sh once first (it fetches the kernel tree)" >&2; exit 1; }
cfg=$(gzip -dc "$(dirname "$0")/../config/config-4.14.117-perf.gz")
for o in "$@"; do
  o=${o#CONFIG_}
  cur=$(printf '%s\n' "$cfg" | grep -E "^(# )?CONFIG_$o[= ]" || echo "(not in config)")
  refs=$(grep -rlE "(IS_ENABLED|IS_BUILTIN|IS_MODULE|defined|ifdef|ifndef)\(?\s*CONFIG_${o}(_MODULE)?\b" "$K/include" 2>/dev/null \
         | grep -v "/include/config/\|/include/generated/" | sed "s#$K/##" || true)
  if [ -z "$refs" ]; then
    printf '%-34s %-34s no header references: safe as a module\n' "$o" "$cur"
  else
    printf '%-34s %-34s CHECK these headers:\n' "$o" "$cur"; printf '    %s\n' $refs
  fi
done

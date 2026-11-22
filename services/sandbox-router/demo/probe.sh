#!/bin/sh
# Offline fixture for the launch contract. Treat stdin only as data.
set -eu
test "$(id -u)" = 65532
grep -Eq '^CapEff:[[:space:]]*0+$' /proc/self/status
grep -Eq '^NoNewPrivs:[[:space:]]*1$' /proc/self/status
test "$(ls /sys/class/net)" = lo
awk '$5 == "/" && $6 ~ /^ro(,|$)/ {found=1} END {exit !found}' /proc/self/mountinfo
test ! -S /var/run/docker.sock
test "$PWD" = /work
cat > input.txt
printf 'Read %s bytes in disposable workspace; kernel %s\n' "$(wc -c < input.txt)" "$(uname -r)"

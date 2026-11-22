#!/usr/bin/env bash
# Run from a source checkout; all host data and output are disposable.
set -euo pipefail
cd "$(dirname "$0")"
if [[ $(podman info --format '{{.Host.Security.Rootless}} {{.Host.CgroupsVersion}}') != 'true v2' ]]; then
  echo 'Requires rootless Podman and cgroup v2.' >&2
  exit 1
fi

umask 077
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/input"
echo 'disposable credential fixture' > "$work/immich.key"
# A synthetic 2x1 RGB PNG. Use only the Python standard library on the host.
python3 - "$work/input/photo.png" <<'PY'
import struct, sys, zlib
def chunk(kind, body):
    return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))
with open(sys.argv[1], "wb") as f:
    f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 2, 1, 8, 2, 0, 0, 0)) +
            chunk(b"IDAT", zlib.compress(bytes([0, 255, 0, 0, 0, 255, 0]))) + chunk(b"IEND", b""))
PY

for target in worker probe; do
  podman build --target "$target" --iidfile "$work/$target.id" -f Dockerfile .
done

args=(--rm --pull=never --network=none --http-proxy=false
      --read-only --read-only-tmpfs=false --cap-drop=ALL
      --security-opt=no-new-privileges --userns=keep-id --user "$(id -u):$(id -g)"
      --memory=128m --memory-swap=128m --cpus=1 --pids-limit=64 --timeout=20
      -v "$work/input:/input:ro,Z")
export IMMICH_API_KEY=disposable-environment-canary
podman run "${args[@]}" --env "CANARY_PATH=$work/immich.key" "$(cat "$work/probe.id")"
# Keep the host destination outside the container. Bound even unexpected output.
podman run "${args[@]}" "$(cat "$work/worker.id")" | head -c 131073 > "$work/result.png"
python3 - "$work/result.png" <<'PY'
import pathlib, struct, sys
b = pathlib.Path(sys.argv[1]).read_bytes()
if (len(b) > 131072 or not b.startswith(b"\x89PNG\r\n\x1a\n\0\0\0\rIHDR")
        or struct.unpack(">II", b[16:24]) != (128, 128) or b[-8:] != b"IEND\xaeB`\x82"):
    sys.exit("invalid thumbnail export")
PY
echo 'PASS: 128x128 PNG exported; sandbox boundary checks passed.'

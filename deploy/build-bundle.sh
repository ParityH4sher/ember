#!/usr/bin/env bash
# Assemble only committed source. Works from any directory in a source checkout.
set -euo pipefail
if (( $# < 1 || $# > 2 )); then
  echo "Usage: bash deploy/build-bundle.sh OUTPUT_DIRECTORY [GIT_REVISION]" >&2
  exit 2
fi
export LC_ALL=C
repo=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
commit=$(git -C "$repo" rev-parse --verify --end-of-options "${2:-HEAD}^{commit}")
epoch=$(git -C "$repo" show -s --format=%ct "$commit")
bundle="ember-${commit:0:12}"
mkdir -p -- "$1"
out=$(cd -- "$1" && pwd)
work=$(mktemp -d "$out/.ember-build.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/$bundle"
git -C "$repo" archive "$commit" -- apps/api apps/frontends/web deploy/gateway deploy/compose.yaml deploy/compose.oidc.yaml deploy/compose.redis.yaml deploy/compose.edge.yaml deploy/compose.agents.yaml \
  deploy/compose.models.yaml deploy/compose.opencode.yaml deploy/litellm deploy/opencode deploy/install.sh deploy/bundle-readme.md |
  tar -xf - -C "$work/$bundle"
cd "$work"
mv "$bundle/deploy/install.sh" "$bundle/install.sh"
mv "$bundle/deploy/bundle-readme.md" "$bundle/README.md"
chmod +x "$bundle/install.sh"
printf '%s\n' "$commit" > "$bundle/apps/api/version.txt"
# File order, time, owner, modes and gzip header must not depend on the
# checkout, machine, invoking user, or when a CI rerun occurs.
tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner \
  --mode='u=rwX,go=rX' --format=gnu -cf - "$bundle" | gzip -n > "$bundle.tar.gz"
sha256sum "$bundle.tar.gz" > "$bundle.tar.gz.sha256"
for file in "$bundle.tar.gz" "$bundle.tar.gz.sha256"; do
  if [[ -e "$out/$file" ]] && ! cmp -s -- "$out/$file" "$file"; then
    echo "Refusing to replace existing, different $file" >&2
    exit 1
  fi
done
for file in "$bundle.tar.gz" "$bundle.tar.gz.sha256"; do
  # Publish without replacing a file created concurrently after the precheck.
  ln -- "$file" "$out/$file" 2>/dev/null || cmp -s -- "$out/$file" "$file" || {
    echo "Could not publish $file without replacing existing data" >&2
    exit 1
  }
done
echo "Bundle: $out/$bundle.tar.gz"

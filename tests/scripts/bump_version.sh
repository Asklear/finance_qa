#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT_DIR"

new_version="${1:-}"
if [[ ! "$new_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: tests/scripts/bump_version.sh <semver>" >&2
  exit 1
fi

old_version="$(sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/buildinfo/version.go)"
if [[ -z "$old_version" ]]; then
  echo "could not read current version from internal/buildinfo/version.go" >&2
  exit 1
fi

update_json_version() {
  local path="$1"
  node - "$new_version" "$path" <<'NODE'
const fs = require("fs");
const version = process.argv[2];
const path = process.argv[3];
const doc = JSON.parse(fs.readFileSync(path, "utf8"));
doc.version = version;
fs.writeFileSync(path, JSON.stringify(doc, null, 2) + "\n");
NODE
}

perl -0pi -e 's/const Version = "[^"]+"/const Version = "'$new_version'"/' internal/buildinfo/version.go
update_json_version plugin/openclaw-finance/package.json
update_json_version plugin/openclaw-finance/openclaw.plugin.json

for path in \
  plugin/openclaw-finance/server/README.md \
  docs/architecture/03-deployment-runtime.md \
  internal/mcp/server_test.go \
  tests/integration/openclaw_finance_plugin_test.go
do
  OLD_VERSION="$old_version" NEW_VERSION="$new_version" perl -0pi -e 's/\Q$ENV{OLD_VERSION}\E/$ENV{NEW_VERSION}/g' "$path"
done

VERSION_PRECHECK_SKIP_DIFF="${VERSION_PRECHECK_SKIP_DIFF:-1}" "$ROOT_DIR/tests/scripts/check_version_preflight.sh"
echo "bumped financeqa version to $new_version"

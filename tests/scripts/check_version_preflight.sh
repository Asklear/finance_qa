#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$ROOT_DIR"

fail() {
  echo "version preflight failed: $*" >&2
  exit 1
}

read_json_version() {
  local path="$1"
  node -e "const fs=require('fs'); const p=process.argv[1]; const doc=JSON.parse(fs.readFileSync(p,'utf8')); if (!doc.version) process.exit(2); process.stdout.write(String(doc.version));" "$path"
}

read_go_version() {
  sed -nE 's/^const Version = "([^"]+)"/\1/p' internal/buildinfo/version.go
}

require_file() {
  local path="$1"
  [[ -f "$path" ]] || fail "missing required version surface: $path"
}

for path in \
  internal/buildinfo/version.go \
  plugin/openclaw-finance/package.json \
  plugin/openclaw-finance/openclaw.plugin.json \
  plugin/openclaw-finance/server/README.md \
  docs/architecture/03-deployment-runtime.md \
  internal/mcp/server_test.go \
  tests/integration/openclaw_finance_plugin_test.go
do
  require_file "$path"
done

go_version="$(read_go_version)"
package_version="$(read_json_version plugin/openclaw-finance/package.json)"
manifest_version="$(read_json_version plugin/openclaw-finance/openclaw.plugin.json)"

[[ -n "$go_version" ]] || fail "could not read Go build version"
[[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "Go build version is not semver: $go_version"
[[ "$package_version" == "$go_version" ]] || fail "package.json version $package_version does not match Go version $go_version"
[[ "$manifest_version" == "$go_version" ]] || fail "openclaw.plugin.json version $manifest_version does not match Go version $go_version"

for path in \
  plugin/openclaw-finance/server/README.md \
  docs/architecture/03-deployment-runtime.md \
  internal/mcp/server_test.go \
  tests/integration/openclaw_finance_plugin_test.go
do
  if ! grep -q "$go_version" "$path"; then
    fail "$path does not mention current version $go_version"
  fi
done

if [[ "${VERSION_PRECHECK_SKIP_DIFF:-0}" == "1" ]]; then
  echo "version preflight ok: version $go_version"
  exit 0
fi

changed_tmp="$(mktemp "${TMPDIR:-/tmp}/financeqa-version-changed.XXXXXX")"
trap 'rm -f "$changed_tmp"' EXIT

if [[ -n "${VERSION_PRECHECK_CHANGED_FILES:-}" ]]; then
  printf '%s\n' "$VERSION_PRECHECK_CHANGED_FILES" >>"$changed_tmp"
fi

if [[ "${VERSION_PRECHECK_SKIP_GIT:-0}" != "1" ]] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  base_ref="${VERSION_PRECHECK_BASE_REF:-}"
  if [[ -z "$base_ref" ]] && git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
    base_ref="@{u}"
  fi
  if [[ -n "$base_ref" ]] && git rev-parse --verify "$base_ref" >/dev/null 2>&1; then
    git diff --name-only "$base_ref"...HEAD >>"$changed_tmp" || git diff --name-only "$base_ref" HEAD >>"$changed_tmp"
  fi
  git diff --name-only >>"$changed_tmp"
  git diff --cached --name-only >>"$changed_tmp"
  git ls-files --others --exclude-standard >>"$changed_tmp"
fi

runtime_changed=0
version_changed=0
runtime_examples=()

while IFS= read -r path; do
  [[ -n "$path" ]] || continue
  case "$path" in
    *_test.go)
      continue
      ;;
  esac
  case "$path" in
    internal/buildinfo/version.go|plugin/openclaw-finance/package.json|plugin/openclaw-finance/openclaw.plugin.json)
      version_changed=1
      ;;
  esac
  case "$path" in
    cmd/*|internal/*|plugin/openclaw-finance/dist/*|plugin/openclaw-finance/server/*|SKILL.md|docs/SKILL_APPENDIX_FULL.md|tests/scripts/sync_openclaw_bridge_and_skill.sh|tests/scripts/claude_finance_final_answer.sh|tests/scripts/run_online_agent_final_answer_check.py)
      runtime_changed=1
      if [[ "${#runtime_examples[@]}" -lt 5 ]]; then
        runtime_examples+=("$path")
      fi
      ;;
  esac
done < <(sort -u "$changed_tmp")

if [[ "$runtime_changed" == "1" && "$version_changed" != "1" ]]; then
  echo "runtime changed but version was not bumped" >&2
  for path in "${runtime_examples[@]}"; do
    echo "- $path" >&2
  done
  echo "bump one of the canonical version files, normally via tests/scripts/bump_version.sh <version>" >&2
  exit 1
fi

echo "version preflight ok: version $go_version"

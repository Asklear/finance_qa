#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

/opt/homebrew/bin/go run -tags scriptmain tests/scripts/run_realdata_question_suite.go \
  -company "南京优集数据科技有限公司" \
  -suite tests/testdata/top20_questions_2026-04-14.json \
  -report scratch/reports/2026-04-14-20问真实数据测试报告.md \
  -title "20道老板高频财务问题真实数据测试报告"

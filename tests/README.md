# 测试目录规范

## 目标

本仓库采用混合测试布局：

- 包内测试负责贴近实现的验证。
- `tests/` 目录负责黑盒、集成、业务准确性回归与测试资产。

这样做是为了兼顾两件事：

- 让核心 package 可以就近维护、快速运行。
- 让跨模块场景、真实数据题库、宿主桥接契约有稳定的统一入口和统一命令。

## 常用命令

优先使用 `Makefile`，它已经设置了 Go 原生并行参数：

```bash
make test-fast        # 快速本地门禁：internal + tests/unit + tests/testutil（使用 -short 跳过重型 query 黑盒）
make test-query-heavy # 重型 query 黑盒：合同/实体/来源链路全流程
make test-integration # CLI / bridge / 跨模块集成，不默认要求真实库
make test-business    # 真实库业务准确性：老板问题、金额、来源、trace
make test-live        # integration 下需要真实库的 smoke/契约
make test-full        # go test ./...，默认不包含 accuracy build tag
```

如果本机安装了 `gotestsum`，可以临时替换输出：

```bash
GO_TEST="gotestsum --format testname --" make test-fast
```

默认快测不连接线上库。真实库测试统一要求：

```bash
FINANCEQA_RUN_LIVE_DB_TESTS=1 go test -tags accuracy -p 4 -parallel 8 ./tests/business -count=1
```

## 放置规则

### 1. 放在源码同目录

适用场景：

- 测单个 package 的内部逻辑。
- 需要访问未导出函数、结构或辅助方法。
- 需要紧跟实现细节一起维护。

示例：

- `internal/accounting/calculator_test.go`
- `internal/query/supplier_payment_filter_test.go`
- `internal/openitems/pairing_pgcompat_test.go`

### 2. 放在 `tests/unit/`

适用场景：

- 黑盒单元测试。
- 规则/配置/路由契约测试。
- 轻量级跨 package 行为验证。
- 不需要真实数据库、不依赖线上环境。

示例：

- `tests/unit/query/intent_period_company_test.go`
- `tests/unit/query/host_payload_test.go`
- `tests/unit/config/keywords_manager_test.go`

### 3. 放在 `tests/business/`

适用场景：

- 老板问题题库。
- 真实库金额准确性。
- 来源表、来源更新时间、计算 trace 断言。
- 需要和 Excel / 独立 SQL 口径核对的业务回归。

要求：

- 文件必须带 `//go:build accuracy`。
- 用 `tests/testdata/` 的 JSON/YAML case 驱动，避免一题一个手写测试函数。
- 子测试能并行的必须 `t.Parallel()`。
- 运行入口是 `make test-business`。

示例：

- `tests/business/contract_first_accuracy_test.go`
- `tests/business/realdata_question_suites_accuracy_test.go`

### 4. 放在 `tests/integration/`

适用场景：

- 跨模块集成。
- bridge / host summary 契约。
- CLI、导入流水线、宿主桥接。
- PostgreSQL schema / live smoke / 发布前连通性验收。

不要放：

- 老板问题金额准确性题库。
- 长篇真实业务问题回归。

示例：

- `tests/integration/finance_bridge_contract_test.go`
- `tests/integration/prod_live_test.go`

### 5. 放在 `tests/testutil/`

适用场景：

- 黑盒测试公共 helper。
- JSON path 断言。
- live DB / engine 初始化。
- fixture copy/cache。

不要放：

- 业务测试用例。
- 生产代码。

### 6. 放在 `tests/testdata/`

仅放：

- JSON 题库
- 脱敏样本
- 参考输出

不要放：

- 可执行脚本
- 业务代码
- 临时人工分析文件

### 7. 放在 `tests/scripts/`

仅放：

- 批量回归脚本
- 发布/同步校验脚本
- 测试驱动工具

不要放：

- 常规 `go test` 用例
- 长期无人维护的临时脚本

## 判断顺序

新增一个测试文件前，按这个顺序判断：

1. 是否强依赖某个 package 的内部实现？
2. 是否是外部调用者视角的黑盒验证？
3. 是否是老板问题/真实库金额准确性回归？
4. 是否跨模块、跨进程、跨数据库或跨宿主？
5. 是否只是测试数据、公共 helper 或测试脚本？

对应结论：

- `是 1`：放源码同目录。
- `是 2`：放 `tests/unit/`。
- `是 3`：放 `tests/business/`，并加 `accuracy` build tag。
- `是 4`：放 `tests/integration/`。
- `是 5`：放 `tests/testutil/`、`tests/testdata/` 或 `tests/scripts/`。

## 并行策略

- package 级并行使用 `go test -p N`。
- 用例级并行使用 `t.Parallel()`。
- 数据驱动题库必须用 `t.Run` 子测试，独立 case 加 `t.Parallel()`。
- 使用 `t.Setenv`、修改进程级环境变量、共享外部状态的测试不要并行。
- 真实库准确性测试可以并行，但要保证只读查询，不在测试里写线上库。
- `tests/unit/query` 中调用完整 `Engine.Query` 且耗时较长的黑盒用例应调用 `runParallelHeavyQueryTest(t)` 或 `runParallelQueryScenarios`，这样 `go test -short` 会跳过它们。
- 轻量规则、解析、配置、纯函数测试不要标重型，应该留在 `make test-fast` 内。

## 当前约定

- 不要求把所有 `*_test.go` 统一搬进 `tests/`。
- 不新增“只是为了看起来整齐”的测试迁移。
- 新测试优先按职责落位，而不是按文件后缀统一收纳。
- `internal/` 下可保留 package 内测试；可执行脚本不得放在 `internal/`。
- 老板问题准确性统一进入 `tests/business/`，不要继续散落在 `tests/integration/`。

## 维护建议

- 提交前优先跑 `make test-fast`。
- 改 query 全链路、来源展示、合同/实体路由时补跑 `make test-query-heavy`。
- 改 CLI / bridge 时补跑 `make test-integration`。
- 改业务路由、实体识别、金额口径时必须跑 `make test-business`。
- 需要生成报告或做发布审计时再走 `tests/scripts/` 下脚本。
- 误入库的系统文件（如 `.DS_Store`）不要保留在 `tests/`。

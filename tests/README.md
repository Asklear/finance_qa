# 测试目录规范

## 目标

本仓库采用混合测试布局：

- 包内测试负责贴近实现的验证。
- `tests/` 目录负责黑盒、集成、回归与测试资产。

这样做是为了兼顾两件事：

- 让核心 package 可以就近维护、快速运行。
- 让跨模块场景、真实数据题库、宿主桥接契约有稳定的统一入口。

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

### 3. 放在 `tests/integration/`

适用场景：

- 跨模块集成。
- bridge / host summary 契约。
- CLI、导入流水线、真实题库回归。
- PostgreSQL / 线上 smoke / 发布前验收。

示例：

- `tests/integration/finance_bridge_contract_test.go`
- `tests/integration/realdata_question_suites_test.go`
- `tests/integration/prod_live_test.go`

### 4. 放在 `tests/testdata/`

仅放：

- JSON 题库
- 脱敏样本
- 参考输出

不要放：

- 可执行脚本
- 业务代码
- 临时人工分析文件

### 5. 放在 `tests/scripts/`

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
3. 是否跨模块、跨进程、跨数据库或跨宿主？
4. 是否只是测试数据或测试脚本？

对应结论：

- `是 1`：放源码同目录。
- `是 2`：放 `tests/unit/`。
- `是 3`：放 `tests/integration/`。
- `是 4`：放 `tests/testdata/` 或 `tests/scripts/`。

## 当前约定

- 不要求把所有 `*_test.go` 统一搬进 `tests/`。
- 不新增“只是为了看起来整齐”的测试迁移。
- 新测试优先按职责落位，而不是按文件后缀统一收纳。

## 维护建议

- 提交前优先跑 `go test ./... -count=1`。
- 真实数据回归继续走 `tests/scripts/` 下脚本。
- 误入库的系统文件（如 `.DS_Store`）不要保留在 `tests/`。

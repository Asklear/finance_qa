# calc_plan 协议定义（最小可用版）

本文定义 `calc_plan` 的最小协议，用于描述一组可执行的财务计算步骤。当前版本只保证结构稳定，不绑定具体执行引擎。

## 1. 协议目标

`calc_plan` 的作用是把一次计算拆成三类信息：

1. `variables`：参与计算的输入变量。
2. `formulas`：基于变量计算出的表达式。
3. `checks`：对输入或中间结果做的校验。

执行后，返回 `execution_result`，用于描述是否成功、计算结果和失败原因。

## 2. 字段定义

### 2.1 Variable

`Variable` 描述一个可被公式引用的值。

字段：

- `name`：变量名，必须唯一，且应能被公式直接引用。
- `value`：变量值，最小版本建议使用数值或字符串承载原始值。
- `unit`：单位，可选，例如 `CNY`、`pct`、`count`。
- `source`：来源说明，可选，例如 `journal.total_income`。
- `description`：变量含义，可选。

### 2.2 Formula

`Formula` 描述一个计算表达式。

字段：

- `name`：公式名，必须唯一。
- `expression`：计算表达式，最小版本使用字符串保存，例如 `income - cost`。
- `description`：公式说明，可选。
- `output`：输出变量名，可选；若省略，则默认结果与 `name` 同名。

### 2.3 Check

`Check` 描述执行前后需要验证的规则。

字段：

- `name`：校验名，必须唯一。
- `expression`：布尔校验表达式，例如 `income >= 0`。
- `severity`：严重级别，建议值为 `error` 或 `warning`。
- `message`：失败提示，可选。

### 2.4 ExecutionResult

`ExecutionResult` 描述一次 `calc_plan` 执行结果。

字段：

- `success`：是否成功。
- `message`：执行摘要或失败说明。
- `outputs`：公式输出结果集合。
- `failed_checks`：未通过的校验项。
- `trace`：可选的执行过程说明，便于排查。

## 3. 示例

### 3.1 计划示例

```json
{
  "variables": [
    {
      "name": "income",
      "value": 120000,
      "unit": "CNY",
      "source": "journal.revenue",
      "description": "当期确认收入"
    },
    {
      "name": "cost",
      "value": 80000,
      "unit": "CNY",
      "source": "journal.cost",
      "description": "当期确认成本"
    }
  ],
  "formulas": [
    {
      "name": "gross_profit",
      "expression": "income - cost",
      "output": "gross_profit",
      "description": "毛利润"
    }
  ],
  "checks": [
    {
      "name": "income_non_negative",
      "expression": "income >= 0",
      "severity": "error",
      "message": "收入不能为负数"
    }
  ]
}
```

### 3.2 执行结果示例

```json
{
  "success": true,
  "message": "calc_plan executed successfully",
  "outputs": {
    "gross_profit": 40000
  },
  "failed_checks": [],
  "trace": [
    "check income_non_negative passed",
    "formula gross_profit = 120000 - 80000"
  ]
}
```

## 4. 失败返回约定

最小可用版本统一使用 `ExecutionResult` 表达失败，不单独定义错误包。

约定如下：

1. `success=false` 表示执行失败。
2. `message` 必须给出可读的失败原因。
3. `failed_checks` 应列出触发失败的校验项；若是解析失败、缺字段或表达式错误，也应尽量给出对应条目。
4. `outputs` 在失败时可以为空或只保留已完成的部分结果。
5. 对于无法继续执行的错误，`trace` 可保留最后一步上下文，方便调试。

常见失败场景：

- 缺少必需变量。
- 公式表达式无法解析。
- 校验表达式为假且 `severity=error`。
- 中间结果出现空值或非法值。

## 5. 最小兼容原则

1. 只要求字段稳定，不要求表达式引擎固定。
2. 变量、公式、校验都用字符串作为可扩展边界。
3. 后续如果接入真正的计算执行器，可以在不破坏本协议的前提下增加字段。

# AGENTS.md — oagentkit/tools

> 外部工具接入——CLI 编码 agent 统一调用/公开资料检索/git 工作副本沙箱/确定性 SAST 扫描/Shell 命令静态裁决
> module：`github.com/oagentkit/tools` · org：[oagentkit = Open Agent Kit](https://github.com/oagentkit)
> 矩阵分层：**tier 2**（集成层，依赖 tier0/tier1 + 重第三方地板）

## 本仓包含

```
`acpx/`
`websearch/`
`workcopy/`
`sast/`
`bashguard/`  ← 自 guardx 迁入（v0.10.0）：Shell 命令静态裁决，依赖 mvdan.cc/sh
```

## 硬规则

### 分层与依赖

- 依赖严格自底向上，**禁止反向依赖**。本仓是 tier 2，只允许依赖 tier ≤ 2 的仓。
- 本仓依赖的矩阵内仓：`guardx`、`runtime`、`textx`。
  ⚠️ guardx 是 tier 0 零依赖底层（`workcopy`/`sast` 用 `guardx/logredact`），
  本仓 tier 2 —— 依赖方向合法。
- 改 module 路径必须同步：全部 import、go.mod、README 安装行。

### 错误处理

- 一律使用 `github.com/oekit/pkg/errors`；业务错误用 BizError。
- 所有 error 禁止 `_` 忽略。

### 并发安全

- 禁止裸 `go func`：用 `github.com/oekit/concurrency/async` 的 GoSafe 系列（panic 被 recover）。
- 共享状态用 mutex / atomic 保护；包级全局可变状态用 `atomic.Pointer[T]`。

### API 设计

- 函数选项模式命名 `XxxOption`（禁止裸 `Option`，多包 co-import 会冲突）。
- 导出符号 PascalCase + godoc 注释；包文档放 `doc.go`。

### 日志

- 日志走 `oekit/logx`，组件 logger 用 `logx.Delegating("oagentkit-tools")`。
- 子进程输出落日志前过 `guardx/logredact` —— 外部 CLI 的输出不可信。

### 发版顺序

本仓的 `CHANGELOG.md` 由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，
**不要手改**。发版顺序的**单一副本**见 org 规则仓
[CONTRIBUTING §发版](https://github.com/oagentkit/.github/blob/main/CONTRIBUTING.md#发版)：
**预写待发版段 → 提交 CHANGELOG.md → 在那个提交上打 annotated tag**。
tag 必须落在 CHANGELOG 提交**之后** —— 下游 `go get` 拿到的是 tag 快照，
快照里的 CHANGELOG.md 若缺自己那一段，发布说明就是残缺的。

⚠️ 无参重生成 CHANGELOG 会**吞掉待发版段**：生成器只按已存在的 tag 切段，
`--next-version` 是唯一能产出待发版段的开关。**预写之后、打 tag 之前，
不要跑无参重生成。**

打 tag 前自检依赖地板：内部 `require` 要钉在**打 tag 那一刻**各依赖仓的最新 tag
（`matrix-tools/scripts/verify_tag_floor.py`）。本仓已有 1 个 CHANGELOG-at-tag 欠账
（`tools@v0.10.0`）—— **反面例子，不是范例**。

## 仓特有纪律

- **CLI 编码 agent 调用必须隔离环境**（用 `runtime.ChildEnv` 构造最小环境），
  不得继承完整宿主环境 —— 凭据泄漏面。
- `workcopy` 的 git 操作必须限制在沙箱目录内，`EnsureMergeBase` 失败要显式报错，
  不得跳过继续。
- SAST 扫描是**确定性前置扫描**，不执行被扫代码。

## 明确不属于本仓的

| 能力 | 归属 |
|---|---|
| 文本/差异/会话/报告 | `github.com/oagentkit/textx` |
| 事件总线/流控/子进程/热插拔/钩子 | `github.com/oagentkit/runtime` |
| 脱敏/出口围栏/权限/沙箱解析/审计 | `github.com/oagentkit/guardx` |
| DB 队列与选主 | `github.com/oagentkit/queue` |
| 通用 errors/stringx/encoding/mathx | `github.com/oekit/pkg` |
| goroutine 池/单飞/重试/令牌桶 | `github.com/oekit/concurrency` |

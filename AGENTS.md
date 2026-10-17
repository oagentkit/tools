# AGENTS.md — oagentkit/tools

> 外部工具接入——CLI 编码 agent 统一调用/公开资料检索/git 工作副本沙箱/确定性 SAST 扫描
> module：`github.com/oagentkit/tools` · org：[oagentkit = Open Agent Kit](https://github.com/oagentkit)
> 矩阵分层：**tier 2**（集成层，依赖 tier0/tier1 + 重第三方地板）

## 本仓包含

```
`acpx/`
`websearch/`
`workcopy/`
`sast/`
```

## 硬规则

### 分层与依赖

- 依赖严格自底向上，**禁止反向依赖**。本仓是 tier 2，只允许依赖 tier ≤ 2 的仓。
- 本仓依赖的矩阵内仓：`guardx`、`runtime`、`textx`。
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

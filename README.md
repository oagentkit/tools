# tools

外部工具接入——CLI 编码 agent 统一调用/公开资料检索/git 工作副本沙箱/确定性 SAST 扫描/Shell 命令静态裁决 —— oagentkit 模块矩阵 **tier 2**（集成层，依赖 tier0/tier1 + 重第三方地板）。

```
go get github.com/oagentkit/tools
```

| 包 | 职责 |
|---|---|
| `acpx` | CLI 编码 agent 统一调用（claude / codex / gemini / kimi / mcode 等） |
| `websearch` | 公开资料检索抽象，接口可插拔（缺省 SearXNG 自建实例） |
| `workcopy` | Git 工作副本沙箱：隔离环境 + merge-base 校验 |
| `sast` | 确定性 SAST 前置扫描（不执行被扫代码） |
| `bashguard` | Shell 命令静态风险裁决（自 `guardx/bashguard` 迁入 v0.10.0）：经真 shell AST 判定只读性，**不执行命令** |
| `bgjob` | 远程后台作业 marker 协议原语：经宿主既有 SSH 会话起脱离会话的长任务，退出码 marker 探测（running/finished+退出码）与就地收尾（RUNNING 拒清、REMOVED 自证） |

**bashguard 作为命令黑名单判定底座**（oagentkit/tools#38）：下游自持正则黑名单
（`rm -rf` / `mkfs` / `shutdown` 一族的整串锚点）会把**引号内的字面量**误杀
（实弹：`grep -iE 'sleep|shutdown'` 被整串锚点判定为含 shutdown，两轮全量评测
连环失败）；AST 判定不会把引号内字面量当命令名，新增危险命令是**表驱动数据
变更**而非正则修炼（`Table.Destructive` 三判据：恒不可逆命令 / 危险 flag / 危险
参数前缀，「多报不漏报」哲学）。迁移建议：`bashguard.Analyze` 的
`Decision.Destructive` 作底座，下游补充规则以追加策略表/复核 `AllCalls`
（全 AST 调用集）的方式叠加，正则族逐步退役。

## 定位

**tier 2** — 集成层，依赖 tier0/tier1 + 重第三方地板。

- 本仓依赖的矩阵内仓：`guardx`、`runtime`、`textx`。

## 外部依赖地板

- `github.com/cloudwego/eino`
- `mvdan.cc/sh/v3`

## 与 oekit 的关系

本矩阵（oagentkit，Agent/LLM 编排工具链）**依赖** oekit（Open Easy Kit，Go 微服务工具链）：
`oekit/pkg` / `oekit/concurrency` / `oekit/logx` 等。oekit 是另一套独立矩阵，不是子品牌。

## 开发

```bash
go test ./... -race
go vet ./...
gofmt -l .
```

规范见 [AGENTS.md](AGENTS.md)。

## License

MIT
- **CLI 编码 agent 调用必须隔离环境**（用 `runtime.ChildEnv` 构造最小环境），
  不得继承完整宿主环境 —— 凭据泄漏面。
- `workcopy` 的 git 操作必须限制在沙箱目录内，`EnsureMergeBase` 失败要显式报错，
  不得跳过继续。
- SAST 扫描是**确定性前置扫描**，不执行被扫代码。
# tools CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.10.1] — 2026-10-20

- 杂务(deps)：依赖地板抬到最新

地板对齐到发版时刻的各依赖最新 tag：
- oekit/pkg、oekit/concurrency 抬到 v0.18.0（oekit/logx 已是最新 v0.7.1）
- 矩阵内跨仓依赖抬到各仓最新 tag
依据 matrix-tools#44：最新 tag 的依赖地板在「打 tag 那一刻」就已落后一格，
只能靠后续版本补齐（tag 受 protect-tags ruleset 保护不能重打）。


- 文档(changelog)：按新生成器规则重生成 CHANGELOG.md

生成器修复（oagentkit/matrix-tools#43）落地：
① `[round-N] ` 前缀被剥离 → conventional commit 解析不再失效，
   **commit body 保留**（此前整段被丢弃，发布说明只剩标题）；
② 非 conventional 提交也保留 body；
③ `--check` 识别「待发版段」（预写了但还没打 tag 的那段）。
本 PR 恢复的是**本就在 commit body 里**的正文（迁移步骤、代码块、
判据锚点）——此前被 `[round-N] ` 前缀导致的解析失败丢弃了。
**没有任何发布说明内容被删除**：删除的行都能在新增行里找到对应
（规范化掉 `(#N)` 与 `[round-N] ` 前缀后逐行比对）。
关联：oagentkit/matrix-tools#43、oagentkit/matrix-tools#40


## [v0.10.0] — 2026-10-19

### 新增(bashguard)：接收 guardx/bashguard —— Shell 静态裁决落到重仓

自 github.com/oagentkit/guardx/bashguard 迁入（guardx v0.7.0 起该路径退役）。
为什么迁到 tools：bashguard 依赖 mvdan.cc/sh/v3（真 shell AST 解析），guardx 是
tier 0 零依赖底层 —— 依赖面按 module 结算，那条 require 会顺着依赖树扩散到
矩阵每一层。带重依赖的能力该待在带该依赖的仓里；tools 本身就有「git 工作副本
沙箱」，同属命令执行面。
包名与语义不变（静态解析、不执行命令；解析失败/含动态词/含写重定向/含复合控制流
一律非只读），只是 import 路径从 github.com/oagentkit/guardx/bashguard 变为
github.com/oagentkit/tools/bashguard。
关联 oagentkit/guardx#3

## [v0.9.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（guardx v0.5.0 / runtime v0.7.0 / textx v0.5.0） (13)

matrix-tools 的 verify_dep_floors.py 首次全矩阵扫描查出的地板漂移，本仓是受影响仓之一。
本轮按 tier 拓扑顺序推进：tier0、tier1 先发，本仓再把地板跟到它们的新 tag——
否则就是「发了新的、旧的立刻变落后」，永远收敛不了。
所升版本与原钉版本相比源码零改动，逐版实测确认：
- textx v0.4.0→v0.5.0 / obs v0.5.0→v0.6.0 / guardx v0.4.1→v0.5.0 /
  runtime v0.5.0→v0.6.0 / mcpkit v0.6.0→v0.7.0  全树逐字节相同（diff -rq 退出码 0）
- oekit/pkg v0.14.0→v0.15.1  仅新增 urlguard 包（纯标准库，零新增三方依赖）
- oekit/concurrency v0.17.0→v0.17.1  仅三个文档文件
- oekit/logx v0.7.0→v0.7.1  仅 CI 配置与文档
无 API 变更、无兼容风险。
本地门禁：go mod tidy -diff 无输出 / go build / go vet / gofmt / go test -race 全通过。
关联 oagentkit/matrix-tools#13


## [v0.8.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（concurrency v0.17.1 / logx v0.7.1 / pkg v0.15.1）

matrix-tools 的 verify_dep_floors.py 首次全矩阵扫描查出：43 条第一方依赖里
28 条落后，横跨 11 个仓；本仓是其中之一。
所升版本与本仓原钉版本相比源码零改动，逐版实测确认：
- textx v0.3.0→v0.4.0 / obs v0.4.1→v0.5.0 / runtime v0.5.0→v0.6.0 /
  mcpkit v0.5.1→v0.6.0  全树逐字节相同（diff -rq 退出码 0）
- oekit/pkg v0.14.0→v0.15.1  仅新增 urlguard 包（纯标准库，pkg 的 go.mod
  连 require 块都没有，零新增三方依赖）
- oekit/concurrency v0.17.0→v0.17.1  仅三个文档文件
- oekit/logx v0.7.0→v0.7.1  仅 CI 配置与文档
无 API 变更、无兼容风险。
本地门禁：go mod tidy -diff 无输出 / go build / go vet / gofmt / go test -race 全通过。


## [v0.7.1] — 2026-10-19

- 杂务(deps)：依赖地板跟进 guardx v0.4.1 / runtime v0.6.0 / textx v0.4.0

guardx v0.3.0 → v0.4.1：拿到 egress 补 0.0.0.0/8 的 SSRF 修复
（oagentkit/guardx#2）。本仓只用 guardx/logredact，而 v0.3.0 与 v0.4.0 指向
同一 commit、logredact 零变更，所以这次升级**不含任何行为变化**，
是纯粹的地板滞后清理。
runtime v0.5.0 → v0.6.0、textx v0.3.0 → v0.4.0：同样落后的地板，一并跟进。
go.sum 仅这三个模块的版本替换，三方依赖一条未动。


## [v0.7.0] — 2026-10-19

- 杂务(deps)：oekit 依赖升级（concurrency v0.16.0→v0.17.0）

本 PR 含两项改动。
## 1. oekit 依赖地板
姊妹矩阵 oekit 的 `concurrency` 出了 v0.17.0，依赖地板一并对齐。
升级前已核对 `oekit/concurrency` v0.16.0...v0.17.0 的全部 2 个提交：
- `feat(ratelimit): KeyedLimiter.Create 护栏内原子登记 (Fixes #33)` — `ratelimit/keyed.go` +28 行
- `docs: 矩阵总览表 DESC 口径同步` — 仅 AGENTS.md / README.md / CHANGELOG.md
无 breaking / remove 标记，未触及任何已有导出签名，故为纯补丁式升级。
## 2. 迁出被 force 移动过的 tag
本仓原先依赖的姊妹仓 tag（发布后被强推到了重写后的 main）内容已与
sum.golang.org 记录不一致，于是任何走 `proxy.golang.org` 或 `GOPROXY=direct`
的构建都会 SECURITY ERROR。本地往往不报错——模块缓存预热 + goproxy.cn
还留着移动前的字节，把问题掩盖了。
已为 8 个模块重打不可变的**附注 tag**（内容与被移动 tag 指向同一 commit，
只换版本号，让校验和数据库重新记录一次正确哈希），并把依赖迁过去。
`go.sum` 的 h1 全部取自 sum.golang.org，仅就地替换目标模块的两行
（zip h1 + go.mod h1），其余行不动。


## [v0.6.1] — 2026-10-18

- 与 v0.6.0 相比无代码差异（版本号/打包变更）

## [v0.6.0] — 2026-10-18

### 新增：tools 仓建立——外部工具接入（4 包）

从 agentkit 单仓切出 tier2 集成层模块，module 路径 github.com/oagentkit/tools。
收编 4 个包：
- acpx      CLI 编码 agent 统一调用（claude/codex/gemini/kimi/mcode 等），隔离环境执行
- websearch 公开资料检索抽象，接口可插拔（缺省 SearXNG 自建实例）
- workcopy  Git 工作副本沙箱：隔离环境 + merge-base 校验
- sast      确定性 SAST 前置扫描（不执行被扫代码）
矩阵内依赖：textx、runtime、guardx。
门禁：go build / go vet 通过；go test -race 中 sast、websearch 全绿。
acpx 与 workcopy 的测试在本沙箱下无法运行 —— 子进程 heredoc 与 git init 复制 hooks
模板被沙箱禁止；已在未改动的原单仓对照确认这批包拆分前同样失败，与本次改动无关。

- 杂务(deps)：依赖地板升到「历史最新 + 0.1.0」

本轮做了版本线重置：清空全部历史 tag，按「历史最新 + 0.1.0」重打新版本
（如 llm v0.6.0 → v0.7.0、toolgate v0.6.1 → v0.7.0、agentkit v1.3.0 → v1.4.0）。
各仓 go.mod 的矩阵内依赖地板同步跟进到新版本号。
**go.sum 的 oagentkit 行已整段删除**，由 CI 的 `go mod tidy` 重算：
新 tag 的内容与任何历史版本都不同，`h1` 必须现算；本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现）。
重算依赖 `.github` 的 go-ci 配了 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 缓存着旧版本，若走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
（第二个提交会补上重算好的 go.sum。）
* chore(deps): 依赖地板升到「历史最新 + 0.1.0」
chore(deps): 依赖地板升到「历史最新 + 0.1.0」
本轮做了版本线重置：清空全部历史 tag，按「历史最新 + 0.1.0」重打新版本
（如 llm v0.6.0 → v0.7.0、toolgate v0.6.1 → v0.7.0、agentkit v1.3.0 → v1.4.0）。
各仓 go.mod 的矩阵内依赖地板同步跟进到新版本号。
**go.sum 的 oagentkit 行已整段删除**，由 CI 的 `go mod tidy` 重算：
新 tag 的内容与任何历史版本都不同，`h1` 必须现算；本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现）。
重算依赖 `.github` 的 go-ci 配了 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 缓存着旧版本，若走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
（第二个提交会补上重算好的 go.sum。）
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。
* chore(deps): 回填 CI 重算的 go.sum
`h1` 由 CI 里真实的 `go mod tidy` 算出后回填——本地没有 Go 工具链，
且 `/go.mod` 的 dirhash 无法自行复现（zip hash 可复现，gomod hash 不可复现），
所以这是唯一权威来源。
重算依赖 `.github` go-ci 里的 `GOPRIVATE=github.com/oagentkit/*` 直连 VCS：
goproxy.cn 仍缓存着版本线重置前的旧内容，走 proxy 会拉回**旧代码**并算出旧 hash，
门禁全绿但依赖是错的。
验证方式就是本 PR 的 `go mod tidy -diff`：若回填有任何偏差，它会照样报差异。

- 杂务(deps)：跟进最新 tag

oagentkit/guardx v0.1.2→v0.2.0、oagentkit/runtime v0.3.1→v0.4.0、oagentkit/textx v0.1.0→v0.2.0
go.sum 取 sumdb 权威 h1，仅就地替换目标 mod 两行。

- 杂务(deps)：oekit 依赖升级

concurrency v0.13.1→v0.16.0、pkg v0.13.0→v0.14.0
姊妹矩阵 oekit 的依赖地板此前被漏掉（上一轮只扫了 oagentkit 内部依赖），本次一并对齐到最新。
升级前已确认 oekit/concurrency v0.13.1..v0.16.0 的 8 个提交全是 feat/docs/chore，
无 breaking/remove 标记。其中 #28 订正了 `WithCooldown(0)` 的语义——
即 llms.txt 里警告过的那个陷阱，升上去正好拿到修正后的行为。
go.sum 的 h1 取自 sumdb，仅就地替换目标 mod 的两行、其余行不动。

- 杂务(deps)：guardx v0.1.1 → v0.1.2

guardx v0.1.2 已发布（含 injectband 新包），本仓跟进。
go.sum 的 h1 取自 sumdb，仅就地替换该 mod 的两行。

- 杂务(deps)：跨仓依赖升级

guardx v0.1.0 → v0.1.1 / runtime v0.1.0 → v0.3.1
全 org 跨仓依赖地板统一升级。go.sum 的 h1 取自 sumdb（经 goproxy.cn 代理），
即 `go mod tidy` 会写入的同一个权威值；只就地替换该 mod 的两行、其余行一个字节不动，
不重排全文件（go.sum 行序由 cmd/go 自己维护，手写重排会引入漂移）。

- 杂务(deps)：oekit/concurrency v0.12.0 -> v0.13.1

本仓不使用上游 breaker（熔断只在 llm 仓），v0.13.0 的 #25 修复与
v0.13.1 的 #26（RetryableWithHeaders 头值 OWS 修剪）对本仓无直接影响，
本次为矩阵内版本对齐。
门禁：go mod tidy -diff / build / vet /
test -race / gofmt 全绿。

- 杂务(deps)：oekit 三模块升至最新（pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0）

拿到上游新增能力：
- concurrency #22 RetryableWithHeaders：403 头感知判定（次级限流 / 主限流耗尽）
- concurrency #23 FanInLimit：受限并发扇出
- concurrency #24 WithTokenClock、#21 Lockout（keyed lockout/penalty）
- logx #12 TraceHandler：给裸 slog.Handler 注入 trace_id
破坏性变更核对（均无影响）：
- logx v0.7.0 ParseLogLevel 签名改为 (Level, error) —— 我们未使用该函数
- concurrency v0.9.0 的 Slot.Panicked 废弃 —— rag 已迁到 Slot.Err + errors.As
门禁：go build / go vet / go test -race 全绿。

- 杂务(deps)：oekit/concurrency v0.6.0 -> v0.9.0

拿到上游显式三态状态机（#17）与 singleflight 可观测折叠（#18），并为本次
breaker 替换评估了语义差异。
用到的子包只有 async / singleflight / syncx —— 上游 breaker 包我们并未引用
（runtime/breaker 是本仓自己的三态实现），所以本次升级零 breaking。
关于 breaker 替换的现状：#17 已落地（closed/open/half-open + 代绑定 + 探测时限 +
开放期失败不延长冷却），但两侧的触发语义仍不对齐，已在上游补两个需求：
- #19 冷却期与探测失联时限共用 probeTimeout，无法独立配置
- #20 缺「连续失败 N 次即熔断」（trip）触发
因此本包暂不删除 —— llm.TestResilientBreakerTrips 钉的是 trip 语义，
上游 v0.9.0 只有速率型触发（dropRatio > 0），现在换会让该测试失败。


- CI：接入 org 共享门禁（规则本体在 oagentkit/.github）

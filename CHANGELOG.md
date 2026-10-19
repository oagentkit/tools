# tools CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.9.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（guardx v0.5.0 / runtime v0.7.0 / textx v0.5.0） (13)

## [v0.8.0] — 2026-10-19

- 杂务(deps)：依赖地板对齐远端最新（concurrency v0.17.1 / logx v0.7.1 / pkg v0.15.1） (#13)

## [v0.7.1] — 2026-10-19

- 杂务(deps)：依赖地板跟进 guardx v0.4.1 / runtime v0.6.0 / textx v0.4.0

## [v0.7.0] — 2026-10-19

- 杂务(deps)：oekit 依赖升级（concurrency v0.16.0→v0.17.0）

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

- 杂务(deps)：依赖地板升到「历史最新 + 0.1.0」 (#5)
- 杂务(deps)：跟进最新 tag (#4)
- 杂务(deps)：oekit 依赖升级 (#3)
- 杂务(deps)：guardx v0.1.1 → v0.1.2 (#2)
- 杂务(deps)：跨仓依赖升级 (#1)
- 杂务(deps)：oekit/concurrency v0.12.0 -> v0.13.1
- 杂务(deps)：oekit 三模块升至最新（pkg v0.13.0 / concurrency v0.12.0 / logx v0.7.0）
- 杂务(deps)：oekit/concurrency v0.6.0 -> v0.9.0

- CI：接入 org 共享门禁（规则本体在 oagentkit/.github）

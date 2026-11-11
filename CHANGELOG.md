# tools CHANGELOG

> 本文件由 `matrix-tools/scripts/gen_changelogs.py` 从 git 历史生成，按 tag 切段、取每段内的 conventional commit 渲染。

## [v0.14.5] — 2026-11-11

### 修复(bashguard,bgjob,workcopy,acpx)：六单清零——绝对路径内核、兜底丢旗、shell 文本派发器、陈旧 marker、在飞保护、qwen 归因（#132 #133 #134 #135 #136 #137）

- #132 destructiveRule 单源 basename 归一：/bin/rm、/usr/bin/mkfs.ext4、./rm
  在顶层/剥壳后/find 载荷的绝对路径内核全部命中规则（1dbd1bd 只护了
  innerKernelVerdict 段首）——RiskWrite「直接跑」档不再收留不可逆命令
- #133 destructiveByRule 兜底递归传子命令之后的全部 token（此前 nil）——
  git 未登记全局旗退兜底后 --hard 判据不再失明
- #134 不可逆表登记 sh/bash/zsh/dash/ksh（-c 值）与 eval（操作数）为
  shell 文本派发器：InnerOperandShellText 新定位形态，bash -c 'rm -rf /'
  与 eval 同判 su -c
- #135 bgjob 启动链先 rm -f 陈旧 marker——同路径复用 OutputPath 不再被
  旧终态误报 FINISHED、Cleanup 不再删仍在写的输出
- #136 workcopy prepare 的 wc-* 目录登记 inflight（tools#108 的另一半）：
  clone 至多 3×5min 的在飞窗口不再被 Sweep 孤儿扫描删掉；defer 原子收口
- #137 acpx gemini 适配器错误文案取 g.name——qwen 的失败不再归因到 gemini
测试按 issue 号命名新增 5 条回归用例 + bgjob marker 计数断言更新（rm -f
+ echo $? 各一次）；全仓 go test -count=1 全绿。

- 杂务(deps)：依赖地板追到最新——guardx v0.24.3 / textx v0.10.9
- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.44.0

## [v0.14.4] — 2026-11-11

### 修复(workcopy,bashguard,acpx,websearch)：五单清零——token 不落盘、模板任务载体哨兵、信封对象回退、schema 描述守卫（#124-#128）

* fix(workcopy,bashguard,acpx,websearch): 五单清零——token 不落盘、不可逆判定全量面、模板任务载体哨兵、信封对象回退、schema 描述守卫（#124 #125 #126 #127 #128）
- #124 workcopy token 不再嵌 clone URL（git 会把带 userinfo 的 URL 逐字写进
  .git/config 持久落盘）：clone/fetch 经 `-c http.extraheader=Authorization:…`
  进程级注入 Basic 凭证，config 里的 origin URL 恒为无凭证形态；refresh 从
  origin URL 反推 scheme 决定是否带鉴权参数。argv 秒级瞬时可见性替代
  小时/天级持久落盘面
- #125 bashguard 不可逆判定吃全量 AllCalls 的修复已在此前 tools#44 轮落地，
  本轮补回归钉（touch && rm -rf / 三形态），回执关单
- #126 acpx validateTemplate 要求模板恰含一个独占参数位的 {prompt}——漏写
  任务载体时 prompt 被静默丢弃而运行照常「成功」，与 RunRequest 强制 Prompt
  非空自相矛盾；多写同样拦
- #127 envelopeErrMsg 对象形态无 message 键/为空时回退原文——「其余形态
  回退原文」曾对任何 JSON 对象结构性不可达，错误信封被吞成无失败经全文
  兜底变成「成功」的垃圾正文
- #128 websearch jsonschema 裸标签换 description= 键值形态（eino 按 key=
  value 切分、无 = 的裸标签整条静默丢弃，「用领域/技术术语」指引从未到达
  模型）；AsTool 加构造期守卫（description 缺失即 panic），静默失效变构造
  期失败
测试按 issue 号命名新增 5 条回归用例；全仓 go test ./... 全绿。
* style: staticcheck QF1008——ToolInfo 嵌入字段选择器去显式 ParamsOneOf

- 杂务(deps)：依赖地板追到最新——guardx v0.24.2 / textx v0.10.8 / runtime v0.12.7（2026-11-11 列车）
- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.43.0
- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.32.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.31.2` | `v0.32.0` |
| `github.com/oekit/logx` | `v0.9.1` | `v0.9.2` |
| `github.com/oekit/pkg` | `v0.41.1` | `v0.42.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。


## [v0.14.3] — 2026-11-10

### 修复(bashguard)：「在别处执行」包装器的内层命令按内核判不可逆（#117）

容器/命名空间/切用户/瞬态服务类包装器的内层命令此前不在判定链上，
docker exec web rm -rf /data 落 write 档而非 destructive，同一效果两档判定：
- kubectl 登记在不可逆表只认 delete 子命令，exec 使 destructiveKernel 早退；
- chroot /host /bin/rm 与 systemd-run /bin/rm 的内层是绝对路径，
  kernelScan 只认裸命令名；
- su -c 的内核是 shell 文本，argv 定位取不到。
修法按 issue 主张表驱动：Policy.InnerCmd（名字 → 内层命令定位规则
InnerCmdLoc），判定面复用既有机制——
- 定位段交 kernelScan 全 token 扫描（与 #114 find -exec 的 execPayloadScan
  同法，段内可再包调度器），钩在 destructiveVerdict 规则路径尾部，
  sudo chroot … 这类外层调度器经 kernelScan 委派同样命中；
- 段首绝对路径按 basename 再查一次规则；
- 旗标值形态（su -c/--command/--session-command）按 shell 文本走
  Analyze 再判，宿主自建规则在值文本里同样生效；
- 与 #109 的 kernelScan 兜底衔接：docker/podman/nsenter 裸形态本就由
  全 token 扫描判到，扩表后归因更准（原因串指到包装器与内层），测试
  收作对照组钉住不退化。
覆盖 issue 所列 8 类形态：docker/podman exec、kubectl exec 的 --、
docker run|create image 后、chroot 第二操作数、nsenter -- 后、su -c 值
再判、systemd-run 首操作数。反证面同钉：内层 grep/ls/可逆 rm 不误报。
Closes oagentkit/tools#117

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/textx v0.10.7

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/textx` | `v0.10.6` | `v0.10.7` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.31.2

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.31.1` | `v0.31.2` |
| `github.com/oekit/logx` | `v0.9.0` | `v0.9.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。


- feat/fix: bgjob marker 协议原语 + bashguard find/git 不可逆补口 + workcopy 在飞刷新保护 + acpx gemini/qwen 守门（#106 #107 #108 #110）

feat(bgjob): 远程后台作业 marker 协议原语（#106）
jumpserver-exec 手搓的「经 SSH 在远端起脱离会话的长任务 + 轮询 + 收尾」
协议原语化（提案正文圈定的最小面，不做进程树管理、不做日志轮转）：四条
实弹坑全部协议化并钉成契约（doc.go 验收项）——
1. 退出码走 marker 文件（输出路径 + ".rc"）：输出文件早就在场，判终态
   只认 marker（running/finished+退出码/removed 三态）；
2. 命令在内层 shell 跑、marker 由外层 wrapper 写：任务内任何 exit 只死
   内层，\0 照常落 marker（拼进用户命令的写法在裸 exit 上永远 RUNNING）；
3. marker 路径 Go 侧单源一次拼定、命令整段单引号包裹：".rc 拼进引号内变
   p.rc.rc、探测永远 RUNNING" 的 P0 活虫在协议层不可能复发；
4. 清理自证：RUNNING 拒绝且文件原样、FINISHED 后 rm + [ -e ] 自证 REMOVED
   才报已清理、REMOVED 幂等。
- Session 小接口收窄为 Exec(string) (stdout, code, err)，宿主各自适配；
- 边界按提案：不做进程树管理；不做日志轮转（输出重定向由调用方经
  WithOutputPath 给定，必填、远端绝对路径）；远端契约 POSIX sh + setsid
  + nohup；
- 测试经本地真 shell 背书协议（比手搓 fake 模拟远端状态机可信）：
  exit 3 → finished/3、裸 exit marker 照写、路径含空格/单引号逐字、
  RUNNING 清理拒绝且文件原样、命令原文钉双层结构与单源拼接。
Closes oagentkit/tools#106


## [v0.14.2] — 2026-11-10

### 修复(bashguard,acpx,workcopy)：不可逆内核定位/只读门/nil logger/部分文本终态守门收口（#100 #101 #102 #103 #104 #105）

style: gofmt acpx/mimo.go acpx/codex.go bashguard/destructive.go——lint 门禁红

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.24.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.24.0` | `v0.24.1` |
| `github.com/oagentkit/runtime` | `v0.12.5` | `v0.12.6` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.41.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.41.0` | `v0.41.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.31.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.31.0` | `v0.31.1` |
| `github.com/oekit/logx` | `v0.8.12` | `v0.9.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。


## [v0.14.1] — 2026-11-09

### 修复(acpx)：截断守门 fail-closed——无终态+尾部截断留痕拒绝兜底残缺流（#94）

复合变体（终端行>1MB×流>8MB）此前只做形状扫描找不到形状就放行兜底。
另修 claude is_error 空串静默成功：空串回退通用文案。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.41.0

## [v0.14.0] — 2026-11-09

### 修复：tools 第三轮 issue 清零二波（#82 #83）

* fix(acpx): claude is_error result 如实上抛并接入终态丢失守门（#82 #83）
* fix(acpx): 事件流兜底前过终态丢失守门，超长终态行被丢不再把事件流原文当最终回答（#83）
---------
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

### 修复：tools 第三轮 issue 清零（#79 #80 #81）

* fix(bashguard): sed 只读档深度校验脚本受限文法——w/W/e/r/R 原语不再经裸操作数/-e/-f 放行（#81）
sed 是含 I/O 原语的 mini-language（w/W 写文件、GNU e 与 s///e 执行 shell 命令、r/R 注入外部文件内容），脚本串以裸操作数/-e/--expression/-f 多路到达，flag 白名单+NoBareOperands 只看得到「有没有操作数」，sed -n 'w /tmp/x' /etc/hosts 曾整体判只读（实测文件真被写出）。
- Policy 新增 CheckArgs 参数深度校验钩子：叶子路径在白名单 flag 消费后调用，收完整 argv，返回非空=非只读原因；实现须自带 fail-closed 纪律
- sed 表项接 sedArgsCheck：脚本按受限文法整体校验（s///[gpiIMm0-9]/y////p/d + 数字/$/re/ 地址、逗号范围、! 否定、任意分隔符），文法外一律拒（{}块、分支标签、~/+N 扩展地址、未闭合、地址悬空）；w/W/e/r/R 与 s 的 w/e flag 点名拒因；-f/--file 脚本内容静态不可证直接拒；-- 后首操作数按 sed 语义定位脚本，其余裸操作数是输入文件不校验
- 白名单而非黑名单：w/W/e/r/R 加行地址加 -e/-f 的组合面太大，黑名单漏一则全漏
回归：TestSedScriptReadOnlyGate——issue 四条复现（w f / 1e id / -e 'w f' / 1w f）全拒，sed -n '1,2p' F 等受限文法内脚本仍只读，env sed 剥壳同判。
* feat(bashguard): Decision 增加 OverLength/Unparsed 拒绝判定细分标记（#80）
超长（>MaxCommandLen）与解析失败此前只有 Unevaluated 伞标记（tools#45），集成方无法区分「判定完成没发现风险」与「拒绝判定」——jumpserver-kit 确认门据 AllCalls/Invocations 全空把 10KB 注释填充撑过长度闸、文本里尾随 rm -rf 的命令归 UNKNOWN 免确认（填充绕过实测）。
- OverLength：len > MaxCommandLen 未进解析器时置位（overlongDecision）
- Unparsed：解析失败未进 AST 时置位
- 两者是 Unevaluated 的细分归因（为何没判），置位时伞标记必并存；Unevaluated/OverLength/Unparsed 全 false 才是「判定完成」
- tools#45 行为不回退：超长仍带头/尾窗 best-effort 不可逆扫描，命中照常置 Destructive 且与细分标记并存；Risk() 语义不变
回归：TestUnevaluatedRefusalMarkers——超长/解析失败各自置位且互斥、判定完成（只读/写/不可逆/sed 拒）三标记全 false、超长窗口命中 rm -rf 仍 Destructive+OverLength 并存。
* feat(bashguard): Decision.Redirs 全 AST 重定向事实面，集成方重定向判定免二次解析（#79）
#35 收编 AllCalls 后集成方仍有一项独立判定要做不了：重定向目标检查（cat disk.img > /dev/sda 写块设备走 Stmt.Redirs 不走调用面），此前只能自己再 parse 一遍同一命令（Analyze 内部一次 + 集成方一次）；AllCalls 只收 CallExpr，Redirs 不在其列。
- Decision 新增 Redirs []RedirInfo：syntax.Walk 全 AST 收集（语句级/子 shell/{...}/$(...) 内全部可见，与判定路径提前返回无关），与 AllCalls 收编动机同型——重复实现会漂、漂的那份决定门禁拦什么
- RedirInfo 字段：Op 操作符字面 / FD 目标 fd（2> 为 2，{varname}> 为 -1，&> 族记 1）/ Target 目标词字面化（heredoc 为定界符，体不在本结构）/ Dynamic 展开标记 / Read 读侧标记
- 超长路径头/尾窗 best-effort 收集（重叠区去重，与 AllCalls 同法）；解析失败为空
- 语义边界：写重定向（Read=false）本身已判非只读，本事实面供非只读之后的目标细分（如 /dev/* 且非 null/zero → 升级处置）；issue 的方案二（上游直接判设备文件）按 issue 意见「前者更好」取方案一，判定策略留给集成方
回归：TestDecisionRedirsFacts——> /dev/sda 目标事实、2>>/fd、读侧标记、heredoc 定界符、动态目标、&>、子 shell/管道后半/命令替换内可见、超长尾窗收集、集成方设备文件检查示例免二次解析。

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/runtime v0.12.5

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/runtime` | `v0.12.4` | `v0.12.5` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.24.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.23.4` | `v0.24.0` |
| `github.com/oagentkit/textx` | `v0.10.5` | `v0.10.6` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.40.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.39.0` | `v0.40.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/textx v0.10.5

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/textx` | `v0.10.4` | `v0.10.5` |
| `github.com/oekit/concurrency` | `v0.30.1` | `v0.31.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.23.4

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.23.3` | `v0.23.4` |
| `github.com/oagentkit/runtime` | `v0.12.3` | `v0.12.4` |
| `github.com/oagentkit/textx` | `v0.10.3` | `v0.10.4` |
| `github.com/oekit/concurrency` | `v0.29.0` | `v0.30.1` |
| `github.com/oekit/logx` | `v0.8.11` | `v0.8.12` |
| `github.com/oekit/pkg` | `v0.38.0` | `v0.39.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.13.4] — 2026-11-08

- 与 v0.13.3 相比无代码差异（版本号/打包变更）

## [v0.13.3] — 2026-11-08

### 修复(websearch/sast)：多行 URL 消毒残余面 + semgrep argv 端界符 + nil Logger 兜底（close #63 #62 #61）

Merge branch 'main' into fix/audit-issues-63-62-61

- 杂务(deps)：依赖地板追到最新——基座 v0.38.0/v0.29.0/v0.8.11 + 矩阵 tier0 新 tag（按本仓依赖取齐）

发版时刻地板时效门禁要求追平。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。


## [v0.13.2] — 2026-11-08

- 杂务(deps)：依赖地板追平 oekit BOM v0.2.9——pkg v0.37.4 / logx v0.8.10 / concurrency v0.28.4

## [v0.13.1] — 2026-11-08

- 杂务(deps)：依赖地板追到最新——concurrency v0.28.4 / logx v0.8.9（按本仓依赖取齐）

oekit 基座 v0.28.4 / v0.8.9 落地，发版时刻地板时效门禁要求追平（matrix-tools#188 发版链）。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.37.4

pkg v0.37.4（httpjson NormalizeBaseURL scheme 大小写归一 #239 + refresh_versions）落地，地板同步追平（matrix-tools#188 发版链）。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。

- 杂务(deps)：依赖地板追到最新——concurrency v0.28.3 / pkg v0.37.3 / runtime v0.12.1 / textx v0.10.1 / guardx v0.23.1（按本仓依赖取齐）

发版时刻地板时效门禁（verify_tag_floor --ref HEAD）要求追平（matrix-tools#188 发版链）。
兼容性实测：go build / go vet / go test -race -count=1 ./... 全绿，go mod tidy -diff 无漂移。

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.23.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.20.0` | `v0.23.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(build)：补统一 Makefile——本地 ci 门禁与共享 CI 同口径（agentkit#138）

Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/runtime v0.12.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/runtime` | `v0.11.0` | `v0.12.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/logx v0.8.6

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/logx` | `v0.8.5` | `v0.8.6` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.13.0] — 2026-11-07

### 修复：bashguard 不可逆旁路/超长 fail-open/档位单源 + kimi 中断吞错 + PR 号卫生 + 检索消毒（#44-#49）

## 概要
六个安全/正确性修复：bashguard 三连修（#44 #45 #48）、acpx kimi 中断吞错（#46）、workcopy PR 号 refspec 卫生（#47）、websearch 检索结果提示词注入消毒（#49）。每 issue 一个 commit，测试全绿（gofmt / go vet / go build / go test -race）。
---
close #44
**根因**：`Analyze` 的不可逆判定 `destructiveScan(d.Invocations)` 消费的是判定路径的消费序列——`Invocations` 在首个非只读语句处提前 break，未知命令/写重定向/动态展开/控制流一旦前置触礁，其后的调用根本不进扫描序列，后置的 `rm -rf /` 判不出 `Destructive`，确认门禁开了稳定可复现的旁路（issue 五例全复现）。全量事实集 `Decision.AllCalls`（#35 为此收编）就在手边却没被消费。
**修法**：`destructiveScan` 改扫 `d.AllCalls`（全 AST simple command 集合：管道后半/命令替换/子 shell/块语句全部可见，与判定路径是否提前返回无关）；补内核兜底 `destructiveKernel`——AllCalls 不剥包装，`sudo`/`doas` 这类不在 Commands 表的调度器（内核命令是参数不是语法调用）跳过前置 flag 后按内核再判一次；已注册命令（echo/git …）的参数是数据不参与（`echo rm -rf` 不误报），方向保守（多报只是多一次确认）。回归：`TestDestructiveScanConsumesAllCalls`（前置触礁五形态全命中）、`TestDestructiveThroughUnregisteredDispatcher`。
---
close #45
**根因**：超长命令（>10K）直接 `return Decision{Reason: …}`——对只读档是保守拒，但对不可逆档是静默放弃：10K+1 字节、前面塞满注释、结尾 `rm -rf /` 的命令 `Destructive=false`，「判据不可判」与「判了不不可逆」被压成同一个信号，且无任何「未扫描」标记供调用方 fail-closed。
**修法**：`Decision` 增加显式 `Unevaluated` 标记（超长与解析失败两条未进 AST 的路径都置位）；超长路径另做头/尾各 10K 字节窗口的 best-effort 解析扫描（payload 位置攻击者可控：头窗覆盖前缀形态，尾窗覆盖「垃圾铺垫+尾部收尾」的 issue 实测攻击形态，尾窗丢弃截断残缺首行，重叠区按调用去重），窗口内命中的不可逆动作照常置 `Destructive`；窗口语法破损时作废，`Unevaluated` 标记兜底保守侧。回归：`TestOverlongCommandNotFailOpen`。
---
close #46
**根因**：kimi 适配器对 `runCLI` 的 err 只在 `lastAssistant` 为空时上抛——进程被杀前吐过任一条 assistant 消息，超时/取消/被信号杀死的 err 即被吞、整体按成功返回（ExitCode 是被杀的非零值而 error 为 nil）。kimi 没有协议终态事件，「收到过 assistant 消息」是远弱于终态的代理条件：超时被杀时任务并未完成。
**修法**：区分 err 类别——`isInterruptedExec`（`procx.ErrTimeout` / `procx.ErrCanceled` / `exec.ExitError` 且 wait status 信号终止）判定「没跑完」的中断类，即使有部分文本也如实上抛 err，部分文本挂 `RunResult`（新增 `Interrupted=true` 标记）供排障；「跑完了但退出码非零」的终态优先路径（既有 `TestKimiExitNonZeroWithResult` 钉住的契约）不变。回归：`TestKimiInterruptedPropagatesError`、`TestKimiSignalKillPropagatesError`。
---
close #47
**根因**：`prepare` 对 `key.DefaultBranch` 有 fail-fast 引用名卫生，但同样拼进 git fetch refspec 的 `key.Number` 完全没有校验——同一 refspec 的两个组成只守了一行。Number 含冒号（`1/head:refs/heads/x`）时拼出的 refspec 被 git fetch 当 src:dst 在沙箱克隆内写本地引用（无 shell 注入面，危害限于克隆内引用伪造，但守卫不齐是不一致）。
**修法**：`Ensure` 入口对 Number 补纯数字白名单（`isPrNumber`——三大平台 PR/MR 号均为十进制，纯数字即合法形态全集），fail-fast 在建任何目录之前；prepare 与 refresh 两条 refspec 路径由同一道守卫覆盖。回归：`TestEnsureRejectsInjectedPRNumber`（冒号注入/空号/选项/分号/路径五形态）。
---
close #48
**根因**：bashguard 的不可逆档与 MCP 工具风险注册表是两个各判各的「destructive」判定（issue 实证 jms-mcp：锁 tools v0.10.4 错过 v0.11.0 不可逆档与 v0.12.0 AllCalls，手维护 20+ 条 per-tool `Risk_destructive` 注册表又在命令层叠 bashguard——双源漂移，不可逆拦截面出现缝隙）。
**修法**：档位体系单源化——导出 `RiskTier`（readonly / write / destructive / unevaluated）与 `Decision.Risk()`（档位从**一次**判定推导：不可逆 > 未评估 > 只读 > 写），per-tool 风险注册表引用同一组枚举、工具专属规则注册进同一张 `Table`（Destructive 面）即参与全部档位判定（含 #44 的 AllCalls 扫描与内核兜底），命令提取统一走 `Decision.AllCalls` 替代手写。枚举 godoc 标注与粗粒度风险级（mcpkit low/medium/high）的映射及 v0.10.4→v0.12.0 迁移注记（CHANGELOG 由生成器产出不可手改，注记落 godoc）。测试：`TestRiskTierSingleSource`、`TestRiskTierSharedTableRegistration`。
---
close #49
**根因**：AsTool 闭包把 Title/URL/Snippet 原样拼进「N. 标题\n URL\n 摘要」编号列表——Title/Snippet 是攻击者可影响的半可信外网内容（SEO 投毒/被抓取页面），敌意等级高于仓库内内容（skillkit 对仓库内 description/title 都过 `fence.EscapeUntrusted`，#34 先例），本仓输入面更敌对却零消毒；snippet 内嵌换行即可伪造编号条目与真实 URL 并列、夹带伪指令（issue 实测探针复现）。
**修法**：对齐 skillkit#34 修法口径——Title 先折叠单行（`strings.Fields` join）再过 `fence.EscapeUntrusted`，Snippet 过 `fence.EscapeUntrusted`（保留多行、行首 markdown 结构被插零宽空格中和：伪造编号条目不再是列表标记）；URL 是承重声明保留原样；消毒中和结构不删内容。guardx v0.19.0 已是本仓直接依赖，fence 零新增依赖面。回归：`TestAsToolSanitizesUntrustedResults`（issue 探针同形态）。
---
## 验证
- `gofmt -l .` 为空；`go vet ./...`、`go build ./...`、`go test ./...`、`go test -race -count=1 ./...` 本地全绿。
- 未触碰 go.mod / CHANGELOG.md / AGENTS.md / README / matrix-tools；bashguard 保持静态解析、不执行命令。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.37.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.37.0` | `v0.37.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.20.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.19.0` | `v0.20.0` |
| `github.com/oagentkit/runtime` | `v0.8.0` | `v0.11.0` |
| `github.com/oagentkit/textx` | `v0.9.0` | `v0.10.0` |
| `github.com/oekit/concurrency` | `v0.25.1` | `v0.28.0` |
| `github.com/oekit/logx` | `v0.8.3` | `v0.8.5` |
| `github.com/oekit/pkg` | `v0.34.1` | `v0.37.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。


## [v0.12.0] — 2026-11-06

### 新增(bashguard,workcopy)：AllCalls 全 AST 调用集 + DeepenOptions 参数化

* feat(bashguard,workcopy): AllCalls 全 AST 调用集 + DeepenOptions 参数化（#35 #36）
tools#35——Decision.AllCalls（jumpserver-kit 实弹反哺）：
- syntax.Walk 全 AST 收集 simple command：管道后半 / 命令替换 $(…) / 进程替换
  <(…) / 子 shell / 花括号组里的调用全部可见，与判定路径是否提前返回无关
- Invocations 补 ⚠️ 注释明确定性（判定路径消费序列：sudo -n rm 只见 sudo、
  eval 字符串只见 eval），AllCalls 服务风险面事实（门禁/审计消费）
- CallExpr 参数词继续下探（参数里可有命令替换，内层调用是独立 CallExpr）
- 包装不剥壳的语义边界写进 doc：sudo/xargs/env 内核是参数不是语法调用
tools#36——workcopy.DeepenOptions：
- StartDepth / MaxRounds / ProbeTimeout 参数化（零值取包级缺省，零破坏）：
  单体大仓放宽轮数、CI 紧张收紧；EnsureMergeBaseOpts 新入口，
  EnsureMergeBase / EnsureMergeBaseTLS 走缺省不变
- 耗尽报错带实际轮数（此前「预算耗尽」与「轮数上限耗尽」无从区分）
tools#38——README 补「bashguard 作为命令黑名单判定底座」迁移说明
（正则整串锚点误杀引号内字面量的实弹 + 表驱动数据变更哲学 + AllCalls 叠加口）。
全仓 go build / vet / -race 绿；新增 6 个钉子测试。
关联 oagentkit/tools#35 oagentkit/tools#36 oagentkit/tools#38
* style(bashguard): gofmt 对齐追加测试块
---------
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.19.0

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.18.0` | `v0.19.0` |
| `github.com/oagentkit/runtime` | `v0.7.3` | `v0.8.0` |
| `github.com/oagentkit/textx` | `v0.8.0` | `v0.9.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.25.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.24.0` | `v0.25.1` |
| `github.com/oekit/logx` | `v0.8.2` | `v0.8.3` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——本轮新 tag（首轮 tidy 撞上新发版 proxy 延迟，重跑补完）

Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oekit/pkg v0.34.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/pkg` | `v0.34.0` | `v0.34.1` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>

- 杂务(deps)：依赖地板追到最新——github.com/oagentkit/guardx v0.17.1

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oagentkit/guardx` | `v0.17.0` | `v0.17.1` |
| `github.com/oagentkit/runtime` | `v0.7.2` | `v0.7.3` |
| `github.com/oagentkit/textx` | `v0.7.1` | `v0.7.2` |
| `github.com/oekit/concurrency` | `v0.23.3` | `v0.24.0` |
| `github.com/oekit/pkg` | `v0.31.2` | `v0.34.0` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


- 文档(websearch)：AsTool panic 契约显式声明（rag#31 同族）

* docs(websearch): AsTool panic 契约显式声明（rag#31 同族对齐）
「静态 schema 即编程错误」从内部注释升级为 API doc 契约——与 rag 同族
问题（rag#31），接口签名无 error 通道时 panic 是唯一出路，但契约必须
可见。
* style: gofmt
---------
Co-authored-by: Murphy-Yi <zhangyi@Murphy-YideMac-Mini.local>


## [v0.11.1] — 2026-11-04

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.2

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.23.1` | `v0.23.2` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.1

oekit 基座又发了一版（`concurrency` v0.23.1，2026-10-29 02:19 CST）⇒ 上一轮抬到
v0.23.0 的地板已经落后 ⇒ 再抬一格。
本仓只用到 `concurrency` 的少量 API ⇒ 实测 go build / go vet / go test -race 全绿，
**零行为差异**；go 指令未被顶起。
只改 go.mod / go.sum ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.0 github.com/oekit/logx v0.8.0 github.com/oekit/pkg v0.31.0

oekit 基座发了新版本（pkg v0.31.0 / concurrency v0.23.0 / logx v0.8.0）⇒ 本仓的
依赖地板落后 ⇒ 抬到最新，让**下一次发版**不再带出落后地板（打 tag 时刻的地板
必须是当时的最新——`verify_tag_floor` / `release.py doctor` 都在查这条）。
兼容性实测：本仓只用这些基座包的少量 API ⇒ go build / go vet / go test -race
全绿，**零行为差异**；go 指令未被顶起（本仓的 go 地板不动）。


- 测试(sast)：测试替身 nopLogger 收编 logx.Nop()——Enabled 恒 false 免掉昂贵参数求值；deps 地板追到最新（concurrency v0.23.3/logx v0.8.2/pkg v0.31.2）

## [v0.11.0] — 2026-10-28

### 新增(bashguard)：给不可逆（destructive）一档——判据进表、进判定

issue #25：bashguard 的判定只有两档（只读 / 非只读），缺「不可逆」这一档。
shell 工具实际需要三档：只读（直接跑）/ 写（可逆，直接跑）/ **不可逆**
（删数据 / 改权限 / 关机 / 写设备 ⇒ 要二次确认）。第三档表达不了，下游只能
自己写一份——而漏判一条的代价是命令真跑了。
改动：
- `Decision.Destructive bool` + `Decision.DestructiveReason string`；
- `Table.Destructive map[string]Policy`：不可逆规则表（**独立于只读表**）；
- `Policy` 补三个不可逆维度：`Destructive` / `DestructiveFlags` /
  `DestructiveArgs`（表键尾 `*` 是前缀族，如 `mkfs*`）。
为什么单列一张表：只读表按设计只放只读命令（rm/mv/cp/chmod 刻意不在表里，
既有测试也钉住了「不在只读策略表」这条理由）⇒ 把写命令塞进只读表会改掉既有
语义，也会让只读表不再是只读表。落法是 Table 加第二个映射，复用 Policy 类型。
为什么与只读判定正交：两者回答的是两个问题——「会不会改东西」与「一旦跑就
收不回来」。删单个文件不是只读但可逆，递归删目录不可逆；合成一档会把可逆的
写也报成不可逆（过度确认同样是坏结果）。
判定要点：
- 不可逆扫描**独立于**只读扫描跑：命令被只读门拒了，它仍可能是不可逆的；
- 包装要剥壳（env 套递归删除 ⇒ 不可逆性在内层命令上）；
- 联合短 flag 认（`-rf` 里的 `r`）；参数按**前缀**匹配（`of=` 命中 `of=/dev/sda`）；
- 子命令级（systemctl disable / kubectl delete）；本层 flag 不在表里定位不到
  子命令时**多报不漏报**（扫全部非 flag token：假阳性只是多要一次确认，
  假阴性是命令真跑了）。
内置表覆盖 issue 点名的典型对象：递归删除、mkfs 系列、dd 写设备、
reboot/shutdown/poweroff/halt/kexec、递归改权限/属主、kill -9、kubectl delete、
journalctl --vacuum-*。
语义边界：Destructive=false 只意味着「没命中已知规则」，**不意味着安全**；
是否二次确认是调用方的策略决定，本包给的是「这条命令不可逆」这个事实。
门禁自检：go build ✅ / go vet ✅ / go test -race ✅ 5 包全绿 /
go mod tidy -diff ✅ / gofmt ✅ / golangci-lint 0 issues ✅。
反证样本——不跑不可逆扫描 4 条红；不认联合短 flag 4 条红；
命中规则表就算不可逆（不看 flag）3 条红。

- 杂务(deps)：依赖地板追平——guardx v0.17.0 / concurrency v0.22.1 / logx v0.7.2 / pkg v0.29.1

矩阵的**发版时刻地板门禁**（`verify_tag_floor`）要求：打 tag 那一刻的依赖地板
必须是当时的最新。本仓四条地板都落后了（guardx v0.15.0→v0.17.0、
concurrency v0.22.0→v0.22.1、logx v0.7.1→v0.7.2、pkg v0.28.0→v0.29.1）
⇒ 不抬地板就打不出 tag（tag 受 `protect-tags` 保护**不可重打**）。
guardx v0.17.0 含本仓要用的 `logredact` 载荷裁剪档与 `invoke.Outcome` 第三格
——都是**纯增量** API，既有调用点逐字编译通过。
门禁自检：go build ✅ / go vet ✅ / go test -race ✅ 5 包全绿 /
go mod tidy -diff ✅ / gofmt ✅。

- 杂务(deps)：依赖地板追平上游本轮新版本——textx v0.7.1

本仓 HEAD 的依赖地板已落后于 textx 的最新版本（`verify_tag_floor.py --org oagentkit --ref HEAD` 判红）。
本仓一旦发版，就会把落后地板发出去——而 tag 受 protect-tags ruleset 保护、打完不能重打，
只能靠后续版本补齐。
本版只抬依赖地板，不改任何行为：
- github.com/oagentkit/textx v0.6.2 → v0.7.1
对应 issue matrix-tools#143。

- 杂务(deps)：依赖地板追平上游本轮新版本——github.com/oagentkit/guardx@v0.15.0、github.com/oekit/concurrency@v0.22.0、github.com/oekit/pkg@v0.28.0

本仓 HEAD 的依赖地板已落后于各依赖的最新版本（`verify_tag_floor.py --org oagentkit --ref HEAD` 判红）。
本仓一旦发版，就会把落后地板发出去——而 tag 受 protect-tags ruleset 保护、打完不能重打，
只能靠后续版本补齐。
本版只抬依赖地板，不改任何行为：
- github.com/oagentkit/guardx → v0.15.0
- github.com/oekit/concurrency → v0.22.0
- github.com/oekit/pkg → v0.28.0
对应 issue matrix-tools#143。


- 文档(tools)：刷正 README 的外部依赖清单 —— 与 go.mod 实测对账

本仓 README 的「外部依赖」清单与**各仓 go.mod 的事实**对不上。
矩阵的单一数据源 `matrix-tools/modules.json` 也有同样漂移，已在
[matrix-tools#126](https://github.com/oagentkit/matrix-tools/pull/126) 同步刷正。
## 现状核实
```bash
$ grep -v indirect tools/go.mod | grep -E "^\s+[a-z].*/"
	github.com/cloudwego/eino v0.9.21
	...
	mvdan.cc/sh/v3 v3.13.1
```
⇒ 第三方直接依赖是 **2 个**（eino、**mvdan.cc/sh/v3**），本仓 README 只列了 1 个。
## 影响
`mvdan.cc/sh/v3` 是 shell 解析器（带 `x/sys` 等传递依赖）。漏列 ⇒ 读者低估依赖面。
## 改动
**只改清单那一行（`- `github.com/cloudwego/eino`` ⇒ 多一行 `- `mvdan.cc/sh/v3``）**，其余段落逐字未动。
## 验收
- [x] 清单与 `go.mod` 实测一致
- [x] 与 `matrix-tools/modules.json` 的登记一致（同期在 matrix-tools#126 刷正）
- [x] **纯 .md 改动** ⇒ 不需要 CHANGELOG 条目、不打 tag、不发版（无发版步骤）
关联 oagentkit/matrix-tools#125


## [v0.10.4] — 2026-10-23

- 杂务(deps)：依赖地板跟到上游本轮新版本——guardx v0.14.0 concurrency v0.20.2 pkg v0.22.0

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高：
本 commit 只改 `go.mod` / `go.sum`，**零 `.go` 变更**。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）在本仓**下一次 push**
时当场报红：
- github.com/oagentkit/guardx@v0.14.0
- github.com/oekit/concurrency@v0.20.2
- github.com/oekit/pkg@v0.22.0
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
⚠️ 现在门禁还判绿，是因为本仓 HEAD（`9db09f3e`，2026-10-21T10:49:45Z）
**早于**这三条依赖的 tag 发布时刻——即**静默期**：
- oekit/pkg v0.22.0        2026-10-22T09:20:11Z   HEAD → 发布 = +22.5h
- guardx v0.14.0           2026-10-22T09:47:05Z   HEAD → 发布 = +23.0h
- oekit/concurrency v0.20.2 2026-10-23T03:09:39Z  HEAD → 发布 = +40.3h
用门禁自己的 `check_ref()` 复现（同一份 go.mod、同一批 tag，只把 ref 时刻
推进到现在）：HEAD 时刻 ⇒ ✅ 0 处红；此刻 push ⇒ ❌ 3 处红。
## 为什么无行为差异
本仓实际 import 的第一方/基座子包与上游改动的交集为**空**：
**`github.com/oagentkit/guardx` v0.13.1 → v0.14.0**
上游 `.go` 改动只落在 `permgate/permgate.go`（新增「只读拦截」段）。
本仓实际 import 的 `guardx/logredact` **一字未动**。
**`github.com/oekit/concurrency` v0.19.0 → v0.20.2**
上游 `.go` 改动只落在 `syncx/`（新增 `Gate.TryAcquire`；`KeyedGate` 的
幂等标记裸 bool → `atomic.Bool` + CAS）。本仓实际 import 的
`concurrency/async`、`concurrency/singleflight` **一字未动**。
**`github.com/oekit/pkg` v0.20.0 → v0.22.0**
上游 diff 只**新增** `pathx/*`（新包）与 `stringx/csv.go`——**纯增量**。
本仓实际 import 的 `pkg/httpjson`、`pkg/jsonrepair` **一字未动**。
⇒ 三条都是行为中性的抬地板，本仓转发面与符号集不变。
## 版本口径
走 **patch v0.10.4**，依据是**本仓自己的连续先例**：本仓连续两次
「只抬地板、零代码改动」都走 patch——
- v0.10.2「依赖地板追平上游本轮新版本——guardx v0.13.1 / concurrency v0.19.0 / pkg v0.20.0」
- v0.10.3「依赖地板跟到矩阵本轮新版本——runtime v0.7.2 / textx v0.6.2」
本次同样是零行为差异、不新增任何包，不构成 breaking。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
go mod tidy -diff             ✅ 无输出
```


## [v0.10.3] — 2026-10-21

- 杂务(deps)：依赖地板跟到矩阵本轮新版本——runtime v0.7.2 textx v0.6.2

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）当场报红：
- github.com/oagentkit/runtime@v0.7.2
- github.com/oagentkit/textx@v0.6.2
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
## 为什么无行为差异
被抬的这几个版本**自身也都是纯依赖地板追平**（各仓代码零改动，只抬
`oekit/pkg` / `oekit/concurrency` / `guardx` / `llm` 四类地板之一），
本仓用到的转发面与符号集一字未动。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```


## [v0.10.2] — 2026-10-21

- 杂务(deps)：依赖地板追平上游本轮新版本——oagentkit/guardx v0.13.1 oekit/concurrency v0.19.0 oekit/pkg v0.20.0

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是隐式 breaking：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 抬了哪几块、为什么无行为差异
**`github.com/oagentkit/guardx` v0.13.0 → v0.13.1**
上游 diff 只有 `go.mod` / `go.sum` / `CHANGELOG.md`——**没有 Go 源码变更**
（内容是它自己把 `oekit/pkg` 地板抬到 v0.20.0）。本仓用到的包与转发面一字未动。
**`github.com/oekit/concurrency` v0.18.0 → v0.19.0**
上游 diff 只动了 2 个包：
- `retry`：**抖动 panic 修复**——`rand.Int63n` 的契约是 n > 0，而「退避幅度为 0」
  是生产可达路径（cfg.Delay=0 / DelayHint 返回 0 / float64→int64 溢出成负数），
  旧实现在这些路径上直接 panic。改为钳到 [0, MaxInt64]。
- `syncx`：新增全局在飞闸门原语（`Gate`）+ 说明同步。
**`breaker` 与 `async` 两个包一字未动。**
**本仓实际 import 的 `oekit/concurrency` 子包：async singleflight **
与上游改动相交的：**（无）**
**`github.com/oekit/pkg` v0.18.0 → v0.20.0**
上游 diff 只动了 5 个包：
- `errors`：**纯增量**——新增 `Errorf`（格式化 + `%w` 包装，nil-safe）。
  `New`/`As`/`Is`/`Unwrap`/`Wrap` 是同型透传，一字未动。
- `httpjson`：**退避溢出修复**——旧写法 `base << (attempt-1)` 在 attempt 较大时
  int64 溢出成负数，负数绕过 `MaxDelay` 封顶，`time.After(负数)` 立刻返回
  ⇒ **该轮退避彻底消失**。改为乘法 + 饱和封顶。正常 attempt 数（1..20）下
  结果**逐字相同**，差别只在溢出的病态路径。
- `urlguard`：**纯增量**——新增 `WithExactHosts`，未设置时行为与既有完全一致。
- `stringx`：**仅注释**（补 `TruncEllipsis` 的 n 语义说明）。
- `pathx`：新增包（`SanitizeRel` 等）。
**本仓实际 import 的 `oekit/pkg` 子包：httpjson jsonrepair **
与上游改动相交的：**httpjson**
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```

- 杂务(deps)：guardx 依赖地板抬到 v0.13.0

本仓只用 guardx/logredact，而 logredact 在 v0.9.0 → v0.13.0 之间**未变**
（那几版只加了 sandbox 的 Capable 与 wire）——故这是纯地板追平，无行为差异。
矩阵的依赖地板时效门禁（verify_dep_floors.py）会把「地板落后远端最新」报出来。
门禁自检：
  go build / vet / test   ✅ 5 包全绿
  gofmt                  ✅
  go mod tidy            ✅ 无漂移
* chore(deps): guardx 依赖地板抬到 v0.13.0
本仓只用 guardx/logredact，而 logredact 在 v0.9.0 → v0.13.0 之间**未变**
（那几版只加了 sandbox 的 Capable 与 wire）——故这是纯地板追平，无行为差异。
矩阵的依赖地板时效门禁（verify_dep_floors.py）会把「地板落后远端最新」报出来。
门禁自检：
  go build / vet / test   ✅ 5 包全绿
  gofmt                  ✅
  go mod tidy            ✅ 无漂移

- 杂务(deps)：oagentkit/guardx 依赖地板抬到 v0.11.0

guardx v0.11.0 新增 sandbox/wire 跨进程调用协议，是**新增能力**而非 API 变更——
本仓无需改代码，只抬地板。
为什么现在补：guardx v0.11.0 是本轮刚发布的，本仓上一个 tag 早于它，
于是这一格欠账一直挂着。依赖地板报告器把「子仓发版让下游落后」显式化，
但报告器恒不失败——真正的硬约束是「打 tag 时刻地板必须是当时的最新」。
本地门禁：go mod tidy 无差异 / go build / go vet 通过。

- 杂务(deps)：oagentkit/guardx 依赖地板抬到 v0.9.0

guardx v0.9.0 新增了 toolfail 与 sandbox 两个 tier0 原语，
是**新增能力**而非 API 变更——本仓无需改代码，只抬地板。
为什么现在补：guardx v0.9.0 是 2026-10-21 发布的，本仓上一个 tag 早于它，
于是这一格欠账一直挂着（依赖地板报告器把「子仓发版让下游落后」显式化，
但报告器恒不失败——真正的硬约束是「打 tag 时刻地板必须是当时的最新」）。
本地门禁：go mod tidy 无差异 / go build / go vet 通过。


- 文档(AGENTS)：补「发版顺序」小节，指向 org CONTRIBUTING 单一副本

13 个业务仓的 AGENTS.md 此前对发版顺序零覆盖（只有 textx 有个
「### 版本与发版」标题，但里面只讲 SemVer 与 module 路径），
发版顺序只写在 org 规则仓的 .github/CONTRIBUTING.md 里，单仓读者看不到。
本次只加**指针 + 本仓相关提醒**，runbook 本体保持**单一副本**
（CONTRIBUTING §发版），避免 13 份副本各自漂移。
覆盖四条：
- CHANGELOG.md 由 matrix-tools/scripts/gen_changelogs.py 从 git 历史生成，不要手改
- tag 必须落在 CHANGELOG 提交之后（下游 go get 拿到的是 tag 快照）
- 警告：无参重生成会吞掉待发版段，--next-version 是唯一能产出待发版段的开关
- 本仓相关提醒（见正文）


## [v0.10.1] — 2026-10-20

- 杂务(deps)：依赖地板抬到最新

地板对齐到发版时刻的各依赖最新 tag：
- oekit/pkg、oekit/concurrency 抬到 v0.18.0（oekit/logx 已是最新 v0.7.1）
- 矩阵内跨仓依赖抬到各仓最新 tag
依据 matrix-tools#44：最新 tag 的依赖地板在「打 tag 那一刻」就已落后一格，
只能靠后续版本补齐（tag 受 protect-tags ruleset 保护不能重打）。


## [v0.10.0] — 2026-10-19

- 与 v0.9.1 相比无代码差异（版本号/打包变更）

## [v0.9.1] — 2026-11-04

### 新增(bashguard)：给不可逆（destructive）一档——判据进表、进判定

issue #25：bashguard 的判定只有两档（只读 / 非只读），缺「不可逆」这一档。
shell 工具实际需要三档：只读（直接跑）/ 写（可逆，直接跑）/ **不可逆**
（删数据 / 改权限 / 关机 / 写设备 ⇒ 要二次确认）。第三档表达不了，下游只能
自己写一份——而漏判一条的代价是命令真跑了。
改动：
- `Decision.Destructive bool` + `Decision.DestructiveReason string`；
- `Table.Destructive map[string]Policy`：不可逆规则表（**独立于只读表**）；
- `Policy` 补三个不可逆维度：`Destructive` / `DestructiveFlags` /
  `DestructiveArgs`（表键尾 `*` 是前缀族，如 `mkfs*`）。
为什么单列一张表：只读表按设计只放只读命令（rm/mv/cp/chmod 刻意不在表里，
既有测试也钉住了「不在只读策略表」这条理由）⇒ 把写命令塞进只读表会改掉既有
语义，也会让只读表不再是只读表。落法是 Table 加第二个映射，复用 Policy 类型。
为什么与只读判定正交：两者回答的是两个问题——「会不会改东西」与「一旦跑就
收不回来」。删单个文件不是只读但可逆，递归删目录不可逆；合成一档会把可逆的
写也报成不可逆（过度确认同样是坏结果）。
判定要点：
- 不可逆扫描**独立于**只读扫描跑：命令被只读门拒了，它仍可能是不可逆的；
- 包装要剥壳（env 套递归删除 ⇒ 不可逆性在内层命令上）；
- 联合短 flag 认（`-rf` 里的 `r`）；参数按**前缀**匹配（`of=` 命中 `of=/dev/sda`）；
- 子命令级（systemctl disable / kubectl delete）；本层 flag 不在表里定位不到
  子命令时**多报不漏报**（扫全部非 flag token：假阳性只是多要一次确认，
  假阴性是命令真跑了）。
内置表覆盖 issue 点名的典型对象：递归删除、mkfs 系列、dd 写设备、
reboot/shutdown/poweroff/halt/kexec、递归改权限/属主、kill -9、kubectl delete、
journalctl --vacuum-*。
语义边界：Destructive=false 只意味着「没命中已知规则」，**不意味着安全**；
是否二次确认是调用方的策略决定，本包给的是「这条命令不可逆」这个事实。
门禁自检：go build ✅ / go vet ✅ / go test -race ✅ 5 包全绿 /
go mod tidy -diff ✅ / gofmt ✅ / golangci-lint 0 issues ✅。
反证样本——不跑不可逆扫描 4 条红；不认联合短 flag 4 条红；
命中规则表就算不可逆（不看 flag）3 条红。

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

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.2

| 依赖 | 现地板 | 抬到 |
|---|---|---|
| `github.com/oekit/concurrency` | `v0.23.1` | `v0.23.2` |
## 为什么
矩阵的**发版时刻地板时效**门禁（`verify_tag_floor --ref HEAD`）要求：各仓 HEAD
（= 下次发版会发出去的内容）的依赖地板，相对那一刻的依赖最新版本，不能已落后。
oekit 基座与矩阵模块都在高频发版 ⇒ 地板是**跑步机**，落后一格就会判红。
## 兼容性实测
本工具在提交前跑完 `go build` / `go vet` / `go test -race -count=1 ./...`（全绿才提交）
与 `go mod tidy -diff`（无漂移）⇒ 改动**只影响依赖版本**，零行为差异。
只改 `go.mod` / `go.sum` ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.1

oekit 基座又发了一版（`concurrency` v0.23.1，2026-10-29 02:19 CST）⇒ 上一轮抬到
v0.23.0 的地板已经落后 ⇒ 再抬一格。
本仓只用到 `concurrency` 的少量 API ⇒ 实测 go build / go vet / go test -race 全绿，
**零行为差异**；go 指令未被顶起。
只改 go.mod / go.sum ⇒ 不需要 CHANGELOG 条目（版本段由下次发版的 `prewrite` 生成）。

- 杂务(deps)：依赖地板追到最新——github.com/oekit/concurrency v0.23.0 github.com/oekit/logx v0.8.0 github.com/oekit/pkg v0.31.0

oekit 基座发了新版本（pkg v0.31.0 / concurrency v0.23.0 / logx v0.8.0）⇒ 本仓的
依赖地板落后 ⇒ 抬到最新，让**下一次发版**不再带出落后地板（打 tag 时刻的地板
必须是当时的最新——`verify_tag_floor` / `release.py doctor` 都在查这条）。
兼容性实测：本仓只用这些基座包的少量 API ⇒ go build / go vet / go test -race
全绿，**零行为差异**；go 指令未被顶起（本仓的 go 地板不动）。

- 杂务(deps)：依赖地板追平——guardx v0.17.0 / concurrency v0.22.1 / logx v0.7.2 / pkg v0.29.1

矩阵的**发版时刻地板门禁**（`verify_tag_floor`）要求：打 tag 那一刻的依赖地板
必须是当时的最新。本仓四条地板都落后了（guardx v0.15.0→v0.17.0、
concurrency v0.22.0→v0.22.1、logx v0.7.1→v0.7.2、pkg v0.28.0→v0.29.1）
⇒ 不抬地板就打不出 tag（tag 受 `protect-tags` 保护**不可重打**）。
guardx v0.17.0 含本仓要用的 `logredact` 载荷裁剪档与 `invoke.Outcome` 第三格
——都是**纯增量** API，既有调用点逐字编译通过。
门禁自检：go build ✅ / go vet ✅ / go test -race ✅ 5 包全绿 /
go mod tidy -diff ✅ / gofmt ✅。

- 杂务(deps)：依赖地板追平上游本轮新版本——textx v0.7.1

本仓 HEAD 的依赖地板已落后于 textx 的最新版本（`verify_tag_floor.py --org oagentkit --ref HEAD` 判红）。
本仓一旦发版，就会把落后地板发出去——而 tag 受 protect-tags ruleset 保护、打完不能重打，
只能靠后续版本补齐。
本版只抬依赖地板，不改任何行为：
- github.com/oagentkit/textx v0.6.2 → v0.7.1
对应 issue matrix-tools#143。

- 杂务(deps)：依赖地板追平上游本轮新版本——github.com/oagentkit/guardx@v0.15.0、github.com/oekit/concurrency@v0.22.0、github.com/oekit/pkg@v0.28.0

本仓 HEAD 的依赖地板已落后于各依赖的最新版本（`verify_tag_floor.py --org oagentkit --ref HEAD` 判红）。
本仓一旦发版，就会把落后地板发出去——而 tag 受 protect-tags ruleset 保护、打完不能重打，
只能靠后续版本补齐。
本版只抬依赖地板，不改任何行为：
- github.com/oagentkit/guardx → v0.15.0
- github.com/oekit/concurrency → v0.22.0
- github.com/oekit/pkg → v0.28.0
对应 issue matrix-tools#143。

- 杂务(deps)：依赖地板跟到上游本轮新版本——guardx v0.14.0 concurrency v0.20.2 pkg v0.22.0

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高：
本 commit 只改 `go.mod` / `go.sum`，**零 `.go` 变更**。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）在本仓**下一次 push**
时当场报红：
- github.com/oagentkit/guardx@v0.14.0
- github.com/oekit/concurrency@v0.20.2
- github.com/oekit/pkg@v0.22.0
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
⚠️ 现在门禁还判绿，是因为本仓 HEAD（`9db09f3e`，2026-10-21T10:49:45Z）
**早于**这三条依赖的 tag 发布时刻——即**静默期**：
- oekit/pkg v0.22.0        2026-10-22T09:20:11Z   HEAD → 发布 = +22.5h
- guardx v0.14.0           2026-10-22T09:47:05Z   HEAD → 发布 = +23.0h
- oekit/concurrency v0.20.2 2026-10-23T03:09:39Z  HEAD → 发布 = +40.3h
用门禁自己的 `check_ref()` 复现（同一份 go.mod、同一批 tag，只把 ref 时刻
推进到现在）：HEAD 时刻 ⇒ ✅ 0 处红；此刻 push ⇒ ❌ 3 处红。
## 为什么无行为差异
本仓实际 import 的第一方/基座子包与上游改动的交集为**空**：
**`github.com/oagentkit/guardx` v0.13.1 → v0.14.0**
上游 `.go` 改动只落在 `permgate/permgate.go`（新增「只读拦截」段）。
本仓实际 import 的 `guardx/logredact` **一字未动**。
**`github.com/oekit/concurrency` v0.19.0 → v0.20.2**
上游 `.go` 改动只落在 `syncx/`（新增 `Gate.TryAcquire`；`KeyedGate` 的
幂等标记裸 bool → `atomic.Bool` + CAS）。本仓实际 import 的
`concurrency/async`、`concurrency/singleflight` **一字未动**。
**`github.com/oekit/pkg` v0.20.0 → v0.22.0**
上游 diff 只**新增** `pathx/*`（新包）与 `stringx/csv.go`——**纯增量**。
本仓实际 import 的 `pkg/httpjson`、`pkg/jsonrepair` **一字未动**。
⇒ 三条都是行为中性的抬地板，本仓转发面与符号集不变。
## 版本口径
走 **patch v0.10.4**，依据是**本仓自己的连续先例**：本仓连续两次
「只抬地板、零代码改动」都走 patch——
- v0.10.2「依赖地板追平上游本轮新版本——guardx v0.13.1 / concurrency v0.19.0 / pkg v0.20.0」
- v0.10.3「依赖地板跟到矩阵本轮新版本——runtime v0.7.2 / textx v0.6.2」
本次同样是零行为差异、不新增任何包，不构成 breaking。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
go mod tidy -diff             ✅ 无输出
```

- 杂务(deps)：依赖地板跟到矩阵本轮新版本——runtime v0.7.2 textx v0.6.2

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是**隐式破坏性变更**：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 为什么抬
上一版 tag 打出去时，它依赖的模块的**新版已经先发出**了——
即「打 tag 那一刻地板就落后」。矩阵的「发版时刻地板时效」门禁
（`verify_tag_floor.py --org oagentkit --latest-only`）当场报红：
- github.com/oagentkit/runtime@v0.7.2
- github.com/oagentkit/textx@v0.6.2
门禁给的修法是「抬 go.mod 地板 → go mod tidy → 重新发 tag」。
本 PR 就是那条修法。
## 为什么无行为差异
被抬的这几个版本**自身也都是纯依赖地板追平**（各仓代码零改动，只抬
`oekit/pkg` / `oekit/concurrency` / `guardx` / `llm` 四类地板之一），
本仓用到的转发面与符号集一字未动。
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```

- 杂务(deps)：依赖地板追平上游本轮新版本——oagentkit/guardx v0.13.1 oekit/concurrency v0.19.0 oekit/pkg v0.20.0

## 下游要做什么
**`go mod tidy`**。地板抬高对下游是隐式 breaking：不跑就报
`updates to go.mod needed`，且报错不提真正原因（**不是本仓的 API 变了**）。
本仓**自身代码零改动**——只看本仓的 diff 看不出这是一次地板抬高。
## 抬了哪几块、为什么无行为差异
**`github.com/oagentkit/guardx` v0.13.0 → v0.13.1**
上游 diff 只有 `go.mod` / `go.sum` / `CHANGELOG.md`——**没有 Go 源码变更**
（内容是它自己把 `oekit/pkg` 地板抬到 v0.20.0）。本仓用到的包与转发面一字未动。
**`github.com/oekit/concurrency` v0.18.0 → v0.19.0**
上游 diff 只动了 2 个包：
- `retry`：**抖动 panic 修复**——`rand.Int63n` 的契约是 n > 0，而「退避幅度为 0」
  是生产可达路径（cfg.Delay=0 / DelayHint 返回 0 / float64→int64 溢出成负数），
  旧实现在这些路径上直接 panic。改为钳到 [0, MaxInt64]。
- `syncx`：新增全局在飞闸门原语（`Gate`）+ 说明同步。
**`breaker` 与 `async` 两个包一字未动。**
**本仓实际 import 的 `oekit/concurrency` 子包：async singleflight **
与上游改动相交的：**（无）**
**`github.com/oekit/pkg` v0.18.0 → v0.20.0**
上游 diff 只动了 5 个包：
- `errors`：**纯增量**——新增 `Errorf`（格式化 + `%w` 包装，nil-safe）。
  `New`/`As`/`Is`/`Unwrap`/`Wrap` 是同型透传，一字未动。
- `httpjson`：**退避溢出修复**——旧写法 `base << (attempt-1)` 在 attempt 较大时
  int64 溢出成负数，负数绕过 `MaxDelay` 封顶，`time.After(负数)` 立刻返回
  ⇒ **该轮退避彻底消失**。改为乘法 + 饱和封顶。正常 attempt 数（1..20）下
  结果**逐字相同**，差别只在溢出的病态路径。
- `urlguard`：**纯增量**——新增 `WithExactHosts`，未设置时行为与既有完全一致。
- `stringx`：**仅注释**（补 `TruncEllipsis` 的 n 语义说明）。
- `pathx`：新增包（`SanitizeRel` 等）。
**本仓实际 import 的 `oekit/pkg` 子包：httpjson jsonrepair **
与上游改动相交的：**httpjson**
## 门禁
```
go build / vet / test -race   ✅
gofmt -l .                    ✅ 空
```

- 杂务(deps)：guardx 依赖地板抬到 v0.13.0

本仓只用 guardx/logredact，而 logredact 在 v0.9.0 → v0.13.0 之间**未变**
（那几版只加了 sandbox 的 Capable 与 wire）——故这是纯地板追平，无行为差异。
矩阵的依赖地板时效门禁（verify_dep_floors.py）会把「地板落后远端最新」报出来。
门禁自检：
  go build / vet / test   ✅ 5 包全绿
  gofmt                  ✅
  go mod tidy            ✅ 无漂移
* chore(deps): guardx 依赖地板抬到 v0.13.0
本仓只用 guardx/logredact，而 logredact 在 v0.9.0 → v0.13.0 之间**未变**
（那几版只加了 sandbox 的 Capable 与 wire）——故这是纯地板追平，无行为差异。
矩阵的依赖地板时效门禁（verify_dep_floors.py）会把「地板落后远端最新」报出来。
门禁自检：
  go build / vet / test   ✅ 5 包全绿
  gofmt                  ✅
  go mod tidy            ✅ 无漂移

- 杂务(deps)：oagentkit/guardx 依赖地板抬到 v0.11.0

guardx v0.11.0 新增 sandbox/wire 跨进程调用协议，是**新增能力**而非 API 变更——
本仓无需改代码，只抬地板。
为什么现在补：guardx v0.11.0 是本轮刚发布的，本仓上一个 tag 早于它，
于是这一格欠账一直挂着。依赖地板报告器把「子仓发版让下游落后」显式化，
但报告器恒不失败——真正的硬约束是「打 tag 时刻地板必须是当时的最新」。
本地门禁：go mod tidy 无差异 / go build / go vet 通过。

- 杂务(deps)：oagentkit/guardx 依赖地板抬到 v0.9.0

guardx v0.9.0 新增了 toolfail 与 sandbox 两个 tier0 原语，
是**新增能力**而非 API 变更——本仓无需改代码，只抬地板。
为什么现在补：guardx v0.9.0 是 2026-10-21 发布的，本仓上一个 tag 早于它，
于是这一格欠账一直挂着（依赖地板报告器把「子仓发版让下游落后」显式化，
但报告器恒不失败——真正的硬约束是「打 tag 时刻地板必须是当时的最新」）。
本地门禁：go mod tidy 无差异 / go build / go vet 通过。

- 杂务(deps)：依赖地板抬到最新

地板对齐到发版时刻的各依赖最新 tag：
- oekit/pkg、oekit/concurrency 抬到 v0.18.0（oekit/logx 已是最新 v0.7.1）
- 矩阵内跨仓依赖抬到各仓最新 tag
依据 matrix-tools#44：最新 tag 的依赖地板在「打 tag 那一刻」就已落后一格，
只能靠后续版本补齐（tag 受 protect-tags ruleset 保护不能重打）。


- 文档(tools)：刷正 README 的外部依赖清单 —— 与 go.mod 实测对账

本仓 README 的「外部依赖」清单与**各仓 go.mod 的事实**对不上。
矩阵的单一数据源 `matrix-tools/modules.json` 也有同样漂移，已在
[matrix-tools#126](https://github.com/oagentkit/matrix-tools/pull/126) 同步刷正。
## 现状核实
```bash
$ grep -v indirect tools/go.mod | grep -E "^\s+[a-z].*/"
	github.com/cloudwego/eino v0.9.21
	...
	mvdan.cc/sh/v3 v3.13.1
```
⇒ 第三方直接依赖是 **2 个**（eino、**mvdan.cc/sh/v3**），本仓 README 只列了 1 个。
## 影响
`mvdan.cc/sh/v3` 是 shell 解析器（带 `x/sys` 等传递依赖）。漏列 ⇒ 读者低估依赖面。
## 改动
**只改清单那一行（`- `github.com/cloudwego/eino`` ⇒ 多一行 `- `mvdan.cc/sh/v3``）**，其余段落逐字未动。
## 验收
- [x] 清单与 `go.mod` 实测一致
- [x] 与 `matrix-tools/modules.json` 的登记一致（同期在 matrix-tools#126 刷正）
- [x] **纯 .md 改动** ⇒ 不需要 CHANGELOG 条目、不打 tag、不发版（无发版步骤）
关联 oagentkit/matrix-tools#125

- 文档(AGENTS)：补「发版顺序」小节，指向 org CONTRIBUTING 单一副本

13 个业务仓的 AGENTS.md 此前对发版顺序零覆盖（只有 textx 有个
「### 版本与发版」标题，但里面只讲 SemVer 与 module 路径），
发版顺序只写在 org 规则仓的 .github/CONTRIBUTING.md 里，单仓读者看不到。
本次只加**指针 + 本仓相关提醒**，runbook 本体保持**单一副本**
（CONTRIBUTING §发版），避免 13 份副本各自漂移。
覆盖四条：
- CHANGELOG.md 由 matrix-tools/scripts/gen_changelogs.py 从 git 历史生成，不要手改
- tag 必须落在 CHANGELOG 提交之后（下游 go get 拿到的是 tag 快照）
- 警告：无参重生成会吞掉待发版段，--next-version 是唯一能产出待发版段的开关
- 本仓相关提醒（见正文）


- 测试(sast)：测试替身 nopLogger 收编 logx.Nop()——Enabled 恒 false 免掉昂贵参数求值；deps 地板追到最新（concurrency v0.23.3/logx v0.8.2/pkg v0.31.2）

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

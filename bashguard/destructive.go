package bashguard

import (
	"fmt"
	"path"
	"strings"
)

// 不可逆（destructive）判定：**独立于只读判定**的一张表。
//
// # 为什么单列一张表
//
// 只读表只放只读命令（"不在只读策略表" 是既有语义，也是既有测试钉住的契约）。
// 而不可逆判据必须覆盖 rm / mkfs / dd 这类**写命令**——它们恰恰不在只读表里。
// 塞进只读表会改掉「不在只读策略表」的既有语义，也会让只读表不再是只读表。
//
// # 为什么与只读判定正交
//
// 两条判定回答的是两个不同的问题：
//
//   - 只读：这条命令**会不会**改东西（能不能直接跑）；
//   - 不可逆：这条命令**一旦跑就收不回来**（要不要二次确认）。
//
// 一个命令可以「不是只读、但可逆」（`rm file` 删单个文件），也可以
// 「不是只读、且不可逆」（`rm -rf /`）。所以不可逆判定**不能**从只读结论里推出来。
//
// # 语义边界
//
// Destructive 为 false **只意味着「没命中已知规则」，不意味着「安全」**——
// 与只读判定同一条边界（能判就判，判不了就不判）。是否要二次确认是**调用方**
// 的策略决定（不同宿主对同一命令的处置可能不同）；本包负责的是
// 「这条命令不可逆」这个**事实**。

// RiskTier 命令风险档位（单源枚举，tools#48）。
//
// 档位体系只此一份：消费方（如自带 MCP 工具风险注册表的宿主）的 per-tool
// 档位应引用同一组枚举，工具专属的不可逆规则注册进 Table（Destructive 面）
// 而不是另立一张平行表——两张表各判各的必然漂移：jms-mcp（堡垒机执行 MCP，
// 不可逆操作是核心安全面）曾锁 tools v0.10.4，错过 v0.11.0 的不可逆档与
// v0.12.0 的 AllCalls，其手维护的 20+ 条 per-tool Risk_destructive 注册表与
// 命令层 bashguard 判定双源漂移，不可逆拦截面出现缝隙。命令提取统一走
// Decision.AllCalls（全 AST 调用集，Analyze 已对其做不可逆扫描），不要手写。
//
// 粗粒度风险级（low/medium/high，对标 mcpkit ToolHints.RiskLevel）的映射：
// readonly→low、write→medium、destructive/unevaluated→high（保守侧）。
type RiskTier string

const (
	// RiskReadOnly 只读档：不执行任何写路径。
	RiskReadOnly RiskTier = "readonly"
	// RiskWrite 写档：非只读（写路径或未知命令）但未命中不可逆判据。
	// 注意未知命令并未被证明「可逆」，保守调用方可视同待确认。
	RiskWrite RiskTier = "write"
	// RiskDestructive 不可逆档：一旦跑就收不回来——二次确认的判据。
	RiskDestructive RiskTier = "destructive"
	// RiskUnevaluated 未评估档：未进 AST 判定（超长/解析失败）——保守侧，
	// 调用方按需与不可逆档同处置（要确认）。
	RiskUnevaluated RiskTier = "unevaluated"
)

// Risk 单源推导本判定的风险档位：不可逆 > 未评估 > 只读 > 写。
//
// Destructive 优先于 Unevaluated：超长命令的头/尾窗扫描命中不可逆时，按
// 更强的信号报（窗口未覆盖全文的事实已由 Unevaluated 并存表达）。
func (d Decision) Risk() RiskTier {
	switch {
	case d.Destructive:
		return RiskDestructive
	case d.Unevaluated:
		return RiskUnevaluated
	case d.ReadOnly:
		return RiskReadOnly
	default:
		return RiskWrite
	}
}

// destructiveScan 扫描调用序列与重定向事实里的不可逆动作。
//
// 与只读判定正交：命令被只读门拒了（rm 不在只读表），它仍可能是不可逆的——
// 而「这条命令不可逆」正是调用方要据此要二次确认的那个事实。
//
// 输入应是**全量调用集**（Analyze 传 Decision.AllCalls，tools#44）：判定路径
// 的消费序列（Invocations）在首个非只读语句处提前截断，前置触礁语句（未知
// 命令/写重定向/动态展开/控制流）可藏住后置的 rm -rf /——不可逆扫描消费
// 提前返回的序列，等于给确认门禁开了稳定可复现的旁路。
//
// 第二路吃**重定向事实面**（tools#175）：Destructive 契约把「写设备」列为
// 覆盖例，但写设备常经重定向到达（`echo x > /dev/sda` 与 `dd of=/dev/sda`
// 同效）——Redirs 事实本就在判定结果里，只看调用集会让同一破坏面按写法分两档。
func (t *Table) destructiveScan(invs []Invocation, redirs []RedirInfo) (bool, string) {
	for _, inv := range invs {
		if inv.Name == "" {
			continue
		}
		if ok, why := t.destructiveVerdict(inv.Name, inv.Args); ok {
			return true, why
		}
		// 内核兜底（tools#44）：全量集合不剥包装，sudo/doas 这类**不在
		// Commands 表**的调度器（sudo/xargs 形态：内核命令是参数不是语法
		// 调用）按内核再判一次。已注册命令（echo/git …）的参数是数据，
		// 不参与——echo rm -rf / 不误报。
		if ok, why := t.destructiveKernel(inv); ok {
			return true, why
		}
	}
	return deviceWriteVerdict(redirs)
}

// deviceWriteVerdict 重定向事实面的设备写判定（tools#175）：写重定向
//（Read=false）目标静态落 /dev/ 且非已知无害汇点 ⇒ 不可逆设备写。
//
// 目标词含动态展开时目标不可静态判定，不入本判定（仍由只读门的保守拒与
// Redirs 事实面留给调用方细分）——与 Invocation.Dynamic 的处置同法。
//
// 无害汇点之外的 /dev/ 节点一律视为设备写（保守侧：块设备 sd*/nvme*、
// LVM mapper、mem/kmem 及未来未知节点——多报的代价只是多一次确认，
// 漏报的代价是设备真被覆写）。
func deviceWriteVerdict(redirs []RedirInfo) (bool, string) {
	for _, r := range redirs {
		if r.Read || r.Dynamic || !strings.HasPrefix(r.Target, "/dev/") {
			continue
		}
		if harmlessDevTarget(strings.TrimPrefix(r.Target, "/dev/")) {
			continue
		}
		return true, fmt.Sprintf("写重定向到设备 %s（不可逆设备写）", r.Target)
	}
	return false, ""
}

// devHarmlessSinks 已知无害设备汇点（整名匹配）：写它们不产生不可逆破坏面。
// 常见 `> /dev/null`、`2> /dev/null` 形态不得被设备写判据误报。
var devHarmlessSinks = map[string]bool{
	"null": true, "zero": true, "full": true,
	"stdout": true, "stderr": true, "stdin": true,
	"tty": true, "console": true,
	"random": true, "urandom": true,
}

// harmlessDevTarget /dev/ 下无害目标的判定：整名命中无害汇点，或落入
// 按前缀族豁免的子树（fd/N 进程描述符、pts/N 伪终端、shm/ 内存文件、
// tty* 串口/终端族——写它们是 IO 不是设备覆写）。
func harmlessDevTarget(rest string) bool {
	if rest == "" {
		return false
	}
	if devHarmlessSinks[rest] {
		return true
	}
	for _, prefix := range []string{"fd/", "pts/", "shm/", "tty"} {
		if strings.HasPrefix(rest, prefix) {
			return true
		}
	}
	return false
}

// destructiveKernel 未注册调度器的内核不可逆判定（sudo/xargs 形态）。
//
// 定位内核：跳过调度器自身的前置 flag（sudo -n 形态）后的首个参数视为内核
// 命令，其余参数归内核。方向保守（与 destructiveByRule 的兜底同一条纪律）：
// 多报的代价只是多要一次确认，漏报的代价是命令真跑了。
func (t *Table) destructiveKernel(inv Invocation) (bool, string) {
	if _, ok := t.Commands[inv.Name]; ok {
		return false, "" // 已注册命令：参数是数据（表内包装已在 destructiveVerdict 剥壳判过）
	}
	if _, ok := t.destructiveRule(inv.Name); ok {
		return false, "" // 自身命中不可逆表：destructiveVerdict 已按自身规则判过
	}
	if ok, why := t.kernelScan(inv.Args); ok {
		return true, fmt.Sprintf("%s 的内核 %s", inv.Name, why)
	}
	return false, ""
}

// kernelScan 内核兜底的全 token 扫描（tools#100/#101 单源）。
//
// 内核定位失败的两类形态都不能在首个对不上规则的 token 处放行——那会让真正
// 的内核 rm 再也轮不到判定：
//   - 调度器带值旗未登记（tools#100）：sudo -u nobody 的 nobody 是 -u 的值，
//     曾被当成内核候选，查不到规则即整体放行；
//   - 剥壳撞上未登记前置旗（tools#101）：env -S/-/--、timeout --kill-after
//     把 stripWrapper 卡在旗上，rest[0] 不是内核。
//
// 于是扫全部非 flag token，对每个命中不可逆规则的 token 逐一按内核判定（其后
// 的参数归它）；token 是已注册包装时递归整段判定（sudo env -S 'rm -rf /' 的
// -S 值拆词只有 env 自己的剥壳路径认识）。
//
// 保守取舍：值归属不明（哪个旗吃哪个参数无表可查），「后续位置上恰好出现
// 不可逆命令名」也当内核候选——sudo ls rm -rf /data 的 rm 明明是 ls 的操作数
// 也会多报。多报的代价只是多要一次确认，漏报的代价是命令真跑了；且这类形态
// 本就被只读门拒（sudo/env -S 都不在只读表），影响的只是风险档位。
func (t *Table) kernelScan(argv []string) (bool, string) {
	for i, tok := range argv {
		if tok == "" || strings.HasPrefix(tok, "-") {
			continue // 调度器/包装的前置 flag（值归属不明，不猜）
		}
		if _, ok := t.destructiveRule(tok); !ok {
			if pol, isCmd := t.Commands[tok]; !isCmd || !pol.Wrapper {
				continue // 既非不可逆内核、也非包装：纯数据/普通命令
			}
		}
		if ok, why := t.destructiveVerdict(tok, argv[i+1:]); ok {
			return true, why
		}
	}
	return false, ""
}

// destructiveVerdict 单次调用的不可逆判定。
func (t *Table) destructiveVerdict(name string, argv []string) (bool, string) {
	// 包装剥壳：env / nohup / timeout 形态里真正动手的是内核命令
	//（env rm -rf / 的不可逆性在 rm 上，不在 env 上）。
	// 剥壳参数表取只读表——那里才有包装自身的 flag 声明。
	if pol, ok := t.Commands[name]; ok && pol.Wrapper {
		if name == "env" {
			argv = expandEnvSplitString(argv)
		}
		rest := stripWrapper(argv, pol)
		if len(rest) == 0 {
			return false, ""
		}
		if strings.HasPrefix(rest[0], "-") {
			// 剥壳撞上未登记前置旗（tools#101）：内核定位不了——退化全 token
			// 扫描兜底（rest[0] 非 flag 时它就是内核，走精确递归不进兜底）
			return t.kernelScan(rest)
		}
		return t.destructiveVerdict(rest[0], rest[1:])
	}
	rule, ok := t.destructiveRule(name)
	if !ok {
		return false, ""
	}
	if hit, why := destructiveByRule(name, argv, rule); hit {
		return true, why
	}
	// 自身判据未命中再看内嵌命令载荷（tools#107）：find -exec rm -rf 的不可逆
	// 性在段内 rm 上，find 自身的判据（-delete）看不到它。
	if ok, why := t.execPayloadScan(name, argv, rule); ok {
		return true, why
	}
	// 「在别处执行」包装器的内层载荷（tools#117）：docker exec web rm -rf 的
	// 不可逆性在段内 rm 上，docker 自身不是可逆性命令——同 tools#107 的
	// 找不到自身判据问题，对每类包装按定位规则取内层段再判。
	return t.remoteExecScan(name, argv, rule)
}

// execPayloadScan 内嵌命令载荷的不可逆判定（tools#107）。
//
// find 的 -exec/-execdir/-ok/-okdir 载荷是一段内核命令（; 或 + 收界）：
// `find . -exec rm -rf {} +` 与 rm -rf 同效，但 find 登记在只读表里、不在
// 不可逆判据上——判定链两处对「已注册命令」早退（destructiveVerdict 无规则
// 即 false；destructiveKernel 把已注册命令的参数当数据），批量删除整体漏出
// 不可逆档。规则登记 ExecFlags 后对每段载荷按内核再判，与顶层 destructiveScan
// 的内核兜底同一条纪律：包装不得洗白内核。
//
// 段内判定走 kernelScan 全 token 扫描而非只看段首：段首命令（find 语法里
// -exec 后紧跟命令名）之外，段中还可能出现调度器（-exec sudo rm -rf … 的
// 内核 rm 不在段首）。保守取舍与 kernelScan 一致：段内数据 token 恰好命中
// 不可逆命令名时多报，代价只是多一次确认，漏报的代价是命令真跑了。
func (t *Table) execPayloadScan(name string, argv []string, rule Policy) (bool, string) {
	for i, a := range argv {
		isExec := false
		for _, f := range rule.ExecFlags {
			if a == f {
				isExec = true
				break
			}
		}
		if !isExec {
			continue
		}
		seg := argv[i+1:]
		for j, tok := range seg {
			if tok == ";" || tok == "+" { // find 载荷段界；缺界=残缺形态，取余段保守判定
				seg = seg[:j]
				break
			}
		}
		if ok, why := t.kernelScan(seg); ok {
			return true, fmt.Sprintf("%s 的 %s 段内内核不可逆：%s", name, a, why)
		}
	}
	return false, ""
}

// remoteExecScan 「在别处执行」包装器的内层不可逆判定（tools#117）。
//
// 容器（docker/podman exec|run|create）、命名空间（chroot/nsenter）、切用户
// （su -c）、瞬态服务（systemd-run）的内层命令与裸命令同效：docker exec web
// rm -rf /data 与 rm -rf /data 必须同档。此前一个都不在判定链上：
//   - docker 走 kernelScan 兜底，但内层是绝对路径（chroot /host /bin/rm）时
//     token 查不到规则（kernelScan 只认裸命令名），整体放行；
//   - kubectl 登记在不可逆表（delete 子命令）使 destructiveKernel 对 exec
//     早退，内层 rm 再也轮不到判定；
//   - su -c 的内核是 shell 文本，argv 定位根本取不到。
//
// 修法按 issue 主张表驱动：Policy.InnerCmd（名字 → 定位规则，InnerCmdLoc），
// 判定面复用既有机制——定位段交 kernelScan 全 token 扫描（与 execPayloadScan
// 的 find -exec 段同法：段内可再包调度器，docker exec web sudo rm -rf /），
// 段首绝对路径按 basename 再查一次规则，旗标值形态按 shell 文本走 Analyze
// 再判（宿主自建规则在值文本里同样生效）。保守取舍与 kernelScan 一致：定位
// 歪了只会让段变宽不会变窄，多报的代价只是多一次确认，漏报的代价是命令真跑了。
func (t *Table) remoteExecScan(name string, argv []string, rule Policy) (bool, string) {
	if len(rule.InnerCmd) == 0 && len(rule.Subcommands) == 0 {
		return false, "" // 非包装器规则（或规则未声明内层载荷）：不参与
	}
	// 本层规则（无子命令形态：chroot / nsenter / su / systemd-run）
	if ok, why := t.innerCmdScan(name, "", argv, rule.InnerCmd); ok {
		return true, why
	}
	// 子命令形态（docker exec / kubectl exec …）：首个命中 Subcommands 键的
	// token 之后整段归子命令（旗与值归属不猜——与 destructiveByRule 的兜底
	// 循环同法，定位歪了由段内 kernelScan 兜住）
	for i, tok := range argv {
		subRule, ok := rule.Subcommands[tok]
		if !ok {
			continue
		}
		if ok, why := t.innerCmdScan(name, tok, argv[i+1:], subRule.InnerCmd); ok {
			return true, why
		}
	}
	return false, ""
}

// innerCmdScan 按定位规则逐条取内层段并判内核。
func (t *Table) innerCmdScan(name, sub string, argv []string, rules []InnerCmdRule) (bool, string) {
	prefix := name
	if sub != "" {
		prefix = name + " " + sub
	}
	for _, r := range rules {
		if r.Loc == InnerFlagValue {
			// 旗标值是 shell 文本：按命令行整体再判一次（su -c 'rm -rf /data'
			// ——argv 定位取不到文本里的内核）
			if val, flag, ok := innerFlagValue(argv, r.Flags); ok {
				if d := Analyze(val, t); d.Destructive {
					return true, fmt.Sprintf("%s 的 %s 值内层不可逆：%s", prefix, flag, d.DestructiveReason)
				}
			}
			continue
		}
		if r.Loc == InnerOperandShellText {
			// 操作数**整体**是 shell 文本（tools#134：eval 'rm -rf /'；
			// tools#158：POSIX eval 把全部参数用空格拼接后整体求值——
			// eval a b c ≡ eval "a b c"）。只取首个操作数时 `eval rm -rf /`
			// 的 -rf / 被当 flag 丢掉，拼接后的完整命令行从未被评估；eval
			// 自身无旗标，内层的 -rf 恰恰是命令文本的组成部分。
			if len(argv) > 0 {
				if d := Analyze(strings.Join(argv, " "), t); d.Destructive {
					return true, fmt.Sprintf("%s 的操作数内层不可逆：%s", prefix, d.DestructiveReason)
				}
			}
			continue
		}
		if ok, why := t.innerKernelVerdict(locateInner(argv, r)); ok {
			return true, fmt.Sprintf("%s 的内层内核不可逆：%s", prefix, why)
		}
	}
	return false, ""
}

// locateInner 按定位形态取内层命令段。
func locateInner(argv []string, r InnerCmdRule) []string {
	switch r.Loc {
	case InnerAfterDashes:
		for i, a := range argv {
			if a == "--" {
				return argv[i+1:]
			}
		}
		return skipFlagAndOperand(argv, 1) // 缺 -- 退化（kubectl exec POD CMD 兜底）
	case InnerAfterOperand:
		return skipFlagAndOperand(argv, 1)
	case InnerFirstOperand:
		return skipFlagAndOperand(argv, 0)
	default:
		return nil
	}
}

// skipFlagAndOperand 跳过 flag token（值归属不猜）与 skip 个裸操作数，返回其后整段。
func skipFlagAndOperand(argv []string, skip int) []string {
	i := 0
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		i++
	}
	for ; skip > 0 && i < len(argv); skip-- {
		i++
	}
	return argv[i:]
}

// innerFlagValue 定位承载命令文本的旗标值：独立参数（-c VALUE）、内联长旗
// （--command=VALUE）、联合短旗内联（-cVALUE）与联合短旗内嵌（-lc VALUE，
// tools#159——`bash -lc 'rm -rf /'` 按 POSIX 即 -l -c，-c 的值同样是命令行；
// 只认 -c 打头拼写曾让 #134 的绕过形态在最常用变体下原样存活）四形态。
func innerFlagValue(argv []string, flags []string) (val, flag string, ok bool) {
	for i, a := range argv {
		for _, f := range flags {
			if a == f && i+1 < len(argv) {
				return argv[i+1], f, true
			}
			if strings.HasPrefix(a, f+"=") {
				return strings.TrimPrefix(a, f+"="), f, true
			}
			// 联合短旗内联：-cVALUE（shell 拼接引号后的单 token 形态）
			if len(f) == 2 && f[0] == '-' && f[1] != '-' &&
				strings.HasPrefix(a, f) && len(a) > len(f) {
				return a[len(f):], f, true
			}
			// 联合短旗内嵌：-lc/-ec 形态——c 不在 token 首位，按 getopt 语义
			// 取 c 之后的剩余部分为值（无剩余则值为下一参数，-lc 'rm…' 的
			// 载荷形态）。与只读侧 flagLookup 的联合短旗纪律同源：那边留着
			// 「曾经的真实漏判面」注释，不可逆档不该重演同一形态。
			if len(f) == 2 && f[0] == '-' && f[1] != '-' &&
				strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
				if idx := strings.IndexByte(a[1:], f[1]); idx >= 0 {
					if rest := a[idx+2:]; rest != "" {
						return rest, f, true
					}
					if i+1 < len(argv) {
						return argv[i+1], f, true
					}
				}
			}
		}
	}
	return "", "", false
}

// innerKernelVerdict 内层段的内核判定：段首可为绝对路径内核（chroot /host
// /bin/rm、systemd-run /usr/bin/rm——kernelScan 只认裸命令名，/bin/rm 曾查
// 不到规则整体放行），先按 basename 再查一次规则；其后交 kernelScan 全
// token 扫描兜底（段内可再包调度器：docker exec web sudo rm -rf /data）；
// 仍不命中时整段按 shell 文本再判一次（tools#162：ssh host 'rm -rf /data'
// 的内层以**单个含空格 token** 到达，kernelScan 逐词查表对整串不命中——
// 与 InnerFlagValue 的 su -c 同机制，宿主自建规则在文本里同样生效）。
func (t *Table) innerKernelVerdict(inner []string) (bool, string) {
	if len(inner) == 0 {
		return false, ""
	}
	if base := path.Base(inner[0]); base != inner[0] {
		if ok, why := t.destructiveVerdict(base, inner[1:]); ok {
			return true, why
		}
	}
	if ok, why := t.kernelScan(inner); ok {
		return true, why
	}
	if d := Analyze(strings.Join(inner, " "), t); d.Destructive {
		return true, d.DestructiveReason
	}
	return false, ""
}

// expandEnvSplitString 把 env 的 -S/--split-string 值原位展开成词序列
// （tools#101）：-S 的值本身就是一条命令行（env 自行拆词执行），不拆开的话
// 剥壳永远卡在旗上、内核不可见。只在不可逆面展开——只读面未登记 -S，仍按
// 未知 flag 拒（保守侧不变）。env 对 -S 串还有引号/转义处理（\n、${} 等），
// 这里按空白朴素拆词是保守近似：拆不出的形态只可能漏判，不会把没跑的当跑了。
func expandEnvSplitString(argv []string) []string {
	for i, a := range argv {
		var val string
		switch {
		case a == "-S" && i+1 < len(argv):
			val = argv[i+1]
		case strings.HasPrefix(a, "--split-string="):
			val = strings.TrimPrefix(a, "--split-string=")
		default:
			continue
		}
		out := append([]string{}, argv[:i]...)
		out = append(out, strings.Fields(val)...)
		if a == "-S" {
			out = append(out, argv[i+2:]...)
		} else {
			out = append(out, argv[i+1:]...)
		}
		return out
	}
	return argv
}

// destructiveRule 查不可逆规则：先精确命中，再前缀族（"mkfs*" 覆盖 mkfs.ext4 等）。
//
// basename 归一（tools#132）：规则表的键是裸命令名（rm / mkfs*），调用方的
// 命令 token 可能是绝对路径（/bin/rm、/usr/bin/mkfs.ext4）——1dbd1bd 只在
// innerKernelVerdict 段首做了归一，顶层/剥壳后/find 载荷的绝对路径内核一律
// 查不到规则。归一收口到本函数单源（innerKernelVerdict 的 path.Base 保留
// 无害，本函数再兜一次）；相对路径 ./rm 同样折算。
//
// 前缀族命中多个时取**最长前缀**（确定性：map 遍历序随机，不能让判定随遍历序变）。
func (t *Table) destructiveRule(name string) (Policy, bool) {
	if rule, ok := t.Destructive[name]; ok {
		return rule, true
	}
	if base := path.Base(name); base != name {
		if rule, ok := t.Destructive[base]; ok {
			return rule, true
		}
		// 前缀族对 basename 再试（/usr/bin/mkfs.ext4 → mkfs*）。
		if rule, ok := t.destructiveRule(base); ok {
			return rule, true
		}
	}
	best, bestLen := Policy{}, -1
	for key, rule := range t.Destructive {
		if !strings.HasSuffix(key, "*") {
			continue
		}
		prefix := strings.TrimSuffix(key, "*")
		if strings.HasPrefix(name, prefix) && len(prefix) > bestLen {
			best, bestLen = rule, len(prefix)
		}
	}
	return best, bestLen >= 0
}

// destructiveByRule 按规则判一次调用。
func destructiveByRule(name string, argv []string, rule Policy) (bool, string) {
	if rule.Destructive {
		return true, fmt.Sprintf("%s 是不可逆动作", name)
	}
	if f, ok := matchDestructiveFlag(argv, rule.DestructiveFlags); ok {
		return true, fmt.Sprintf("%s 带 %s（不可逆 flag）", name, f)
	}
	if a, ok := matchDestructiveArg(argv, rule.DestructiveArgs); ok {
		return true, fmt.Sprintf("%s 带 %s（不可逆参数）", name, a)
	}
	if len(rule.Subcommands) > 0 {
		if sub, rest, reason := splitSubcommand(argv, rule); reason == "" && sub != "" {
			if subRule, ok := rule.Subcommands[sub]; ok {
				return destructiveByRule(name+" "+sub, rest, subRule)
			}
		}
		// 兜底：本层 flag 不在表里 → 定位不了子命令。安全方向是**多报**而不是漏报，
		// 于是退一步扫全部非 flag token（可能把同名实参当成子命令 ⇒ 假阳性，
		// 但假阳性的代价只是多要一次确认，假阴性的代价是命令真跑了）。
		// 递归传**子命令之后的全部 token**（tools#133）：传 nil 曾把 flag 判据
		// 全丢——`git --no-literal-pathspecs reset --hard` 的 --hard 在兜底递归
		// 里被丢掉，reset{--hard} 判据失明。（tok 之后的 token 才是子命令的
		// argv；tok 之前的本层旗对子命令无意义，与正路 splitSubcommand 的 rest
		// 同一语义。）
		for i, tok := range argv {
			if strings.HasPrefix(tok, "-") {
				continue
			}
			if subRule, ok := rule.Subcommands[tok]; ok {
				rest := append([]string{}, argv[i+1:]...)
				return destructiveByRule(name+" "+tok, rest, subRule)
			}
		}
	}
	return false, ""
}

// matchDestructiveFlag 判断 argv 里是否出现该不可逆 flag，返回命中的那个。
//
// 短 flag 支持**联合形态**（rm -rf 里的 r）：GNU 短 flag 可折叠，
// 而「递归删除」几乎都写成 rm -rf / rm -r。
func matchDestructiveFlag(argv []string, flags []string) (string, bool) {
	for _, a := range argv {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return f, true
			}
			// 联合短 flag：-rf 含 r（长 flag --foo 不参与）
			if len(f) == 2 && f[0] == '-' && f[1] != '-' &&
				strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 &&
				strings.ContainsRune(a[1:], rune(f[1])) {
				return f, true
			}
		}
	}
	return "", false
}

// matchDestructiveArg 判断 argv 里是否有参数命中该不可逆参数（**前缀**匹配）。
//
// 前缀而非全等：`of=` 要命中的是 `of=/dev/sda`（dd 的写设备目标），
// 而不是一个恰好等于 "of=" 的参数。
func matchDestructiveArg(argv []string, args []string) (string, bool) {
	for _, a := range argv {
		for _, want := range args {
			if want != "" && strings.HasPrefix(a, want) {
				return want, true
			}
		}
	}
	return "", false
}

// Package bashguard bash 命令静态风险解析：不执行命令，经真 shell AST
// （mvdan.cc/sh）解析成调用序列，按策略表判定只读性。对标 ZCode
// bash-command-parser.ts + bash-readonly-policy 的「权限判定不执行命令」。
//
// 判定原则（保守到宁可误报）：
//   - 解析失败、含动态词（$() / 反引号 / 未展开 $VAR）、含写重定向、
//     复合控制流（if/for/while/case/函数定义）→ 一律非只读——无法静态证明
//     安全的就不放行；
//   - 调用按策略表：命令不在表=未知（非只读）；命中危险 flag=非只读；
//     未知 flag=非只读；值型 flag 吃一个参数后继续；
//   - 安全包装（env/nice/nohup/time/timeout/stdbuf/xargs）剥壳后判定内核；
//   - 复合命令（&& || ; |）每一段都必须只读，整体才只读。
//
// # 三档：只读 / 写 / 不可逆
//
// 只读判定回答「会不会改东西」；**不可逆判定**（Decision.Destructive）回答
// 「一旦跑就收不回来」——两者正交：rm file 不是只读但可逆，rm -rf / 不可逆。
// 不可逆判据走**独立的一张表**（Table.Destructive，见 destructive.go）：
// 只读表只放只读命令，而不可逆判据要覆盖 rm / mkfs / dd 这类写命令。
// 「要不要二次确认」是调用方的策略决定；本包给的是「这条命令不可逆」这个事实。
//
// 上限 10K 字符（对标 ZCode：超长命令不进解析器）。超长路径带 Unevaluated
// 标记（细分归因 OverLength / Unparsed，tools#80）+ 头/尾窗 best-effort
// 不可逆扫描——不静默 fail-open。
package bashguard

import (
	"fmt"
	"strconv"
	"strings"

	mvdan "mvdan.cc/sh/v3/syntax"
)

// MaxCommandLen 解析上限。
const MaxCommandLen = 10 * 1024

// FlagKind 值型 flag 的参数形态。
type FlagKind string

const (
	FlagBool     FlagKind = "bool"     // 不吃参数
	FlagString   FlagKind = "string"   // 吃一个任意字面参数
	FlagNumber   FlagKind = "number"   // 吃一个数值参数
	FlagOptional FlagKind = "optional" // 可带可不带参数（按字面形态分辨）
)

// Policy 单命令策略。
type Policy struct {
	// ReadOnly 命令本体是否只读。
	ReadOnly bool
	// SafeFlags 已知安全 flag（含 - 前缀，如 "-n"）。长 flag 单写。
	SafeFlags map[string]FlagKind
	// UnsafeFlags 命中即非只读（显式优先于 SafeFlags）。
	UnsafeFlags []string
	// Subcommands 子命令策略表（git 形态，可嵌套）：子命令不在表=非只读。
	Subcommands map[string]Policy
	// Wrapper 安全包装（env/timeout 形态）：剥壳后按剩余首参判定。
	Wrapper bool
	// ConsumeBare Wrapper 剥 flag 后先吃掉的前置裸操作数个数（timeout 的时长）。
	ConsumeBare int
	// NoBareOperands 裸操作数即写形态（git branch <名> 创建 / git stash 裸 push /
	// git tag <名> 打标）：出现任何 flag 之外的裸操作数 → 非只读。
	NoBareOperands bool
	// AllowBareOperandsPrefix NoBareOperands 的豁免前缀（date 的 +FORMAT 形态：
	// 裸操作数以 "+" 开头是格式串不是写目标）。前缀空串不放行任何操作数。
	AllowBareOperandsPrefix []string
	// CheckArgs 参数深度校验钩子（tools#81）：flag 表表达不了的 mini-language
	// 参数面（sed 脚本：w/W 写文件、e/s///e 执行命令可藏于裸操作数与 -e/
	// --expression 的**值**里，flag 白名单与 NoBareOperands 都看不到内容）。
	// 收到完整 argv（含 flag 及其值），返回非空串=非只读原因。
	//
	// 调用时机：叶子路径（evalLeafPolicy）在白名单 flag 消费**之后**——未知
	// flag 先被表拒，钩子只处理表放过来的形态；子命令/包装路径不调用。实现
	// 必须自带 fail-closed 纪律（看不懂的一律拒），钩子内不执行任何命令。
	CheckArgs func(name string, argv []string) string
	// Destructive 该命令恒不可逆（删数据 / 写设备 / 关机 …）。
	//
	// 只在**不可逆规则表**（Table.Destructive）里有意义——只读表是只读表，
	// 不可逆判据单列一张（见 destructive.go 的说明）。
	Destructive bool
	// DestructiveFlags 命中即不可逆的 flag（"-r" / "--recursive"）。
	// 短 flag 支持联合形态（-rf 里的 r）。
	DestructiveFlags []string
	// DestructiveArgs 命中即不可逆的参数（**前缀**匹配："of=" 命中 "of=/dev/sda"）。
	DestructiveArgs []string
	// ExecFlags 不可逆面的内嵌命令旗标（tools#107）：flag 的载荷是一段内核
	// 命令（find -exec CMD [arg…] ; / + 形态）。不可逆判定对每段载荷按内核
	// 再判——包装不得洗白内核（与 xargs/sudo/env 剥壳同一条纪律）：find 登记
	// 在只读表、判定链把已注册命令的参数当数据，rm -rf 写进 -exec 段曾整体
	// 漏出不可逆档。只在不可逆规则表里有意义；载荷段以 ";" 或 "+" token 收界
	//（find 语法），缺界取余段保守判定。
	ExecFlags []string
}

// Table 策略表。
type Table struct {
	Commands map[string]Policy
	// Destructive 不可逆规则表：命令名 → 判据（Policy 的 Destructive* 三字段
	// 之外还消费 Subcommands / SafeFlags——子命令定位与 git 族规则，tools#107——
	// 与 ExecFlags——内嵌命令载荷，tools#107）。
	//
	// 与只读表**分开**：只读表只放只读命令（"不在只读策略表" 是既有语义），
	// 而不可逆判据要覆盖 rm / mkfs / dd 这类写命令。键尾的 "*" 表示前缀族
	//（"mkfs*" 覆盖 mkfs.ext4 等）。
	Destructive map[string]Policy
}

// Invocation 提取的单次调用（观测面）。
type Invocation struct {
	Name    string
	Args    []string
	Dynamic bool // 含动态词（argv 非字面）
}

// RedirInfo 重定向事实（观测面，tools#79）。
//
// 收编动机与 AllCalls 同型：重定向判定（`cat disk.img > /dev/sda`——写块设备
// 走的是 Stmt.Redirs 不是调用面）此前集成方只能自己再 parse 一遍同一命令
// （Analyze 内部一次 + 集成方一次），重复实现会漂、漂的那份决定门禁拦什么。
// Decision.Redirs 给出全 AST 的重定向集，集成方在同一判定结果上做目标检查
// （如 /dev/* 设备文件），解析 2→1。
//
// 语义边界：
//   - 只读判定的保守结论不变——任何写重定向（Read=false）本身就把命令判非
//     只读；本结构是给「非只读之后还要细分目标」的调用方的事实面；
//   - heredoc（<< / <<- / <<<）的 Target 是定界符词，**体不在本结构**（体的
//     动态展开由只读判定自身保守处理）；
//   - &> / &>> 同时作用于 fd 1 与 2，FD 记 1（Op 已含全量信息）。
type RedirInfo struct {
	Op      string // 操作符字面（">" / ">>" / "&>" / "<" / "<<" / "<<<" …）
	FD      int    // 作用的目标 fd：">" 默认 1、"2>" 为 2；{varname}> 形态为 -1（fd 由运行时变量决定）
	Target  string // 目标词字面化文本（heredoc 为定界符；含展开时为拼接的部分文本，Dynamic=true）
	Dynamic bool   // 目标词含动态展开（$()/反引号/$VAR）
	Read    bool   // 读侧重定向（<、<<、<<-、<<<）：目标是被读的源，不产生写
}

// Decision 判定结果。
type Decision struct {
	// ReadOnly 命令可判只读（不执行任何写路径）。
	ReadOnly bool
	// Reason 非只读原因（人可读，审计用）。
	Reason string
	// Destructive 该调用含不可逆动作（删数据 / 写设备 / 关机 / 递归改权限 …）。
	//
	// 与 ReadOnly **正交**：只读与否是另一档。一个命令可以「不是只读、但可逆」
	//（rm file），也可以「不是只读、且不可逆」（rm -rf /）。
	//
	// 语义边界：false 只意味着「没命中已知规则」，**不意味着安全**——
	// 是否要二次确认是调用方的策略决定，本包只给「这条命令不可逆」这个事实。
	Destructive bool
	// DestructiveReason 不可逆依据（人可读，审计用）；未命中时为空。
	DestructiveReason string
	// Unevaluated 命令未进 AST 判定（超长未进解析器 / 解析失败，tools#45）。
	//
	// 此时 ReadOnly=false 只是保守拒，**Destructive=false 不再意味着「判了
	// 不不可逆」**——未评估就是不可判：「判据不可判」与「判了不不可逆」不得
	// 压成同一个信号。调用方对未评估输入须按保守侧处理（如照常要二次确认），
	// 不得当「已判安全」。超长路径另有头/尾窗 best-effort 扫描兜底（见
	// overlongDecision）：窗口内命中的不可逆动作照常置 Destructive。
	//
	// 细分归因见 OverLength / Unparsed（tools#80）：未评估的伞标记在此，
	// 两细分只回答「为什么没判」，两者置位时本标记必置位。
	Unevaluated bool
	// OverLength 命令超过 MaxCommandLen 未进解析器（tools#80）：Unevaluated
	// 的细分归因之一。集成方据此区分「超长拒绝判定」与「解析失败拒绝判定」，
	// 并与「判定完成没发现风险」（Unevaluated/OverLength/Unparsed 全 false）
	// 相区分——拒绝判定（任一标记置位）时 ReadOnly=false 只是保守拒，命令
	// 文本里的实际风险（如 10KB 注释填充后尾随的 rm -rf …）未被评估，调用方
	// 须自行升级处置（如照常要确认）。
	OverLength bool
	// Unparsed 解析失败未进 AST 判定（tools#80）：Unevaluated 的细分归因之一，
	// 语义同 OverLength（拒绝判定，须按保守侧处理）。
	Unparsed bool
	// Invocations 提取的调用序列（观测/风险面）。
	//
	// ⚠️ 这是**判定路径**的消费序列，不是全量事实：判定在首个非只读调用上
	// 提前返回（Analyze 的 break），其后的调用不再评估；包装剥壳后判定的是
	// 内核，调用面记录的是包装（sudo -n rm 只见 sudo）。做确认门禁/风险审计
	// 请用 AllCalls（见其注释）。
	Invocations []Invocation
	// AllCalls 全 AST 的 simple command 集合（syntax.Walk，oagentkit/tools#35）：
	// 管道后半、命令替换 $(…)、子 shell (…)、{ …; } 里的调用**全部可见**，
	// 与判定路径是否提前返回无关。
	//
	// 与 Invocations 的分工：Invocations 服务 ReadOnly/Destructive 判定的
	// 归因（为何判非只读）；AllCalls 服务「这条命令文本里**出现了什么**」的
	// 风险面事实——jumpserver-kit 此前为补这个集合在守卫层自接了一份
	// syntax.Walk（判定门禁的绕过全堵上依赖它），重复实现会漂、漂的那份
	// 决定门禁拦什么，故上游收编。
	//
	// 语义边界（调用方须知）：
	//   - 包装不剥壳：sudo -n rm -rf /x 是一条调用（sudo，args 含 rm…）——
	//     sudo/xargs/env 的内核是**参数**不是语法调用，消费方按需自剥
	//     （本包自身的不可逆扫描已对未注册调度器做内核兜底，见
	//     destructive.go 的 destructiveKernel）；
	//   - eval "rm -rf /x" 的 rm 是字符串数据不是语法调用，只见 eval；
	//   - Dynamic=true 的调用（参数含 $()/反引号/$VAR）同样入列——动态参数
	//     本身就是风险信号。
	AllCalls []Invocation
	// Redirs 全 AST 的重定向事实集（syntax.Walk，tools#79）：语句级、子 shell
	// (…)、{ …; }、命令替换 $(…) 里的重定向全部可见，与判定路径是否提前返回
	// 无关。服务集成方的重定向目标判定（写块设备等走 Redirs 不走调用面），
	// 免集成方为拿重定向再 parse 一遍同一命令（与 AllCalls 的收编动机同型）。
	//
	// 语义边界见 RedirInfo：heredoc 体不在此结构；写重定向（Read=false）本身
	// 已使命令非只读，本事实面供非只读之后的目标细分。超长路径为头/尾窗
	// best-effort 收集（重叠区去重，与 AllCalls 同法）；解析失败为空。
	Redirs []RedirInfo
}

// collectAllCalls 全 AST 收集 simple command（syntax.Walk，f 为 nil 返回空）。
func collectAllCalls(f *mvdan.File) []Invocation {
	if f == nil {
		return nil
	}
	var out []Invocation
	mvdan.Walk(f, func(n mvdan.Node) bool {
		call, ok := n.(*mvdan.CallExpr)
		if !ok {
			return true // 继续下探
		}
		dynamic := false
		for _, a := range call.Assigns {
			if a.Value == nil {
				continue
			}
			if _, dyn := literalWord(a.Value); dyn {
				dynamic = true
			}
		}
		name := ""
		argv := make([]string, 0, len(call.Args))
		for i, w := range call.Args {
			s, dyn := literalWord(w)
			if dyn {
				dynamic = true
			}
			if i == 0 {
				name = s
				continue
			}
			argv = append(argv, s)
		}
		out = append(out, Invocation{Name: name, Args: argv, Dynamic: dynamic})
		return true // 继续下探：参数词里可有命令替换 $(cmd)（内层调用是独立 CallExpr）
	})
	return out
}

// collectRedirs 全 AST 收集重定向事实（syntax.Walk，f 为 nil 返回空）。
func collectRedirs(f *mvdan.File) []RedirInfo {
	if f == nil {
		return nil
	}
	var out []RedirInfo
	mvdan.Walk(f, func(n mvdan.Node) bool {
		r, ok := n.(*mvdan.Redirect)
		if !ok {
			return true // 继续下探：子 shell / { …; } / $(…) 里可有重定向
		}
		out = append(out, redirInfo(r))
		return true
	})
	return out
}

// redirInfo 单条重定向的事实化：操作符字面、目标 fd、目标词字面化。
func redirInfo(r *mvdan.Redirect) RedirInfo {
	info := RedirInfo{Op: r.Op.String(), Read: readRedirOps[r.Op]}
	switch {
	case r.N != nil: // 显式 fd（2> 形态）；{varname}> 形态 fd 由运行时变量决定
		if fd, err := strconv.Atoi(r.N.Value); err == nil {
			info.FD = fd
		} else {
			info.FD = -1
		}
	default:
		info.FD = defaultFD(r.Op)
	}
	if r.Word != nil {
		info.Target, info.Dynamic = literalWord(r.Word)
	}
	return info
}

// defaultFD 无显式 fd 时各重定向操作符的缺省目标 fd；&>/&>> 族同时作用于
// 1 与 2，记 1（Op 字面已含全量信息）。
func defaultFD(op mvdan.RedirOperator) int {
	switch op {
	case mvdan.RdrIn, mvdan.DplIn, mvdan.Hdoc, mvdan.DashHdoc, mvdan.WordHdoc:
		return 0
	}
	return 1
}

// Analyze 静态解析与判定。超长/解析失败返回非只读（保守）且 Unevaluated=true。
func Analyze(cmdStr string, t *Table) Decision {
	if t == nil {
		t = defaultTable() // 只读共享模板，免每调用拷贝整表
	}
	if len(cmdStr) > MaxCommandLen {
		return t.overlongDecision(cmdStr)
	}
	parser := mvdan.NewParser(mvdan.Variant(mvdan.LangBash))
	f, err := parser.Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return Decision{Reason: fmt.Sprintf("解析失败: %v", err), Unevaluated: true, Unparsed: true}
	}
	d := Decision{ReadOnly: true, AllCalls: collectAllCalls(f), Redirs: collectRedirs(f)}
	for _, stmt := range f.Stmts {
		if reason := t.evalStmt(stmt, &d.Invocations); reason != "" {
			d.ReadOnly = false
			d.Reason = reason
			break
		}
	}
	// 不可逆判定**独立于**只读判定：命令被只读门拒了（rm 不在只读表），
	// 它仍可能是不可逆的——而「这条命令不可逆」正是调用方要据此要二次确认的
	// 那个事实。只读结论先出也不影响它。扫描面是**全量 AllCalls**（tools#44）：
	// Invocations 是判定路径的消费序列，首个非只读语句即 break、其后的调用
	// 不在列——曾用它扫不可逆，前置触礁语句（未知命令/写重定向/动态展开/
	// 控制流）可稳定藏住后置的 rm -rf /。
	if ok, why := t.destructiveScan(d.AllCalls); ok {
		d.Destructive = true
		d.DestructiveReason = why
	}
	return d
}

// overlongDecision 超长命令的保守收口（tools#45）：超长不进解析器是既有
// 上限，但此前直接 Destructive=false——一条 10K+1 字节、前面塞满注释、结尾
// rm -rf / 的命令据此绕过不可逆二次确认（「判据不可判」与「判了不不可逆」
// 被压成同一个信号，且无任何「未扫描」标记供调用方 fail-closed）。两层保守：
//   - Unevaluated=true：显式「未评估」标记，调用方按保守侧处理（未评估 ≠ 安全）；
//   - 头/尾各 MaxCommandLen 字节窗口的 best-effort 解析扫描：payload 位置
//     攻击者可控，头窗覆盖前缀形态，尾窗覆盖「垃圾铺垫 + 尾部收尾」的实测
//     攻击形态。窗口截断致语法破损时该窗口作废（Unevaluated 已兜底保守侧）。
func (t *Table) overlongDecision(cmdStr string) Decision {
	d := Decision{
		Reason:      fmt.Sprintf("命令超长（%d 字节 > %d 上限），不进解析器", len(cmdStr), MaxCommandLen),
		Unevaluated: true,
		OverLength:  true,
	}
	seen := map[string]bool{}
	for _, w := range truncatedWindows(cmdStr) {
		f, err := mvdan.NewParser(mvdan.Variant(mvdan.LangBash)).Parse(strings.NewReader(w), "")
		if err != nil {
			continue // 残缺窗口作废：best-effort，未评估标记才是保守侧的兜底
		}
		for _, inv := range collectAllCalls(f) {
			key := inv.Name + "\x00" + strings.Join(inv.Args, "\x00")
			if inv.Dynamic {
				key += "\x00dyn"
			}
			if seen[key] {
				continue // 头/尾窗重叠区去重（AllCalls 是事实集，不重复计数）
			}
			seen[key] = true
			d.AllCalls = append(d.AllCalls, inv)
		}
		for _, ri := range collectRedirs(f) { // 重定向事实面同法收集去重（tools#79）
			key := fmt.Sprintf("%s\x00%d\x00%s\x00%v\x00%v", ri.Op, ri.FD, ri.Target, ri.Dynamic, ri.Read)
			if seen[key] {
				continue
			}
			seen[key] = true
			d.Redirs = append(d.Redirs, ri)
		}
	}
	if ok, why := t.destructiveScan(d.AllCalls); ok {
		d.Destructive = true
		d.DestructiveReason = why
	}
	return d
}

// truncatedWindows 超长命令的 best-effort 解析窗口：头/尾各 MaxCommandLen
// 字节。尾窗先丢掉截断产生的残缺首行（从完整行边界起步，避免半行词干扰
// 解析；头窗的残缺尾词只是数据，无须处理）。
func truncatedWindows(s string) []string {
	head := s[:MaxCommandLen]
	tail := s[len(s)-MaxCommandLen:]
	if i := strings.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	if head == tail {
		return []string{head}
	}
	return []string{head, tail}
}

// evalStmt 评估单个语句；返回空串=只读，非空=非只读原因。
func (t *Table) evalStmt(stmt *mvdan.Stmt, invocations *[]Invocation) string {
	if reason := redirectsReason(stmt.Redirs); reason != "" {
		return reason
	}
	return t.evalCmd(stmt.Cmd, invocations)
}

// readRedirOps 读侧重定向（不产生写）：< 、<< heredoc、<<< word-heredoc。
var readRedirOps = map[mvdan.RedirOperator]bool{
	mvdan.RdrIn: true, mvdan.Hdoc: true, mvdan.DashHdoc: true, mvdan.WordHdoc: true,
}

func redirectsReason(redirs []*mvdan.Redirect) string {
	for _, r := range redirs {
		if readRedirOps[r.Op] {
			// 输入侧重定向的**目标词与 heredoc 体**同样必须字面：真实 shell 会
			// 执行 `< $(cmd)`、`< <(cmd)`、`<<< $(cmd)` 与未引号定界 heredoc 体
			// 里的命令替换——只看 Op 放行曾让 `cat < $(rm -rf build)` 全程判
			// 只读（第六轮审计 C 级；引号定界 `<<'EOF'` 的体是纯字面，不受影响）
			if r.Word != nil {
				if _, dyn := literalWord(r.Word); dyn {
					return "读侧重定向目标含动态展开（< $(…)/< <(…) 会执行）"
				}
			}
			if r.Hdoc != nil {
				if _, dyn := literalWord(r.Hdoc); dyn {
					return "heredoc 体含动态展开（未引号定界符下 $(…) 会执行）"
				}
			}
			continue
		}
		return fmt.Sprintf("写重定向 op=%d", int(r.Op))
	}
	return ""
}

func (t *Table) evalCmd(node mvdan.Command, invocations *[]Invocation) string {
	switch c := node.(type) {
	case *mvdan.CallExpr:
		return t.evalCall(c, invocations)
	case *mvdan.BinaryCmd:
		if reason := t.evalStmt(c.X, invocations); reason != "" {
			return reason
		}
		return t.evalStmt(c.Y, invocations)
	case *mvdan.Subshell:
		for _, s := range c.Stmts {
			if reason := t.evalStmt(s, invocations); reason != "" {
				return reason
			}
		}
		return ""
	case *mvdan.Block:
		for _, s := range c.Stmts {
			if reason := t.evalStmt(s, invocations); reason != "" {
				return reason
			}
		}
		return ""
	case *mvdan.TimeClause:
		// bash 保留字 time [-p]：对被计时语句递归判定（mvdan 在 bash 模式把
		// `time cmd` 解析为 TimeClause 而非 CallExpr——表里的 time 条目只覆盖
		// 引用形态，`time ls` 曾因走不到而报"不支持的控制流"误拒）
		return t.evalStmt(c.Stmt, invocations)
	default:
		return fmt.Sprintf("不支持的控制流 %T（if/for/while/case/函数等无法静态证明只读）", node)
	}
}

// evalCall 单调用判定：字面化 argv → 剥安全包装 → 策略表。
func (t *Table) evalCall(call *mvdan.CallExpr, invocations *[]Invocation) string {
	var argv []string
	dynamic := false
	for _, w := range call.Args {
		s, dyn := literalWord(w)
		if dyn {
			dynamic = true
		}
		argv = append(argv, s)
	}
	// 前置环境赋值（FOO=… cmd）的右值同样必须字面：赋值里的命令替换/参数展开
	// 在真实 shell 里先于内核命令执行——X=$(rm -rf build) ls 的副作用判定面
	// 曾完全看不到（第五轮审计修复）
	for _, a := range call.Assigns {
		if a.Naked || a.Value == nil {
			continue
		}
		if _, dyn := literalWord(a.Value); dyn {
			name := ""
			if len(call.Args) > 0 {
				n, _ := literalWord(call.Args[0])
				name = n
			}
			invocationsAdd(invocations, Invocation{Name: name, Args: argv, Dynamic: true})
			return fmt.Sprintf("前置赋值 %s=… 含动态展开（命令替换/参数展开先于命令执行）", a.Name.Value)
		}
	}
	// 命令名本身动态 = 无法判定目标
	name := ""
	if len(call.Args) > 0 {
		n, dyn := literalWord(call.Args[0])
		if dyn {
			return "命令名含动态展开（$()/反引号/$VAR），无法静态判定"
		}
		name = n
		argv = argv[1:]
	} else if len(call.Assigns) > 0 {
		return "纯环境赋值语句（VAR=…）不带调用，无法证明后续用途"
	}
	if dynamic {
		invocationsAdd(invocations, Invocation{Name: name, Args: argv, Dynamic: true})
		return fmt.Sprintf("参数含动态展开（%s …），无法静态判定", name)
	}
	invocationsAdd(invocations, Invocation{Name: name, Args: argv})

	return t.evalInvocation(name, argv)
}

// evalInvocation 表驱动判定（递归剥包装）。
func (t *Table) evalInvocation(name string, argv []string) string {
	pol, ok := t.Commands[name]
	if !ok {
		return fmt.Sprintf("命令 %q 不在只读策略表", name)
	}
	if !pol.ReadOnly && !pol.Wrapper {
		return fmt.Sprintf("命令 %q 策略为非只读", name)
	}
	if pol.Wrapper {
		// 剥壳：跳过包装自身的 flag 与 KEY=VAL 赋值，判定内核命令
		rest := stripWrapper(argv, pol)
		if len(rest) == 0 {
			return "" // 纯包装无内核（env 裸跑=打印环境）
		}
		return t.evalInvocation(rest[0], rest[1:])
	}
	return evalPolicy(name, argv, pol)
}

// stripWrapper 剥包装参数：KEY=VAL 赋值、本包装已声明的安全 flag、ConsumeBare
// 个前置裸操作数（timeout 的时长）。
func stripWrapper(argv []string, pol Policy) []string {
	rest := argv
	for len(rest) > 0 {
		a := rest[0]
		if strings.Contains(a, "=") && !strings.HasPrefix(a, "-") {
			rest = rest[1:]
			continue
		}
		if kind, ok := pol.SafeFlags[a]; ok {
			rest = rest[1:]
			next, has := "", len(rest) > 0
			if has {
				next = rest[0]
			}
			if valueFlagEats(kind, a, next, has) {
				rest = rest[1:]
			}
			continue
		}
		break
	}
	// ConsumeBare 只吃**裸**操作数（timeout 的时长）：前置剥壳撞上未登记 flag
	// break 出来时，旗 token 不能当裸操作数吞掉——否则它顶替了真正的时长位置，
	// rest[0] 变成时长/内核错位，内核定位整体失真（tools#101）
	for i := 0; i < pol.ConsumeBare && len(rest) > 0 && !strings.HasPrefix(rest[0], "-"); i++ {
		rest = rest[1:]
	}
	return rest
}

// valueFlagEats 值型 flag 是否吞并紧随的独立参数（非内联 = 值、下一个 token
// 非 flag 形态）。consumeFlags/splitSubcommand/stripWrapper 三处同构单源
// （第六轮审计收敛——此前 stripWrapper 漏判内联 = 形态）。
func valueFlagEats(kind FlagKind, flag, next string, hasNext bool) bool {
	return kind != FlagBool && !strings.Contains(flag, "=") &&
		hasNext && !strings.HasPrefix(next, "-")
}

// evalPolicy 策略执行：危险 flag 显式优先 → 子命令先剥离（子命令的 flag 归
// 子命令策略）→ 白名单 flag 消费（未知即拒）→ 裸操作数守卫。
func evalPolicy(name string, argv []string, pol Policy) string {
	if reason := rejectUnsafeFlags(name, argv, pol); reason != "" {
		return reason
	}
	if len(pol.Subcommands) > 0 {
		return evalSubcommandPolicy(name, argv, pol)
	}
	return evalLeafPolicy(name, argv, pol)
}

// rejectUnsafeFlags 危险 flag 显式优先（含 --flag=value 内联形态）。
func rejectUnsafeFlags(name string, argv []string, pol Policy) string {
	for _, a := range argv {
		for _, u := range pol.UnsafeFlags {
			if a == u || strings.HasPrefix(a, u+"=") {
				return fmt.Sprintf("%s 的 %s 是写路径 flag", name, u)
			}
		}
	}
	return ""
}

// evalSubcommandPolicy 子命令分派：先定位子命令 token（值型 flag 吃参后首个
// 非 flag 项），其后整段交给子命令策略——git log --oneline 的 --oneline 属于
// log 不属于 git。
func evalSubcommandPolicy(name string, argv []string, pol Policy) string {
	sub, rest, giReason := splitSubcommand(argv, pol)
	if giReason != "" {
		return giReason
	}
	if sub == "" {
		if pol.NoBareOperands {
			return name + " 裸调用是写形态（如 stash 默认 push）"
		}
		return ""
	}
	subPol, ok := pol.Subcommands[sub]
	if !ok {
		return fmt.Sprintf("%s 子命令 %q 不在只读表", name, sub)
	}
	if !subPol.ReadOnly && !subPol.Wrapper {
		return fmt.Sprintf("%s 子命令 %q 非只读", name, sub)
	}
	if subPol.Wrapper {
		return "嵌套包装子命令不支持"
	}
	return evalPolicy(name+" "+sub, rest, subPol)
}

// evalLeafPolicy 叶子命令：白名单 flag 消费（未知即拒）+ 参数深度校验钩子 +
// 裸操作数守卫。
func evalLeafPolicy(name string, argv []string, pol Policy) string {
	operands, reason := consumeFlags(name, argv, pol)
	if reason != "" {
		return reason
	}
	if pol.CheckArgs != nil {
		if reason := pol.CheckArgs(name, argv); reason != "" {
			return reason
		}
	}
	if pol.NoBareOperands {
		for _, op := range operands {
			if !allowedBareOperand(op, pol.AllowBareOperandsPrefix) {
				return fmt.Sprintf("%s 带裸操作数 %q 是写形态（创建分支/标签/弹栈/设时钟等）", name, op)
			}
		}
	}
	return ""
}

// allowedBareOperand NoBareOperands 守卫的豁免前缀判定（date +FORMAT 形态）。
func allowedBareOperand(op string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(op, p) {
			return true
		}
	}
	return false
}

// consumeFlags 按白名单消费 flag（-- 后全是操作数）；返回裸操作数与拒因。
func consumeFlags(name string, argv []string, pol Policy) (operands []string, reason string) {
	i := 0
	afterSep := false
	for i < len(argv) {
		a := argv[i]
		if afterSep || !strings.HasPrefix(a, "-") || a == "-" {
			operands = append(operands, a)
			i++
			continue
		}
		if a == "--" { // -- 之后全是操作数
			afterSep = true
			i++
			continue
		}
		if ok, kind := flagLookup(a, pol); ok {
			i++
			next, has := "", i < len(argv)
			if has {
				next = argv[i]
			}
			if valueFlagEats(kind, a, next, has) {
				i++ // 值参数
			}
			continue
		}
		return operands, fmt.Sprintf("%s 的 flag %q 不在安全表", name, a)
	}
	return operands, ""
}

// flagLookup 单 flag 解析：精确命中；`--flag=value` 形态（optional/string 型，
// 值内联不吃下一参）；联合短 flag（-rn）：全部组成字符必须各自是 FlagBool 且
// 不在 UnsafeFlags——值型 flag 折进联合 flag 后其参数会伪装成操作数（GNU getopt
// 下 sort -ro f = -r -o f 是写路径），这是曾经的真实漏判面。
func flagLookup(a string, pol Policy) (bool, FlagKind) {
	if kind, ok := pol.SafeFlags[a]; ok {
		return true, kind
	}
	// --flag=value：值内联。两级查表：先查 base 本键（optional/string 型放行）；
	// miss 或 bool 型再查 base+"=" 显式内联键（表里 "--format=" 形态的键——
	// 登记 bool 的 flag（git log --format/%H 这类）经此路径支持内联值）
	if base, val, found := strings.Cut(a, "="); found && strings.HasPrefix(base, "--") {
		if kind, ok := pol.SafeFlags[base]; ok && (kind == FlagOptional || kind == FlagString) {
			_ = val
			return true, FlagBool // 值已内联，不吃下一参
		}
		if _, ok := pol.SafeFlags[base+"="]; ok {
			return true, FlagBool
		}
		return false, ""
	}
	// 联合短 flag：全组成 bool 才放行
	if !strings.HasPrefix(a, "--") && len(a) > 2 && strings.HasPrefix(a, "-") {
		for _, u := range pol.UnsafeFlags {
			if u != "" && strings.HasPrefix(a, u) {
				return false, ""
			}
		}
		allBool := true
		for _, ch := range a[1:] {
			kind, ok := pol.SafeFlags["-"+string(ch)]
			if !ok || kind != FlagBool {
				allBool = false
				break
			}
		}
		if allBool {
			return true, FlagBool
		}
	}
	return false, ""
}

// splitSubcommand 在子命令策略表中定位子命令：按本层 flag 白名单消费（值型
// 吃参）直到首个非 flag token。返回子命令名与其后整段参数；无子命令返回空名。
func splitSubcommand(argv []string, pol Policy) (sub string, rest []string, reason string) {
	i := 0
	for i < len(argv) {
		a := argv[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			return a, argv[i+1:], ""
		}
		if a == "--" {
			if i+1 < len(argv) {
				return argv[i+1], argv[i+2:], ""
			}
			return "", nil, ""
		}
		if ok, kind := flagLookup(a, pol); ok {
			i++
			next, has := "", i < len(argv)
			if has {
				next = argv[i]
			}
			if valueFlagEats(kind, a, next, has) {
				i++
			}
			continue
		}
		return "", nil, fmt.Sprintf("flag %q 不在本层安全表", a)
	}
	return "", nil, ""
}

// literalWord 字面化一个词：全字面返回 (text,false)；含展开返回 (拼到一半的文本,true)。
func literalWord(w *mvdan.Word) (string, bool) {
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *mvdan.Lit:
			b.WriteString(p.Value)
		case *mvdan.SglQuoted:
			b.WriteString(p.Value)
		case *mvdan.DblQuoted:
			for _, ip := range p.Parts {
				if lit, ok := ip.(*mvdan.Lit); ok {
					b.WriteString(lit.Value)
				} else {
					b.WriteString("<dynamic>")
					return b.String(), true
				}
			}
		default:
			// ParamExp / CmdSubst / ArithmExp 等：动态
			b.WriteString("<dynamic>")
			return b.String(), true
		}
	}
	return b.String(), false
}

func invocationsAdd(invocations *[]Invocation, inv Invocation) {
	*invocations = append(*invocations, inv)
}

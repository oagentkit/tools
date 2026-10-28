package bashguard

import (
	"fmt"
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

// destructiveScan 扫描调用序列里的不可逆动作。
//
// 与只读判定正交：命令被只读门拒了（rm 不在只读表），它仍可能是不可逆的——
// 而「这条命令不可逆」正是调用方要据此要二次确认的那个事实。
func (t *Table) destructiveScan(invs []Invocation) (bool, string) {
	for _, inv := range invs {
		if inv.Name == "" {
			continue
		}
		if ok, why := t.destructiveVerdict(inv.Name, inv.Args); ok {
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
		rest := stripWrapper(argv, pol)
		if len(rest) == 0 {
			return false, ""
		}
		return t.destructiveVerdict(rest[0], rest[1:])
	}
	rule, ok := t.destructiveRule(name)
	if !ok {
		return false, ""
	}
	return destructiveByRule(name, argv, rule)
}

// destructiveRule 查不可逆规则：先精确命中，再前缀族（"mkfs*" 覆盖 mkfs.ext4 等）。
//
// 前缀族命中多个时取**最长前缀**（确定性：map 遍历序随机，不能让判定随遍历序变）。
func (t *Table) destructiveRule(name string) (Policy, bool) {
	if rule, ok := t.Destructive[name]; ok {
		return rule, true
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
		for _, tok := range argv {
			if strings.HasPrefix(tok, "-") {
				continue
			}
			if subRule, ok := rule.Subcommands[tok]; ok {
				return destructiveByRule(name+" "+tok, nil, subRule)
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

// sed 只读脚本判定（tools#81）：sed 是含 I/O 原语的 mini-language——w file /
// W file 写文件、GNU 的 e 与 s///e 执行 shell 命令、r/R 注入外部文件内容，
// 而脚本串以裸操作数 / -e / --expression / -f 多路到达。flag 白名单 +
// NoBareOperands 的表驱动守卫只看得到「有没有操作数」，看不到「脚本内容」，
// `sed -n 'w /tmp/x' /etc/hosts` 曾整体放行（只读档内可写任意文件、可执行命令）。
//
// 处置不是黑名单（w/W/e/r/R 加行地址加 -e/-f 的组合面太大，漏一则全漏），
// 而是**受限文法**：脚本必须整体匹配
//
//	script   = { sep | 注释 | command }
//	command  = [addr ["," addr]] ["!"] cmd
//	addr     = 数字 | "$" | "/" regexp "/"
//	cmd      = "s" d re d repl d [sflags]     （d = 任意分隔符字符）
//	         | "y" d set1 d set2 d
//	         | "p" | "d"
//	sflags   = { "g" | "p" | "i" | "I" | "m" | "M" | 数字 }
//
// 文法外任何形态一律判非只读（保守 fail-closed，宁可误报），显式包括：
//   - 写/执行原语：w / W / e / r / R 命令字母、s 的 w / e flag；
//   - 常见但不在文法的命令字母（{} 块、b/t 分支与标签、a/i/c、q、n/N、
//     h/H/g/G/x、=/l 等）；
//   - 扩展地址（~ 步进、+N 偏移、\I/\M 大小写修饰）、未闭合的 s///y///、
//     地址悬空（后无命令）；
//   - -f / --file：脚本内容来自文件，静态不可证 → 拒。
//
// 语义代价是误报（如 `sed -n '/x/{p}'`、`sed '2q'` 被拒）：只读门禁的误报
// 代价是多一次确认/人工过目，漏报的代价是只读档内真写文件、真执行命令。
package bashguard

import (
	"fmt"
	"strings"
)

// sedArgsCheck sed 的 Policy.CheckArgs 钩子（tools#81）：在 flag 白名单消费
// 之后对**完整 argv** 做脚本定位与受限文法校验。
//
// 脚本定位按 sed 语义：-e / --expression[=] 的值是脚本；无 -e/-f 在前时首个
// 裸操作数是脚本、其余裸操作数是输入文件（文件名不按脚本校验——路径里出现
// "w" 字母不是 sed 命令）。-f / --file[=] 的脚本内容静态不可证，直接拒。
func sedArgsCheck(name string, argv []string) string {
	scriptSeen := false
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			// -- 之后全是操作数：首个（若脚本尚未定位）为脚本，其余为输入文件
			for _, op := range argv[i+1:] {
				if !scriptSeen {
					if reason := sedScriptReadOnly(op); reason != "" {
						return reason
					}
					scriptSeen = true
				}
			}
			return ""
		case a == "-e" || a == "--expression":
			if i+1 >= len(argv) {
				// flag 白名单消费已保证值形态，这里是防御（钩子独立于表解析）
				return fmt.Sprintf("%s 的 %s 缺表达式参数", name, a)
			}
			if reason := sedScriptReadOnly(argv[i+1]); reason != "" {
				return reason
			}
			scriptSeen = true
			i++
		case strings.HasPrefix(a, "--expression="):
			if reason := sedScriptReadOnly(strings.TrimPrefix(a, "--expression=")); reason != "" {
				return reason
			}
			scriptSeen = true
		case a == "-f" || a == "--file" || strings.HasPrefix(a, "--file="):
			return fmt.Sprintf("%s 的 %s 脚本内容来自文件，无法静态判定只读", name, a)
		default:
			// 其余为白名单内的 bool flag（已过表校验）或裸操作数
			if !strings.HasPrefix(a, "-") || a == "-" {
				if !scriptSeen {
					if reason := sedScriptReadOnly(a); reason != "" {
						return reason
					}
					scriptSeen = true
				}
				// 其余裸操作数 = 输入文件（读侧），不校验
			}
		}
	}
	return ""
}

// sedScriptReadOnly 单段 sed 脚本（一个 -e 表达式或首个裸操作数）的受限文法
// 判定：返回空串=只读；非空=非只读原因。只认 s[///]/y/// /p /d 与数字、$、
// /re/、逗号范围、! 否定；其余一切判非只读。
func sedScriptReadOnly(script string) string {
	i, n := 0, len(script)
	skipBlank := func() { // 空白与分隔（POSIX 允许地址/!/命令间夹 blank）
		for i < n {
			switch script[i] {
			case ' ', '\t', '\r', '\n', ';':
				i++
				continue
			}
			break
		}
	}
	skipBang := func() { // 否定 !（可重复——重复取反语义不变，无安全差别）与空白交错
		for i < n {
			if script[i] == ' ' || script[i] == '\t' || script[i] == '!' {
				i++
				continue
			}
			break
		}
	}
	for {
		skipBlank()
		if i >= n {
			return "" // 空脚本 / 尾部分隔：无命令=无写路径
		}
		if script[i] == '#' { // 注释：吃到行尾（# 作 s/y 分隔符的形态在命令体内扫描，不经过这里）
			for i < n && script[i] != '\n' {
				i++
			}
			continue
		}
		// 地址（可有可无）：数字 | $ | /re/，加逗号范围
		next, _, reason := sedAddr(script, i)
		if reason != "" {
			return reason
		}
		i = next
		skipBang()
		if i < n && script[i] == ',' { // 范围第二地址
			i++
			for i < n && (script[i] == ' ' || script[i] == '\t') {
				i++
			}
			next2, matched, reason := sedAddr(script, i)
			if reason != "" {
				return reason
			}
			if !matched {
				return fmt.Sprintf("sed 范围地址缺第二地址（%q）", script)
			}
			i = next2
			skipBang()
		}
		if i >= n {
			return fmt.Sprintf("sed 地址后缺命令（未闭合 %q）", script)
		}
		switch script[i] {
		case 's':
			next, reason := sedSCommand(script, i+1)
			if reason != "" {
				return reason
			}
			i = next
		case 'y':
			next, reason := sedYCommand(script, i+1)
			if reason != "" {
				return reason
			}
			i = next
		case 'p', 'd':
			i++ // 单字母命令；紧跟的下一命令（pd 形态）由循环顶处理
		default:
			return sedCmdRejectReason(script[i], script)
		}
	}
}

// sedAddr 解析一个地址：数字 | $ | /regexp/。
// 返回 (扫描终点, 是否解析到地址, 非只读原因)。没解析到地址（命令字母起头）
// 不是错误——地址可有可无。~ 步进 / +N 偏移 / \I 大小写修饰等扩展地址不在
// 受限文法，由调用方按「地址后非命令字符」或文法外命令拒。
func sedAddr(script string, i int) (next int, matched bool, reason string) {
	n := len(script)
	if i >= n {
		return i, false, ""
	}
	switch c := script[i]; {
	case c >= '0' && c <= '9':
		for i < n && script[i] >= '0' && script[i] <= '9' {
			i++
		}
		return i, true, ""
	case c == '$':
		return i + 1, true, ""
	case c == '/':
		i++
		for i < n {
			switch script[i] {
			case '\\':
				i += 2 // 转义（含 \/）：越过（越过 n 时下方 for 条件自然收口）
				continue
			case '/':
				return i + 1, true, ""
			case '\n':
				return i, true, fmt.Sprintf("sed 地址 /regexp/ 未闭合（裸换行 %q）", script)
			}
			i++
		}
		return i, true, fmt.Sprintf("sed 地址 /regexp/ 未闭合（%q）", script)
	}
	return i, false, ""
}

// sedSCommand s 命令体：d re d repl d [sflags]（d=分隔符）。i 指向 's' 之后。
// flags 白名单 [g p i I m M 0-9]；w（s///w file 写文件）与 e（s///e 执行命令）
// 显式点名拒绝，其余未知 flag 也拒。
func sedSCommand(script string, i int) (next int, reason string) {
	return sedDelimBody(script, i, "s", true)
}

// sedYCommand y 命令体：d set1 d set2 d（d=分隔符），无 flags。
func sedYCommand(script string, i int) (next int, reason string) {
	return sedDelimBody(script, i, "y", false)
}

// sedDelimBody s/y 共用的三段定界体扫描。
func sedDelimBody(script string, i int, cmd string, sFlags bool) (next int, reason string) {
	n := len(script)
	if i >= n {
		return i, fmt.Sprintf("sed %s 命令缺分隔符（未闭合 %q）", cmd, script)
	}
	if script[i] == '\\' {
		// \<char> 定界符（生僻形态）：不支持——保守拒
		return i, fmt.Sprintf("sed %s 命令的反斜杠定界符不在受限文法（%q）", cmd, script)
	}
	delim := script[i]
	for pass := 0; pass < 2; pass++ { // re/set1、repl/set2 两段，各以未转义 delim 收口
		i++
		closed := false
		for i < n {
			c := script[i]
			if c == '\\' { // 转义（含分隔符本身）：越过；越过 n 时外层 for 条件自然收口
				i += 2
				continue
			}
			if c == delim {
				closed = true
				break
			}
			if c == '\n' {
				return i, fmt.Sprintf("sed %s 命令未闭合（裸换行 %q）", cmd, script)
			}
			i++
		}
		if !closed {
			return i, fmt.Sprintf("sed %s 命令未闭合（%q）", cmd, script)
		}
	}
	i++ // 收口第三段定界符
	if !sFlags {
		return i, ""
	}
	for i < n {
		switch c := script[i]; {
		case c == 'w':
			return i, fmt.Sprintf("sed 的 s///w 写文件（%q）", script)
		case c == 'e':
			return i, fmt.Sprintf("sed 的 s///e 执行 shell 命令（%q）", script)
		case c == 'g' || c == 'p' || c == 'i' || c == 'I' || c == 'm' || c == 'M' || c >= '0' && c <= '9':
			i++
		default:
			return i, "" // 命令结束（分隔符/下一命令/EOF）；该字符交主循环处置
		}
	}
	return i, ""
}

// sedCmdRejectReason 文法外命令字母的拒因：写/执行原语点名，其余给受限文法
// 白名单说明（审计可读）。
func sedCmdRejectReason(c byte, script string) string {
	switch c {
	case 'w':
		return fmt.Sprintf("sed 命令 'w' 写文件（%q）", script)
	case 'W':
		return fmt.Sprintf("sed 命令 'W' 写文件（首行）（%q）", script)
	case 'e':
		return fmt.Sprintf("sed 命令 'e' 执行 shell 命令（%q）", script)
	case 'r':
		return fmt.Sprintf("sed 命令 'r' 注入外部文件内容（%q）", script)
	case 'R':
		return fmt.Sprintf("sed 命令 'R' 注入外部文件内容（%q）", script)
	}
	return fmt.Sprintf("sed 命令 %q 不在受限只读文法（仅 s///、y///、p、d）", string(rune(c)))
}

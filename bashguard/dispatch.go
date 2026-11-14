// 统一派发器解包（tools#183）：bashguard 的 AST 判定面（Analyze）对
// `bash --rcfile x -c 'cmd'` / `env bash -c 'cmd'` / `sudo bash -c 'cmd'`
// 这类外壳形态本就有内核兜底（InnerFlagValue 定位 -c 载荷、kernelScan 对未
// 注册调度器全 token 扫描），但 guardx/permgate 的**段级**消费（allow/deny
// 名单按段匹配）此前各自维护一份词法解包（guardx#181 的 splitCommand），
// 覆盖面收窄在 `eval`、`[path/]sh|bash|…` + 连续选项簇两族——选项间夹裸
// operand（`bash --rcfile x -c`）与前缀派发（`env`/`sudo`/`nohup`/`setsid`
// + 内核 shell）不解包，外壳包一层后内层命令在段面上不可见，写档判定随之
// 漂白。本文件把解包语义**单源化**在这里导出：段级消费方（guardx/permgate）
// 统一改用本函数，不再各自维护词法实现。
package bashguard

import "strings"

// MaxDispatchDepth 派发器递归解包深度上限（tools#183）：防 `eval eval eval …`
// 病态嵌套的递归失控。**3 层**沿用 guardx#181 既有上限——远超真实命令形态。
// 到限后剩余段保持原样返回、不再下钻：deny/ask 侧整段命中语义不依赖解包，
// allow 侧恒失配兜底——两个方向都停在保守侧。
const MaxDispatchDepth = 3

// UnwrapDispatch 统一派发器解包（tools#183）：把命令按 shell 复合操作符拆段，
// 并对每段递归解包派发器，返回**全部段**——原段原样保留，派发器脚本体剥对称
// 引号后作为独立段并入。
//
// # 覆盖形态
//
//   - eval 派发器：`eval 'cmd …'`——eval 的操作数整体是运行期真实执行的命令
//     行（POSIX 把全部参数空格拼接后求值）；
//   - shell 派发器：`[path/]sh|bash|zsh|dash|ksh` + 选项簇 + 含 c 的选项
//     （`-c`/`-lc`/`-ec` 等）——**选项间夹裸 operand 不再卡壳**（tools#183
//     主修形态：`bash --rcfile x -c 'cmd'` 的 rcfile 路径是 bash 的 operand，
//     选项簇照常扫到含 c 的旗标、其后 token 即脚本体）；
//   - 前缀派发器链：`env` / `sudo` / `nohup` / `setsid`（tools#183：env
//     FOO=1 bash -c …、sudo -u nobody bash -c …）——前缀自身不执行内核、只
//     修饰执行环境，剥到内核派发器再解；
//   - 前导环境赋值：`FOO=1 bash -c 'cmd'`。
//
// # 返回形态与消费纪律
//
// 复合操作符（`;` `&`（含 && 的两个 & 各自触发）`|` `||` 换行）拆段：引号内
// （'…' / "…"，反斜杠转义）不拆、引号外未转义 \x 按字面保留。段集消费的两个
// 方向都取保守侧：deny/ask「任一段命中即拦/即问」（多段只多拦）；allow「每段
// 命中才放行」（多段只更严）。解包只可能**多并段**（原段恒保留），两个方向
// 都不会因解包放松。
//
// # 深度上限
//
// 递归解包上限 MaxDispatchDepth=3：`eval eval eval eval 'x'` 在第 3 层解包后
// 剩余段保持原样（返回值含各层原段与解包层，最内层不再下钻）。
//
// # 词法边界
//
// 本函数是**词法面**单源，不做完整 shell 解析——命令替换 $(…)、未引号 heredoc
// 体、$'…' 等 AST 形态归 Analyze 的判定面（其只读/不可逆两档对派发器载荷另有
// 内核兜底）。引号成组的 token 不会被误认派发器头；解不开的形态一律原样保留
// （与 guardx#181「拆不动保持原样」同款边界）。
func UnwrapDispatch(cmd string) []string {
	return unwrapDispatchDepth(cmd, 0)
}

// unwrapDispatchDepth 拆段 + 逐段解包（depth 计解包层数：0=顶层，达到
// MaxDispatchDepth 后不再下钻）。
func unwrapDispatchDepth(cmd string, depth int) []string {
	segs := splitDispatchSegments(cmd)
	if depth >= MaxDispatchDepth {
		return segs
	}
	out := make([]string, 0, len(segs)+2)
	for _, seg := range segs {
		out = append(out, seg)
		if inner, ok := unwrapDispatchOne(seg); ok {
			out = append(out, unwrapDispatchDepth(inner, depth+1)...)
		}
	}
	return out
}

// splitDispatchSegments 复合操作符拆段（词法）：`;` `&`（含 `&&`——POSIX 的
// 后台控制操作符，不带引号时真实 shell 在此分命令，两个 & 各自触发一次
// flush、空段由 flush 丢弃）`|`（含 `||`）与换行；单/双引号内不拆（双引号
// 内反斜杠转义下一字符）；引号外 \x 转义不拆段。段首尾空白剥除、空段丢弃；
// 无操作符时返回单段。
func splitDispatchSegments(cmd string) []string {
	var (
		segs     []string
		cur      strings.Builder
		inSingle bool
		inDouble bool
	)
	flush := func() {
		if t := strings.TrimSpace(cur.String()); t != "" {
			segs = append(segs, t)
		}
		cur.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			cur.WriteByte(c)
		case inDouble:
			if c == '\\' && i+1 < len(cmd) {
				cur.WriteByte(c)
				i++
				cur.WriteByte(cmd[i])
			} else {
				if c == '"' {
					inDouble = false
				}
				cur.WriteByte(c)
			}
		default:
			switch c {
			case '\\':
				cur.WriteByte(c)
				if i+1 < len(cmd) {
					i++
					cur.WriteByte(cmd[i])
				}
			case '\'':
				inSingle = true
				cur.WriteByte(c)
			case '"':
				inDouble = true
				cur.WriteByte(c)
			case ';', '\n', '|', '&':
				flush()
			default:
				cur.WriteByte(c)
			}
		}
	}
	flush()
	return segs
}

// segTok 段内词元的原文区间：含引号定界符——脚本体提取需要原文保真（剥的
// 只是对称引号，段内引号结构不得被词法化抹掉）。
type segTok struct{ start, end int }

// lexSegTokens 按空白切词元：引号成组（含定界符）、反斜杠转义不破词。
func lexSegTokens(s string) []segTok {
	var toks []segTok
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			switch s[i] {
			case '\\':
				i++ // 转义下一字符（词尾悬挂 \ 留给解析失败路径）
				if i < len(s) {
					i++
				}
			case '\'':
				i++
				for i < len(s) && s[i] != '\'' {
					i++
				}
				if i < len(s) {
					i++ // 收引号
				}
			case '"':
				i++
				for i < len(s) {
					if s[i] == '\\' && i+1 < len(s) {
						i += 2
						continue
					}
					if s[i] == '"' {
						i++
						break
					}
					i++
				}
			default:
				i++
			}
		}
		toks = append(toks, segTok{start, i})
	}
	return toks
}

// dispatchPrefixes 前缀派发器档案（tools#183）：env/sudo/nohup/setsid 自身
// 不执行内核、只修饰执行环境，剥壳只为**定位内核位置**——与只读表的 Wrapper
// 剥壳（stripWrapper，判定语义）无关。values 是吃一个独立参数的旗（内联
// `--flag=v` 形态天然自包含）；未登记旗的值归属不猜（tools#100 教训的词法面
// 版）：略过旗 token 继续扫，内核候选必须命中派发器形态才解包——定位歪了
// 只会"不解包"（原样保留，保守侧），不会错位解包。
var dispatchPrefixes = map[string]prefixProfile{
	// env [-i] [-0] [-u NAME] [-S CMD] [KEY=VAL…] CMD
	"env": {bools: setOf("-i", "-0"), values: setOf("-u", "-S", "--split-string")},
	// sudo [-n -E -b -H -K -k -l -v -P -S -A -i -s] [-u USER] [-g GROUP]
	// [-p PROMPT] [-C fd] … CMD
	"sudo": {bools: setOf("-n", "-E", "-b", "-H", "-K", "-k", "-l", "-v", "-P", "-S", "-A", "-i", "-s"),
		values: setOf("-C", "-D", "-g", "-p", "-r", "-R", "-t", "-T", "-U", "-u")},
	// nohup [--append] CMD
	"nohup": {bools: setOf("--append")},
	// setsid [-c -e -f -s -w] CMD
	"setsid": {bools: setOf("-c", "-e", "-f", "-s", "-w", "--ctty", "--exec", "--fork", "--session", "--wait")},
}

// prefixProfile 前缀派发器的参数档案（见 dispatchPrefixes）。
type prefixProfile struct {
	bools  map[string]bool
	values map[string]bool
}

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// shellDispatchNames shell 派发器族（tools#183 与 guardx#181 同名单）：内核
// 是 `[path/]sh|bash|zsh|dash|ksh` 时按含 c 的选项簇定位脚本体。
var shellDispatchNames = setOf("sh", "bash", "zsh", "dash", "ksh")

// unwrapDispatchOne 单段解包：前导赋值与前缀链剥到内核，内核是 eval 或 shell
// 派发器时返回其脚本体（对称引号已剥）。非派发器形态返回 false——段原样保留。
func unwrapDispatchOne(seg string) (string, bool) {
	toks := lexSegTokens(seg)
	if len(toks) == 0 {
		return "", false
	}
	tokText := func(i int) string { return seg[toks[i].start:toks[i].end] }
	// 前导 KEY=VAL 环境赋值（FOO=1 bash -c 'cmd'）：跳过。非赋值形态的含 =
	// token 是普通命令名（不可执行形态），停在这里当内核候选。
	i := 0
	for i < len(toks) {
		t := tokText(i)
		if strings.HasPrefix(t, "-") || !strings.Contains(t, "=") || !isEnvAssign(t) {
			break
		}
		i++
	}
	if i >= len(toks) {
		return "", false
	}
	// 前缀派发器链：env/sudo/nohup/setsid 可叠（sudo env bash -c …），逐层
	// 剥到内核；链断（值旗悬空/内核不是派发器）即整段不解。
	for {
		if prof, isPrefix := dispatchPrefixes[dispatchName(tokText(i))]; isPrefix {
			next, ok := skipPrefixArgs(seg, toks, i+1, prof)
			if !ok {
				return "", false
			}
			i = next
			continue
		}
		break
	}
	switch name := dispatchName(tokText(i)); {
	case name == "eval":
		if i+1 >= len(toks) {
			return "", false // eval 无操作数
		}
		inner := strings.TrimSpace(seg[toks[i].end:])
		if inner == "" {
			return "", false
		}
		return trimSymmetricQuotes(inner), true
	case shellDispatchNames[name]:
		return unwrapShellC(seg, toks, i+1)
	}
	return "", false
}

// skipPrefixArgs 从 j 起跳过前缀派发器自身的参数面（旗 / KEY=VAL 赋值），
// 返回内核候选下标。值旗悬空或纯前缀无内核（env 裸跑=打印环境）返回 false。
func skipPrefixArgs(seg string, toks []segTok, j int, prof prefixProfile) (int, bool) {
	for j < len(toks) {
		t := seg[toks[j].start:toks[j].end]
		switch {
		case t == "--": // 选项终止符：其后首个 token 即内核候选
			j++
			if j >= len(toks) {
				return 0, false
			}
			return j, true
		case strings.HasPrefix(t, "-") && len(t) > 1:
			if prof.bools[t] {
				j++
				continue
			}
			if prof.values[t] {
				if j+1 >= len(toks) {
					return 0, false // 值旗悬空：内核定位不了
				}
				j += 2
				continue
			}
			j++ // 未登记旗：值归属不猜，略过旗本身继续扫
			continue
		case strings.Contains(t, "=") && isEnvAssign(t):
			j++ // 前缀自带的环境赋值（env FOO=1 bash …）
			continue
		default:
			return j, true
		}
	}
	return 0, false
}

// unwrapShellC shell 派发器的脚本体定位（tools#183 主修形态）：从 from 起
// 扫描选项簇——旗 token 与**夹在中间的裸 operand**（bash --rcfile x 的 x）
// 逐个略过，命中含 c 的选项簇时其后一个 token 即脚本体；`--` 终止选项解析，
// 其后全是 operand（bash -- -c 的 -c 是脚本文件名不是派发旗）。无含 c 旗标
// （bash --rcfile x、bash script.sh）不是 -c 派发，不解包。
func unwrapShellC(seg string, toks []segTok, from int) (string, bool) {
	for i := from; i < len(toks); i++ {
		t := seg[toks[i].start:toks[i].end]
		if t == "--" {
			return "", false
		}
		if !strings.HasPrefix(t, "-") || len(t) == 1 || !cFlagCluster(t) {
			continue
		}
		if i+1 >= len(toks) {
			return "", false // -c 悬空：残缺形态不解
		}
		return trimSymmetricQuotes(seg[toks[i+1].start:toks[i+1].end]), true
	}
	return "", false
}

// cFlagCluster 选项簇是否为含 c 的派发旗（-c / -lc / -ec 形态）。口径沿用
// guardx#181 的既有判定并放行大小写：短横线打头、全字母、**c 的前后各 ≤3 个
// 字母**——`--rcfile`/`--config` 这类恰含 c 的长选项不得被当成派发旗（否则
// -c 的真实脚本体被错当 operand 漏解）。
func cFlagCluster(t string) bool {
	var body string
	switch {
	case strings.HasPrefix(t, "--"):
		body = strings.ToLower(strings.TrimPrefix(t, "--"))
	case strings.HasPrefix(t, "-"):
		body = strings.ToLower(strings.TrimPrefix(t, "-"))
	default:
		return false
	}
	if len(body) == 0 {
		return false
	}
	for i := 0; i < len(body); i++ {
		if body[i] < 'a' || body[i] > 'z' {
			return false
		}
	}
	// 任一 c 满足前 ≤3 后 ≤3 即派发旗（-c/-lc/-ec）；--rcfile 的 c 后跟 4 个
	// 字母（file），超界不命中
	for i := 0; i < len(body); i++ {
		if body[i] == 'c' && i <= 3 && len(body)-1-i <= 3 {
			return true
		}
	}
	return false
}

// dispatchName 派发器头的归一：剥对称引号（'bash' 运行期仍是 bash）、小写
// （guardx#181 的 (?i) 同款保守——大写形态多解只会多并段）、取 basename
// （/bin/bash、./bash 归一）。
func dispatchName(tok string) string {
	tok = strings.ToLower(trimSymmetricQuotes(tok))
	if k := strings.LastIndexByte(tok, '/'); k >= 0 {
		tok = tok[k+1:]
	}
	return tok
}

// trimSymmetricQuotes 剥掉首尾成对的引号（派发器脚本体的 '…' / "…"）。
func trimSymmetricQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

// isEnvAssign KEY=VAL 环境赋值形态判定：KEY 须是合法环境名（字母/下划线
// 开头，组成 [A-Za-z0-9_]）。
func isEnvAssign(t string) bool {
	k, _, ok := strings.Cut(t, "=")
	if !ok || k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

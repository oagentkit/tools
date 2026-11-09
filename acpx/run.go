package acpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/oagentkit/runtime/procx"
	"github.com/oagentkit/textx/textutil"
	"github.com/oekit/pkg/jsonrepair"
)

// runCLI 适配器统一执行骨架：validate → 进程组执行（procx：环境白名单 +
// timeout + stdout 限容/按行拆分单源）。各适配器只负责 argv 构造与事件流解析。
// stderr 一并透传（RunResult.Stderr 的排障通道——此前整条丢弃）。
func runCLI(ctx context.Context, req RunRequest, argv []string, onLine func(string)) (stdout, stderr string, code int, err error) {
	if err := req.validate(); err != nil {
		return "", "", -1, err
	}
	stdout, stderr, code, err = procx.Run(ctx, procx.RunRequest{
		Argv: argv, Dir: req.WorkDir, Env: req.Env, Timeout: req.Timeout, OnLine: onLine,
	})
	return stdout, stderr, code, err
}

// stderrTail 子进程 stderr 尾巴（≤400 rune，rune 对齐）。错误路径 procx 已把
// 尾巴嵌进 error 文本，这里服务结果面（RunResult.Stderr）的 transcript 排障。
func stderrTail(stderr string) string {
	if stderr == "" {
		return ""
	}
	return textutil.TruncEllipsis(strings.TrimRight(stderr, "\n"), 400)
}

// isInterruptedExec 中断类执行错判定（tools#46）：超时（procx.ErrTimeout）、
// 调用方取消（procx.ErrCanceled）或子进程被信号杀死（外部 kill/OOM：
// exec.ExitError 且 wait status 为信号终止）。这类错意味着「没跑完」——与
// 「跑完了但退出码非零」（终态优先路径按协议终态处置）是两类语义：前者拿到的
// 任何文本都只是部分输出，不得代理终态。
func isInterruptedExec(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, procx.ErrTimeout) || errors.Is(err, procx.ErrCanceled) {
		return true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			return ws.Signaled()
		}
	}
	return false
}

// fallbackResult 全文兜底结果：JSON/事件流不可用时把 stdout 整体当文本。
func fallbackResult(stdout, stderr string, code int) *RunResult {
	return &RunResult{Text: strings.TrimSpace(stdout), ExitCode: code, Raw: []byte(stdout), Stderr: stderrTail(stderr)}
}

// stdoutTruncated 判定 stdout 是否带 procx 截断留痕（tools#94）：noteTruncated
// 在 captured stdout 尾部追加「\n…（stdout 超过 N 字节上限，已截断）」。双条件
// 从紧判定（截断短语 + 尾缀），代理 CLI 自身输出出现完整标记形态的概率远低
// 于误报面。
func stdoutTruncated(stdout string) bool {
	return strings.Contains(stdout, "（stdout 超过") && strings.HasSuffix(strings.TrimRight(stdout, " \t\r\n"), "已截断）")
}

// lostTerminalErr 终态丢失守门（tools#83）：stdout 里**存在**协议终态事件
// 形状的行、却没被 OnLine 流识别成终态（hasTerm=false）时如实报错，拒绝把
// 整条事件流原文当「最终回答」成功返回。
//
// 为什么会有这种组合：procx 的 lineWriter 对单行超 maxLineLen（1MB）的行
// 整行丢弃且对调用方零信号（tools#83）——agent 任务产出大文件/长 diff 时
// 终态事件恰好是那一行的形态真实存在（claude 的 result、mimo 的
// step_finish…），OnLine 没见过 ⇒ hasTerm=false ⇒ 走全文兜底，把可达数 MB
// 的 NDJSON 事件流原文当 RunResult.Text 交给消费方。协议解析漂移（CLI 版本
// 升级改了终态形状）是同一组合的另一来源。
//
// 处置取「宁误报不静默给垃圾」：终态形状就在 stdout 里而我们没认出来，说明
// 流水线有未知故障——报错（错误信息点名两种可能成因）让调用方排障，好于把
// 原始事件流当成功答案注入下游 agent 循环。isTerminalLine 逐行做形状判定
// （各适配器提供自家终态形状的廉价判定），首条命中即返回。
func lostTerminalErr(who, stdout string, hasTerm bool, isTerminalLine func(string) bool) error {
	if hasTerm || isTerminalLine == nil {
		return nil
	}
	// 截断守门（tools#94）：procx cappedBuffer 只保 stdout 前 8MB，超出部分
	// 连同终端行一起被静默丢弃——「无终态 + 尾部截断留痕」= 终端行大概率已
	// 随截断被丢，在残缺 stdout 里找不到终态形状是必然而非异常。fail-closed：
	// 拒绝把残缺事件流当成功回答兜底。
	if stdoutTruncated(stdout) {
		return fmt.Errorf("acpx: %s stdout 触发截断上限且未见协议终态——终端行大概率已随截断被丢弃，拒绝把残缺事件流当最终回答兜底（tools#94）", who)
	}
	for len(stdout) > 0 {
		line := stdout
		if i := strings.IndexByte(stdout, '\n'); i >= 0 {
			line, stdout = stdout[:i], stdout[i+1:]
		} else {
			stdout = ""
		}
		if isTerminalLine(line) {
			return fmt.Errorf("acpx: %s 事件流含终态事件形状但未被识别为终态（可能单行超 1MB 上限被丢弃，或 CLI 版本协议漂移）——宁误报不静默，拒绝把事件流原文当最终回答兜底", who)
		}
	}
	return nil
}

// contentLineErr 正文行对账守门（tools#105）：codex/kimi 的正文来源行不是协议
// 终态（是数据行，每条都进 lastText）——「OnLine 识别的条数」与「stdout 里现存
// 的同形状条数」对不上 ⇒ 有行被 lineWriter 整行丢弃（>1MB，tools#83 机制），
// 手里的最后一条可能是陈旧断片；截断留痕在场（tools#94）时流尾被剪、对账本身
// 不可证。两条都 fail-closed：拒绝把部分文本当最终回答成功返回——吐过部分文本
// 的跑曾借「text 非空即返回」绕过终态/截断守门（#105 残留面，#83 只在空文本时
// 接管）。seen 须与 isContentLine 同判据计数（OnLine 实际识别的条数）。
func contentLineErr(who, stdout string, seen int, isContentLine func(string) bool) error {
	if stdoutTruncated(stdout) {
		return fmt.Errorf("acpx: %s stdout 触发截断上限——正文行可能已随截断丢失，拒绝把部分文本当最终回答兜底（tools#94/#105）", who)
	}
	n := 0
	for len(stdout) > 0 {
		line := stdout
		if i := strings.IndexByte(stdout, '\n'); i >= 0 {
			line, stdout = stdout[:i], stdout[i+1:]
		} else {
			stdout = ""
		}
		if isContentLine(line) {
			n++
		}
	}
	if n > seen {
		return fmt.Errorf("acpx: %s 事件流含 %d 条正文形状的行、仅识别 %d 条（单行超 1MB 上限被整行丢弃）——宁误报不静默，拒绝把陈旧断片当最终回答兜底（tools#105）", who, n, seen)
	}
	return nil
}

// parseJSONOut 容错解析 agent 的 JSON 输出：平衡括号感知地截取首个 JSON 对象
// （日志混入/尾随噪音均可容忍）后 Unmarshal。失败时调用方走 fallbackResult。
func parseJSONOut(stdout string, out any) error {
	candidate := jsonrepair.ExtractObject(stdout)
	if candidate == "" {
		candidate = stdout
	}
	return json.Unmarshal([]byte(candidate), out)
}

// streamError 事件流失败的统一收尾：exit=0 但流内含 error/abort 时如实报错——
// 部分失败形态（配额耗尽/模型不支持/中止）进程退出码仍为 0，不得因恰好收到
// 部分文本而当成功返回（2026-09 huginn 实弹教训）。detail 为空 = 无失败。
func streamError(who, kind, detail string) error {
	if detail == "" {
		return nil
	}
	return fmt.Errorf("acpx: %s %s: %s", who, kind, detail)
}

// envelopeOut 通用 JSON 信封输出（{"text","sessionID","tokens","error"}）。
type envelopeOut struct {
	Text      string `json:"text"`
	SessionID string `json:"sessionID"`
	// Error 信封错误（"error":null 或缺省 = 无失败）。字符串与 {"message":...}
	// 两种形态都收——各家 CLI 约定不一（opencode 实测形态待真机核实，宽容解析）。
	Error  json.RawMessage `json:"error"`
	Tokens *struct {
		Input  int `json:"input"`
		Output int `json:"output"`
	} `json:"tokens"`
}

// envelopeErrMsg 信封错误文案提取：null/空 = 无失败；字符串直取；
// 对象取 message；其余形态回退原文。
func envelopeErrMsg(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return strings.TrimSpace(str)
	}
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return strings.TrimSpace(obj.Message)
	}
	return s
}

// envelopeResult 信封结果装配：信封 error 如实上抛（部分错误形态 exit 仍为 0，
// 只能从信封识别——与 mimo/gemini 同纪律，曾把错误信封解析成功后以空 text
// 触发全文兜底当成功返回）；解析失败或 text 为空时全文兜底（版本差异/日志混入）。
func envelopeResult(req RunRequest, who, stdout, stderr string, code int) (*RunResult, error) {
	var out envelopeOut
	if err := parseJSONOut(stdout, &out); err != nil {
		return fallbackResult(stdout, stderr, code), nil
	}
	if msg := envelopeErrMsg(out.Error); msg != "" {
		emitError(req, msg, stdout)
		return nil, streamError(who, "运行错误", msg)
	}
	if out.Text == "" {
		return fallbackResult(stdout, stderr, code), nil
	}
	r := &RunResult{Text: out.Text, SessionID: out.SessionID, ExitCode: code, Raw: []byte(stdout), Stderr: stderrTail(stderr)}
	if out.Tokens != nil {
		r.Usage = Usage{InputTokens: out.Tokens.Input, OutputTokens: out.Tokens.Output}
	}
	return r, nil
}

// tokenPair 事件流 usage 的 input/output token 对（claude/codex 同一 JSON 形态；
// mimo/opencode 键名不同，各自内联）。
type tokenPair struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// emit 流式事件的 nil 安全发送（回调在 stdout 读取 goroutine 中同步执行，
// 不得阻塞/panic——见 RunRequest.OnEvent 契约）。emitText/emitError 是其
// 常用形态的便捷包装；thinking/tool_call 等事件直接构造 Event 后经 emit 发送。
func emit(req RunRequest, ev Event) {
	if req.OnEvent != nil {
		req.OnEvent(ev)
	}
}

// emitText 流式文本事件的 nil 安全发送。
func emitText(req RunRequest, text, line string) {
	emit(req, Event{Type: EventText, Text: text, Raw: json.RawMessage(line)})
}

// emitError 流式错误事件的 nil 安全发送。各适配器对运行期 error 事件
// 全量转发（transcript 可排障——此前只转 text，纯错误跑 transcript 0 字节）。
func emitError(req RunRequest, text, line string) {
	emit(req, Event{Type: EventError, Text: text, Raw: json.RawMessage(line)})
}

// finalizeStream 事件流适配器统一的三段式收尾骨架（claude/pi/mimo 等 JSONL
// 流共用——各家手写曾反复漂移：终态优先漏迁移、空文本终态被当空壳触发兜底）：
//  1. 执行错（超时/信号/非零退出）且终态已到达 → 终态优先：agent 正常完成但
//     进程退出码非零时以结果为准；终态到达即权威，文本可为空（空文本终态
//     ≠ 空壳，不得退回 stdout 全文兜底把事件流原文当正文）
//  2. exit=0 但流内 error/abort → 如实报错：部分失败不得因恰好收到部分文本
//     而当成功返回（2026-09 huginn 实弹教训）
//  3. 终态 → 返回；无终态 → stdout 全文兜底（版本差异/日志混入/流尾截断）
//
// term 由调用方在协议终态事件到达时装配；hasTerm=false 时 term 被忽略。
// streamErr 为流内失败（streamError 产物），nil = 无失败。
func finalizeStream(stdout, stderr string, code int, execErr error, term *RunResult, hasTerm bool, streamErr error) (*RunResult, error) {
	if execErr != nil {
		if hasTerm && term != nil {
			term.ExitCode = code
			term.Raw = []byte(stdout)
			term.Stderr = stderrTail(stderr)
			return term, nil
		}
		return nil, execErr
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if hasTerm && term != nil {
		term.ExitCode = code
		term.Raw = []byte(stdout)
		term.Stderr = stderrTail(stderr)
		return term, nil
	}
	// 会话 ID 与终态正交（来自 session/init 事件）：无终态兜底时仍保留，
	// 调用方拿它续聊不受输出格式异常影响
	r := fallbackResult(stdout, stderr, code)
	if term != nil {
		r.SessionID = term.SessionID
	}
	return r, nil
}

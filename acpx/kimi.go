package acpx

import (
	"context"
	"encoding/json"
	"strings"
)

// 编译期断言。
var _ Agent = (*Kimi)(nil)

// Kimi Kimi Code CLI 适配器（MoonshotAI/kimi-code，参数经本机真实 CLI --help 核实）。
//
// 协议要点（非交互模式，headless）：
//   - `kimi -p "<prompt>"` 非交互执行（本身即全自主；不可与 --auto 组合——真实 CLI 实测报错）
//   - `--output-format stream-json` 输出 JSONL 消息流：{"role":"assistant","content":"..."}
//   - `-m/--model NAME` 覆盖模型；`-S/--session ID` 续聊
//
// 注：文档站描述的旧版 kimi-cli 用 `--print`；新一代 kimi-code 已改为 `-p` 直跑。
type Kimi struct {
	Bin string // 默认 "kimi"
}

// NewKimi 创建 Kimi 适配器。
func NewKimi() *Kimi { return &Kimi{Bin: "kimi"} }

func (k *Kimi) Name() string { return "kimi" }

// Capabilities 支持 Model/Session（--model / --session）；沙箱/轮数/工具白名单无对应参数
// （非交互模式本身即全自主——传 Sandbox 会被拒绝而非裸跑）。
func (k *Kimi) Capabilities() Capability {
	return Capability{Model: true, Session: true}
}

// Run 执行 Kimi print 模式任务。
func (k *Kimi) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv := []string{k.Bin, "-p", promptArg(req.Prompt), "--output-format", "stream-json"}
	if req.Model != "" {
		argv = append(argv, "--model", req.Model)
	}
	if req.SessionID != "" {
		argv = append(argv, "--session", req.SessionID)
	}

	var lastAssistant string
	onLine := func(line string) {
		var msg kimiMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			return // 非消息行（日志/警告）忽略
		}
		if msg.Role == "assistant" && msg.Content != "" {
			lastAssistant = msg.Content
			emitText(req, msg.Content, line)
		}
	}

	stdout, stderr, code, err := runCLI(ctx, req, argv, onLine)
	// 中断类执行错（超时/取消/被信号杀死）不得因「收到过部分 assistant 消息」
	// 被吞（tools#46）：kimi 没有协议终态事件，「收到过 assistant 消息」只是
	// 弱代理——超时被杀时任务并未完成，半截文本曾被整体按成功返回（ExitCode
	// 是被杀的值而非 0、error 却为 nil，消费方无从察觉）。err 如实上抛，
	// 部分文本挂 RunResult（Interrupted=true）供排障。
	if err != nil && isInterruptedExec(err) {
		return &RunResult{
			Text: lastAssistant, ExitCode: code, Raw: []byte(stdout),
			Stderr: stderrTail(stderr), Interrupted: true,
		}, err
	}
	// 终态优先（与 claude/codex/pi/mimo 同契约）：已收到 assistant 消息
	//（agent 正常完成）但进程退出码非零时以结果为准，不丢文本——该路径的
	// err 是「跑完了但退出码非零」，不是中断
	if err != nil && lastAssistant == "" {
		return nil, err
	}
	if lastAssistant == "" {
		// 终态丢失守门（tools#83）：kimi 无协议终态事件，assistant 消息行
		// 就是唯一的正文来源——stdout 里**存在** assistant 消息形状的行却没
		// 进 lastAssistant（单行超 1MB 被丢弃是主形态：agent 任务产出大文件/
		// 长 diff 的最终消息即可触发；解析漂移亦然），说明 OnLine 流丢了行，
		// 拒绝把整条 JSONL 事件流原文当最终回答兜底
		if lost := lostTerminalErr("kimi", stdout, false, kimiTerminalLine); lost != nil {
			return nil, lost
		}
		// stream-json 不可用（旧版本/文本模式）：全文兜底
		lastAssistant = strings.TrimSpace(stdout)
	}
	return &RunResult{Text: lastAssistant, ExitCode: code, Raw: []byte(stdout), Stderr: stderrTail(stderr)}, nil
}

// kimiTerminalLine 正文形状判定（tools#83 守门用）：assistant 消息行（与
// OnLine 的取文判据同款：role=assistant 且 content 非空）。
func kimiTerminalLine(line string) bool {
	var msg kimiMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return false
	}
	return msg.Role == "assistant" && msg.Content != ""
}

// kimiMessage stream-json 消息行。
type kimiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

package acpx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// 编译期断言。
var _ Agent = (*Gemini)(nil)

// ---------- Gemini CLI / Qwen Code（同族协议） ----------

// Gemini Gemini CLI 适配器（google-gemini/gemini-cli；Qwen Code 为其 fork，参数同族）。
//
// 协议要点（非交互模式）：
//   - `gemini -p "<prompt>" --output-format json` 输出单 JSON 对象：
//     {"response":"...","stats":{"models":{"<model>":{"tokens":{"prompt":N,"candidates":N}}}}}
//   - `-m/--model`；`--approval-mode default|auto_edit|yolo`（沙箱映射）
type Gemini struct {
	Bin string // 默认 "gemini"
	namedAgent
}

// NewGemini 创建 Gemini CLI 适配器。
func NewGemini() *Gemini {
	return &Gemini{Bin: "gemini", namedAgent: namedAgent{name: "gemini", fallback: "gemini"}}
}

// NewQwen 创建 Qwen Code 适配器（gemini-cli fork，参数同族）。
func NewQwen() *Gemini {
	return &Gemini{Bin: "qwen", namedAgent: namedAgent{name: "qwen", fallback: "gemini"}}
}

// Capabilities 支持 Model/Sandbox（-m / --approval-mode）；非交互模式无续聊、轮数与工具白名单无对应参数。
func (g *Gemini) Capabilities() Capability {
	return Capability{Model: true, Sandbox: true}
}

// geminiOut --output-format json 结构（容错解析：只取需要的字段）。
//
// Error 用 json.RawMessage 单源收一切形态（tools#110）：gemini/qwen 的错误
// 信封有**字符串**（{"error":"quota exceeded"}）与**对象**（{"error":{"code":429,
// "status":"RESOURCE_EXHAUSTED"}}，未必带 message）两类真实形态——此前声明为
// *struct{Message string}，字符串形态连整份 JSON 解析都失败、对象无 message
// 过不了 message 判据，两条路都把错误原文当「成功」的最终回答返回。存在
// 即失败（envelopeHasError），文案经 envelopeErrMsg 三路收敛。
type geminiOut struct {
	Response string          `json:"response"`
	Error    json.RawMessage `json:"error"`
	Stats    *struct {
		Models map[string]struct {
			Tokens struct {
				Prompt     int `json:"prompt"`
				Candidates int `json:"candidates"`
			} `json:"tokens"`
		} `json:"models"`
	} `json:"stats"`
}

// geminiTruncatedErr 截断守门（tools#110，对齐 tools#94 的 fail-closed 纪律）：
// gemini 是单对象 JSON 形态，没有流式适配器那套终态守门，stdout 被 procx
// 截断（8MB 上限）⇒ JSON 必然残缺/留痕混入——截断残片连同尾注曾被全文兜底
// 当成功的最终回答返回（token 账单按 8MB 走、内容全是垃圾、提示混在答案里
// 而不是走 error 通道）。
func geminiTruncatedErr(name string) error {
	// name 用调用方 g.name（tools#137）：适配器被 qwen 复用，硬写 "gemini"
	// 让 qwen 的失败在日志/告警里归因到 gemini 名下。
	return fmt.Errorf("acpx: %s stdout 触发截断上限——单对象 JSON 已残缺，拒绝把截断残片当最终回答兜底（tools#94/#110）", name)
}

// Run 执行 Gemini CLI 非交互任务。
func (g *Gemini) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	argv := []string{g.exec(g.Bin), "-p", promptArg(req.Prompt), "--output-format", "json"}
	if req.Model != "" {
		argv = append(argv, "-m", req.Model)
	}
	if v, ok := mapSandbox(req.Sandbox, "default", "auto_edit", "yolo"); ok {
		argv = append(argv, "--approval-mode", v)
	}

	res, err := runCLI(ctx, req, argv, nil)
	if err != nil {
		return nil, err
	}
	stdout, stderr, code := res.Stdout, res.Stderr, res.ExitCode

	var out geminiOut
	if err := parseJSONOut(stdout, &out); err != nil {
		// 主解析失败 ≠ 立即兜底（tools#110，三路收口）：
		//   - 截断事实在场（StdoutTruncated 结构化字段，tools#178——不匹配
		//     procx 人读截断文案）⇒ 单对象 JSON 已残缺，fail-closed
		//     拒绝兜底——残片连同截断尾注曾被当成功答案全款走 token 账单；
		//   - error 信封在场（主解析的失败常是其它字段形态漂移）⇒ 如实报错；
		//   - 两者皆无才允许全文兜底（非 JSON 输出：版本差异/日志混入）。
		if res.StdoutTruncated {
			return nil, geminiTruncatedErr(g.name)
		}
		if msg := envelopeFallbackErrMsg(stdout); msg != "" {
			emitError(req, msg, stdout)
			return nil, streamError(g.name, "运行错误", msg)
		}
		return fallbackResult(stdout, stderr, code), nil
	}
	// 运行期错误如实上抛（部分错误形态 exit 仍为 0，只能从信封识别——与 mimo
	// 同纪律）。存在即失败（envelopeHasError），文案三路收敛：字符串直取 /
	// 对象.message / 其它形态原文——对象无 message 键的形态按原文报（tools#110）。
	if envelopeHasError(out.Error) {
		msg := envelopeErrMsg(out.Error)
		if msg == "" {
			msg = strings.TrimSpace(string(out.Error))
		}
		emitError(req, msg, stdout)
		return nil, streamError(g.name, "运行错误", msg)
	}
	if out.Response == "" {
		// 空响应兜底同样过截断守门（tools#110）：截断形态下「解析成功但响应
		// 为空」的残stdout 兜底会把截断留痕混进答案
		if res.StdoutTruncated {
			return nil, geminiTruncatedErr(g.name)
		}
		return fallbackResult(stdout, stderr, code), nil
	}
	r := &RunResult{Text: out.Response, ExitCode: code, Raw: []byte(stdout), Stderr: stderrTail(stderr)}
	if out.Stats != nil {
		for _, m := range out.Stats.Models {
			r.Usage.InputTokens += m.Tokens.Prompt
			r.Usage.OutputTokens += m.Tokens.Candidates
		}
	}
	return r, nil
}

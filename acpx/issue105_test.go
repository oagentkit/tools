package acpx

import (
	"context"
	"strings"
	"testing"
)

// 本文件钉 tools#105 的回归：tools#83/#94 的守门（lostTerminalErr 终态形状扫描
// + 截断留痕 fail-closed）只在 text == "" 时接管——mimo/codex/kimi 在吐过部分
// 文本（s.text/lastText/lastAssistant 非空）时直接返回该文本，不扫终态形状、
// 不查截断留痕。部分输出的跑（吐了若干文本后 agent 产出大文件 ⇒ 终态/
// assistant_message 行 >1MB 被 lineWriter 丢弃，或流总量超 8MB 触发截断）⇒
// 陈旧部分文本以 nil error 成功返回。修后：无终态路径一律过守门；codex/kimi
// 的正文行（数据行而非协议终态）按「识别条数 vs stdout 现存条数」对账。

// bigJSONLine 构造一条 >1MB 的合法单行 JSON 事件（value 由 'a' 填充）。
func bigJSONLine(prefix, suffix string) string {
	return "printf '" + prefix + "'; head -c 1200000 /dev/zero | tr '\\0' 'a'; printf '" + suffix + "'\n"
}

// runText 只取 Text 与 err（RunResult.Raw 达 MB 级，不整体打印）。
func runText(t *testing.T, a Agent) (string, error) {
	t.Helper()
	res, err := a.Run(context.Background(), RunRequest{Prompt: "x"})
	if res == nil {
		return "", err
	}
	return res.Text, err
}

// TestMimoPartialTextThenDroppedFinishGuarded issue 实测复现场景：先吐 text
// 事件、终态 step_finish 行 1.2MB 被 lineWriter 丢弃——修复前陈旧片段以
// nil error 成功返回；修后守门报错。
func TestMimoPartialTextThenDroppedFinishGuarded(t *testing.T) {
	script := "printf '%s\\n' '{\"type\":\"text\",\"part\":{\"text\":\"部分输出\"}}'\n" +
		bigJSONLine(`{"type":"step_finish","sessionID":"s1","part":{"tokens":{"input":1},"cost":0,"pad":"`, `"}}`+"\n")
	m := NewMimo()
	m.Bin = fakeCLI(t, script)

	text, err := runText(t, m)
	if err == nil || !strings.Contains(err.Error(), "mimo") {
		t.Fatalf("终态行被丢后不得把陈旧部分文本当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestMimoTruncatedStreamWithPartialTextGuarded 截断变体：小行 text 事件流
// 8.5MB 触发截断留痕——修复前最后一个片段以成功返回；修后 fail-closed。
func TestMimoTruncatedStreamWithPartialTextGuarded(t *testing.T) {
	script := "yes '{\"type\":\"text\",\"part\":{\"text\":\"片段\"}}' | head -c 8500000\n"
	m := NewMimo()
	m.Bin = fakeCLI(t, script)

	text, err := runText(t, m)
	if err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("流截断后不得把陈旧片段当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestMimoNoTerminalToleranceUnaffected 反证：无终态宽容路径保留——吐过文本、
// stdout 无 step_finish 形状、无截断留痕（旧版本 CLI 不发终态事件）时照旧
// 成功返回文本，守门不得误伤。
func TestMimoNoTerminalToleranceUnaffected(t *testing.T) {
	script := "printf '%s\\n' '{\"type\":\"text\",\"part\":{\"text\":\"增量片段\"}}'\n"
	m := NewMimo()
	m.Bin = fakeCLI(t, script)

	text, err := runText(t, m)
	if err != nil {
		t.Fatalf("无终态宽容路径不得报错: %v", err)
	}
	if text != "增量片段" {
		t.Fatalf("Text = %q", text)
	}
}

// TestCodexPartialTextThenDroppedMessageGuarded issue 实测复现场景：先吐一条
// 小 assistant_message（进 lastText）、最终消息 1.2MB 被丢、last-message 文件
// 缺席——修复前陈旧片段以成功返回；修后对账（2 条 vs 识别 1 条）报错。
func TestCodexPartialTextThenDroppedMessageGuarded(t *testing.T) {
	script := "printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"item_type\":\"assistant_message\",\"text\":\"片段\"}}'\n" +
		bigJSONLine(`{"type":"item.completed","item":{"item_type":"assistant_message","text":"`, `"}}`+"\n")
	c := NewCodex()
	c.Bin = fakeCLI(t, script)

	text, err := runText(t, c)
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("终态消息行被丢后不得把陈旧片段当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestCodexPartialTextTruncatedGuarded 截断变体：last-message 文件缺席 +
// 流截断留痕——对账不可证，fail-closed。
func TestCodexPartialTextTruncatedGuarded(t *testing.T) {
	script := "yes '{\"type\":\"item.completed\",\"item\":{\"item_type\":\"assistant_message\",\"text\":\"片段\"}}' | head -c 8500000\n"
	c := NewCodex()
	c.Bin = fakeCLI(t, script)

	text, err := runText(t, c)
	if err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("文件缺席+流截断不得把陈旧片段当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestCodexLastTextToleranceUnaffected 反证：last-message 文件缺席（旧版本/
// 写文件失败）但 assistant_message 条数对齐、无截断——lastText 是完整最终
// 消息，宽容路径照旧成功，对账守门不得误伤。
func TestCodexLastTextToleranceUnaffected(t *testing.T) {
	script := "printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"item_type\":\"assistant_message\",\"text\":\"完整消息\"}}'\n"
	c := NewCodex()
	c.Bin = fakeCLI(t, script)

	text, err := runText(t, c)
	if err != nil {
		t.Fatalf("条数对齐的宽容路径不得报错: %v", err)
	}
	if text != "完整消息" {
		t.Fatalf("Text = %q", text)
	}
}

// TestKimiPartialTextThenDroppedMessageGuarded kimi 同形态：先吐一条小
// assistant 消息、最终消息 1.2MB 被丢——修复前陈旧片段以成功返回。
func TestKimiPartialTextThenDroppedMessageGuarded(t *testing.T) {
	script := "printf '%s\\n' '{\"role\":\"assistant\",\"content\":\"片段\"}'\n" +
		bigJSONLine(`{"role":"assistant","content":"`, `"}`+"\n")
	k := NewKimi()
	k.Bin = fakeCLI(t, script)

	text, err := runText(t, k)
	if err == nil || !strings.Contains(err.Error(), "kimi") {
		t.Fatalf("正文行被丢后不得把陈旧片段当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestKimiTruncatedStreamWithPartialTextGuarded kimi 截断变体：流截断留痕在场
// 时手里的小片段可能是断片，fail-closed。
func TestKimiTruncatedStreamWithPartialTextGuarded(t *testing.T) {
	script := "yes '{\"role\":\"assistant\",\"content\":\"片段\"}' | head -c 8500000\n"
	k := NewKimi()
	k.Bin = fakeCLI(t, script)

	text, err := runText(t, k)
	if err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("流截断后不得把陈旧片段当成功返回（tools#105）: text=%q err=%v", text, err)
	}
}

// TestPiPartialTextGateIndependentOfText pi 核对（issue 要求同形态核查）：
// pi 的守门条件是 !sawAssistant（与文本空否无关），assistant message_end
// 被丢 ⇒ sawAssistant=false ⇒ 守门照常接管——无 text 非空旁路面。
func TestPiPartialTextGateIndependentOfText(t *testing.T) {
	script := bigJSONLine(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"`, `"}]}}`+"\n")
	p := NewPi()
	p.Bin = fakeCLI(t, script)

	res, err := p.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "终态") {
		t.Fatalf("超长 message_end 被丢应守门报错（pi 本无旁路）: %+v err=%v", res, err)
	}
	if res != nil {
		t.Fatalf("守门报错路径不得同时返回结果: %+v", res)
	}
}

// TestContentLineErr 单源判定：截断留痕在场即报错；条数对齐放行；多出的行报错。
func TestContentLineErr(t *testing.T) {
	line := `{"role":"assistant","content":"片段"}`
	isContent := kimiTerminalLine
	if err := contentLineErr("kimi", "…（stdout 超过 8388608 字节上限，已截断）", 0, isContent); err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("截断留痕必须 fail-closed: %v", err)
	}
	if err := contentLineErr("kimi", line+"\n"+line+"\n", 2, isContent); err != nil {
		t.Fatalf("条数对齐必须放行: %v", err)
	}
	if err := contentLineErr("kimi", line+"\n"+line+"\n", 1, isContent); err == nil || !strings.Contains(err.Error(), "仅识别 1 条") {
		t.Fatalf("多出的行必须报错: %v", err)
	}
	if err := contentLineErr("kimi", "plain text mode", 0, isContent); err != nil {
		t.Fatalf("无正文形状必须放行: %v", err)
	}
}

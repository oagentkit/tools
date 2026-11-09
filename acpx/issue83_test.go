package acpx

import (
	"context"
	"strings"
	"testing"
)

// 本文件钉 tools#83 的回归：procx lineWriter 对单行超 maxLineLen（1MB）的行
// 整行丢弃且对 OnLine 调用方零信号（runtime 仓 procx 侧语义）——acpx 各适配器
// 把「OnLine 见到过终态事件」当终态判据，终态行落进丢弃窗后 hasTerm=false，
// 走全文兜底把整条 NDJSON 事件流原文（可达数 MB）当 RunResult.Text 成功返回。
// 修后：各适配器在兜底前过终态丢失守门 lostTerminalErr——stdout 里存在协议
// 终态形状的行却没被识别为终态（行被丢或协议漂移）时如实报错，宁误报不静默。

// bigLine 构造一条 >1MB 的合法单行事件（fake CLI 内联 heredoc 放不下，
// 用 printf + head -c 拼）。
func bigLine(prefix, suffix string) string {
	return "printf '" + prefix + "'; head -c 1200000 /dev/zero | tr '\\0' 'a'; printf '" + suffix + "'\n"
}

// TestClaudeOversizedTerminalLineGuarded issue 实测复现场景：假 claude CLI
// 输出一行 1.2MB 的合法 result 事件——修复前 Run 返回 Text=整条 JSONL 原文、
// err=nil；修后守门报错。
func TestClaudeOversizedTerminalLineGuarded(t *testing.T) {
	script := bigLine(`{"type":"result","result":"`, `","session_id":"sess-9","is_error":false}`+"\n")
	bin := fakeCLI(t, script)
	c := NewClaudeCode()
	c.Bin = bin

	res, err := c.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "终态") {
		t.Fatalf("超长终态行被丢后不得把事件流原文当回答（tools#83）: %v", err)
	}
	if res != nil {
		t.Fatalf("守门报错路径不得同时返回结果: %+v", res)
	}
}

// TestKimiOversizedAssistantLineGuarded kimi 无协议终态、assistant 消息行即
// 正文来源：1.2MB 的 assistant 行被丢后同样不得全文兜底。（tools#105 起 kimi
// 守门改走 contentLineErr 对账——报错措辞是「正文形状的行」而非「终态」。）
func TestKimiOversizedAssistantLineGuarded(t *testing.T) {
	script := bigLine(`{"role":"assistant","content":"`, `"}`+"\n")
	bin := fakeCLI(t, script)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "拒绝把") {
		t.Fatalf("超长 assistant 行被丢后不得把事件流原文当回答（tools#83/#105）: %v", err)
	}
	if res != nil {
		t.Fatalf("守门报错路径不得同时返回结果: %+v", res)
	}
}

// TestKimiPlainTextFallbackUnaffected 对照组：stream-json 不可用（旧版本/
// 文本模式，stdout 无 JSON 行）时全文兜底照旧——守门不得误伤合法兜底。
func TestKimiPlainTextFallbackUnaffected(t *testing.T) {
	bin := fakeCLI(t, `echo '这是旧版 CLI 的纯文本输出'`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x"})
	if err != nil {
		t.Fatalf("纯文本兜底路径不得报错: %v", err)
	}
	if res.Text != "这是旧版 CLI 的纯文本输出" {
		t.Fatalf("Text = %q", res.Text)
	}
}

// TestLostTerminalErr 守门单源：hasTerm=true 放行、形状命中报错、无形状放行。
func TestLostTerminalErr(t *testing.T) {
	if err := lostTerminalErr("x", "{\"type\":\"result\"}\n", true, claudeTerminalLine); err != nil {
		t.Fatalf("hasTerm=true 必须放行: %v", err)
	}
	if err := lostTerminalErr("x", "noise\nno shape here\n", false, claudeTerminalLine); err != nil {
		t.Fatalf("无终态形状必须放行: %v", err)
	}
	err := lostTerminalErr("x", "warn line\n{\"type\":\"result\",\"result\":\"big\"}\n", false, claudeTerminalLine)
	if err == nil || !strings.Contains(err.Error(), "acpx: x") {
		t.Fatalf("形状命中必须报错并点名适配器: %v", err)
	}
}

// TestTerminalLineMatchers 各家终态形状判定：命中自家形状、放过别家/内嵌
// 形态（assistant 文本里内嵌的 {"type":"result"} 是字符串值，不影响顶层判定）。
func TestTerminalLineMatchers(t *testing.T) {
	nested := `{"type":"assistant","message":{"content":[{"type":"text","text":"{\"type\":\"result\"}"}]}}`
	cases := []struct {
		line string
		want map[string]bool
	}{
		{`{"type":"result","result":"r"}`, map[string]bool{"claude": true}},
		{nested, map[string]bool{"claude": false}},
		{`{"type":"step_finish","sessionID":"s"}`, map[string]bool{"mimo": true}},
		{`{"type":"message_end","message":{"role":"assistant","content":[]}}`, map[string]bool{"pi": true}},
		{`{"type":"message_end","message":{"role":"user","content":[]}}`, map[string]bool{"pi": false}},
		{`{"role":"assistant","content":"正文"}`, map[string]bool{"kimi": true}},
		{`{"role":"assistant","content":""}`, map[string]bool{"kimi": false}},
		{`{"type":"item.completed","item":{"item_type":"assistant_message","text":"t"}}`, map[string]bool{"codex": true}},
		{`{"type":"item.completed","item":{"item_type":"tool_call"}}`, map[string]bool{"codex": false}},
		{`not json`, map[string]bool{"claude": false, "mimo": false, "pi": false, "kimi": false, "codex": false}},
	}
	matchers := map[string]func(string) bool{
		"claude": claudeTerminalLine, "mimo": mimoTerminalLine, "pi": piTerminalLine,
		"kimi": kimiTerminalLine, "codex": codexTerminalLine,
	}
	for _, tc := range cases {
		for name, want := range tc.want {
			if got := matchers[name](tc.line); got != want {
				t.Fatalf("%s 判定 %q = %v，想要 %v", name, tc.line, got, want)
			}
		}
	}
}

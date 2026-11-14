package acpx

import (
	"context"
	"strings"
	"testing"
)

// 本文件钉 tools#94 的回归：procx cappedBuffer 只保 stdout 前 8MB，超出部分
// 连同超长终端行一起被丢（复合变体：终端行 >1MB × 流总量 >8MB）——此前
// lostTerminalErr 只做形状扫描，在残缺 stdout 里找不到形状就放行兜底。
// 修后：无终态 + 尾部截断留痕 ⇒ fail-closed 报错，拒绝兜底残缺事件流。

// TestTruncatedVerdictFromStructuredField_Issue178 tools#178 回归钉：截断判定
// 面 = procx 结构化字段（RunResult.StdoutTruncated）而非 noteTruncated 人读
// 文案——判定面与文案解耦。双向钉：
//   - 结构化字段为 true 且 stdout 无任何截断文案 ⇒ 守门照常命中（文案缺席
//     不再失效——procx 改文案不再静默重开 tools#94 的已关洞）；
//   - stdout 恰含完整截断标记文案但结构化字段为 false ⇒ 守门不接管（代理
//     CLI 自身输出以该短语收尾不再误判 fail-closed 假红）。
func TestTruncatedVerdictFromStructuredField_Issue178(t *testing.T) {
	marker := "\n…（stdout 超过 8388608 字节上限，已截断）"
	if err := lostTerminalErr("claude", "plain events", true, false, claudeTerminalLine); err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("结构化截断事实在场必须 fail-closed（判定面=字段而非文案，tools#178）: %v", err)
	}
	if err := lostTerminalErr("claude", "events"+marker, false, false, claudeTerminalLine); err != nil {
		t.Fatalf("stdout 含完整截断文案但结构化字段为 false 不得接管（拿文案当协议，tools#178）: %v", err)
	}
}

// TestLostTerminalErrTruncatedGate 无终态 + 截断事实在场 ⇒ fail-closed；
// hasTerm=true 时截断事实不接管（终态优先）。
func TestLostTerminalErrTruncatedGate(t *testing.T) {
	if err := lostTerminalErr("claude", "events", true, false, claudeTerminalLine); err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("无终态+截断事实在场必须 fail-closed: %v", err)
	}
	if err := lostTerminalErr("claude", "events", true, true, claudeTerminalLine); err != nil {
		t.Fatalf("hasTerm=true 截断事实不得接管: %v", err)
	}
	if err := lostTerminalErr("claude", "plain fallback text", false, false, claudeTerminalLine); err != nil {
		t.Fatalf("无截断事实的合法兜底不得报错: %v", err)
	}
}

// TestClaudeIsErrorEmptyResultGuarded claude is_error 且 result 空串：不得静默成功。
func TestClaudeIsErrorEmptyResultGuarded(t *testing.T) {
	script := "printf '%s\\n' '{\"type\":\"result\",\"result\":\"\",\"session_id\":\"s1\",\"is_error\":true}'\n"
	bin := fakeCLI(t, script)
	c := NewClaudeCode()
	c.Bin = bin

	res, err := c.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "is_error") {
		t.Fatalf("is_error=true 空串 result 不得静默成功（tools#94）: %v", err)
	}
	if res != nil {
		t.Fatalf("报错路径不得同时返回结果: %+v", res)
	}
}

// TestClaudeTruncatedCompoundStreamGuarded 复合变体实测：小行事件流 8.5MB 挤掉
// 1.2MB 超长终端行（OnLine 1MB 行上限 + buffer 8MB 上限双重命中）⇒ fail-closed。
func TestClaudeTruncatedCompoundStreamGuarded(t *testing.T) {
	script := "yes '{\"type\":\"assistant\",\"message\":{\"content\":[]}}' | head -c 8500000\n" +
		"printf '%s' '{\"type\":\"result\",\"result\":\"'; head -c 1200000 /dev/zero | tr '\\0' 'a'; printf '\",\"session_id\":\"s9\",\"is_error\":false}\\n'\n"
	bin := fakeCLI(t, script)
	c := NewClaudeCode()
	c.Bin = bin

	res, err := c.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "截断") {
		t.Fatalf("复合变体（终端行>1MB × 流>8MB）必须 fail-closed（tools#94）: %v", err)
	}
	if res != nil {
		t.Fatalf("守门报错路径不得同时返回结果: %+v", res)
	}
}

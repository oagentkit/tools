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

// TestStdoutTruncatedMarker stdoutTruncated 判定：完整标记形态才命中，
// 「（stdout 超过」短语 + 尾缀「已截断）」双条件从紧。
func TestStdoutTruncatedMarker(t *testing.T) {
	cases := []struct {
		stdout string
		want   bool
	}{
		{"plain output", false},
		{"…（stdout 超过 8388608 字节上限）", false},                // 缺尾缀
		{"noise\n已截断）", false},                              // 缺短语
		{"events\n…（stdout 超过 8388608 字节上限，已截断）", true},     // 完整标记
		{"events\n…（stdout 超过 8388608 字节上限，已截断）\n\n", true}, // 尾随空白容忍
	}
	for _, tc := range cases {
		if got := stdoutTruncated(tc.stdout); got != tc.want {
			t.Fatalf("stdoutTruncated(%q) = %v，想要 %v", tc.stdout, got, tc.want)
		}
	}
}

// TestLostTerminalErrTruncatedGate 无终态 + 截断留痕 ⇒ fail-closed；
// hasTerm=true 时截断留痕不接管（终态优先）。
func TestLostTerminalErrTruncatedGate(t *testing.T) {
	marker := "\n…（stdout 超过 8388608 字节上限，已截断）"
	if err := lostTerminalErr("claude", "events"+marker, false, claudeTerminalLine); err == nil || !strings.Contains(err.Error(), "tools#94") {
		t.Fatalf("无终态+截断留痕必须 fail-closed: %v", err)
	}
	if err := lostTerminalErr("claude", "events"+marker, true, claudeTerminalLine); err != nil {
		t.Fatalf("hasTerm=true 截断留痕不得接管: %v", err)
	}
	if err := lostTerminalErr("claude", "plain fallback text", false, claudeTerminalLine); err != nil {
		t.Fatalf("无截断留痕的合法兜底不得报错: %v", err)
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

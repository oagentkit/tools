package acpx

// 2026-11-11 issue 清零战回归测试（#126 #127）：每条用例钉一个失效形态，
// 标题带 issue 号——回归时按号检索。

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestEnvelopeErrMsgObjectFallback_Issue127 对象形态无 message 键/为空必须
// 回退原文——「其余形态回退原文」对对象形态曾结构性不可达，{"code":500} 被
// 吞成无失败、错误经全文兜底变成「成功」的垃圾正文。
func TestEnvelopeErrMsgObjectFallback_Issue127(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"字符串形态直取", `"boom"`, "boom"},
		{"对象 message 键", `{"message":"配额耗尽"}`, "配额耗尽"},
		{"对象无 message 键→原文", `{"code":500,"msg":"配额耗尽"}`, `{"code":500,"msg":"配额耗尽"}`},
		{"对象 message 空→原文", `{"message":""}`, `{"message":""}`},
		{"null=无失败", `null`, ""},
		{"空=无失败", "", ""},
	}
	for _, c := range cases {
		if got := envelopeErrMsg(json.RawMessage(c.raw)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	// 端到端：无 message 键的对象信封必须如实上抛，不得走全文兜底。
	var stdout strings.Builder
	stdout.WriteString(`{"text":"","error":{"code":500,"msg":"配额耗尽"}}`)
	_, err := envelopeResult(RunRequest{}, "test-agent", stdout.String(), "", 0)
	if err == nil {
		t.Fatal("对象信封错误应如实上抛（tools#127）")
	}
	if !strings.Contains(err.Error(), "配额耗尽") {
		t.Fatalf("错误应含原文文案，实际 %v", err)
	}
}

// TestValidateTemplateRequiresPrompt_Issue126 模板必须恰含一个 {prompt}——
// 漏写任务载体时 prompt 被静默丢弃而运行照常「成功」，与 RunRequest 强制
// Prompt 非空自相矛盾。
func TestValidateTemplateRequiresPrompt_Issue126(t *testing.T) {
	// 零 {prompt}：拒绝。
	if err := validateTemplate([]string{"run", "--json"}); err == nil {
		t.Fatal("无 {prompt} 应报错（tools#126）")
	}
	// 双 {prompt}：同样拒绝（首个吞 prompt、其余收空串）。
	if err := validateTemplate([]string{"{prompt}", "{prompt}"}); err == nil {
		t.Fatal("多 {prompt} 应报错")
	}
	// 内联占位符：既有守卫保持。
	if err := validateTemplate([]string{"run", "--prompt={prompt}"}); err == nil {
		t.Fatal("内联占位符应报错")
	}
	// 合法形态：独占参数位，恰一个。
	if err := validateTemplate([]string{"run", "--json", "{prompt}"}); err != nil {
		t.Fatalf("合法模板不应报错: %v", err)
	}
	// 端到端：NewGenericAgent 构造即校验，漏 {prompt} 在 Run 前暴露。
	g := NewGenericAgent("x", []string{"run", "--json"}, true)
	if _, err := g.Run(context.Background(), RunRequest{Prompt: "do it"}); err == nil || !strings.Contains(err.Error(), "{prompt}") {
		t.Fatalf("漏 {prompt} 的模板应在 Run 前 fail-fast，实际 err=%v", err)
	}
}

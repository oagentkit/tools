package acpx

import (
	"context"
	"strings"
	"testing"
)

// tools#110：gemini/qwen 适配器的 error 信封被吞成「成功」、stdout 截断静默兜底。
// 失败形态必须返回 error（宁误报不静默给垃圾）——错误原文/8MB 残缺 JSON 被当
// 最终回答注入下游 agent 循环是 claude#82 同类教训的复发。

// TestGeminiErrorEnvelopeStringForm 回归（tools#110）：error 是**字符串**形态。
// 此前 geminiOut.Error 声明为 *struct{Message string}，字符串让整份 JSON 解析
// 失败 ⇒ 落进全文兜底 ⇒ 错误原文当成功返回（err=nil）。
func TestGeminiErrorEnvelopeStringForm(t *testing.T) {
	bin := fakeCLI(t, `echo '{"error":"quota exceeded"}'`)
	g := NewGemini()
	g.Bin = bin

	var events []Event
	res, err := g.Run(context.Background(), RunRequest{Prompt: "x", OnEvent: func(e Event) { events = append(events, e) }})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("error 字符串信封应如实报错: res=%+v err=%v", res, err)
	}
	if res != nil {
		t.Fatalf("失败跑不得返回结果: %+v", res)
	}
	if len(events) != 1 || events[0].Type != EventError {
		t.Fatalf("应转发 EventError: %v", events)
	}
}

// TestGeminiErrorEnvelopeObjectNoMessage 回归（tools#110）：error 是对象但
// **没有 message 键**。此前 message 判据不成立 ⇒ 继续走空响应兜底 ⇒ 错误
// JSON 原文当成功返回（err=nil）。
func TestGeminiErrorEnvelopeObjectNoMessage(t *testing.T) {
	bin := fakeCLI(t, `echo '{"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}'`)
	g := NewGemini()
	g.Bin = bin

	res, err := g.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") {
		t.Fatalf("无 message 的错误对象应按原文报错: res=%+v err=%v", res, err)
	}
	if res != nil {
		t.Fatalf("失败跑不得返回结果: %+v", res)
	}
}

// TestGeminiErrorEnvelopeNullMeansOK 钉住 "error":null 的既有形态：null/缺省
// = 无失败（不因「存在即失败」误伤正常跑）。
func TestGeminiErrorEnvelopeNullMeansOK(t *testing.T) {
	bin := fakeCLI(t, `echo '{"response":"ok","error":null}'`)
	g := NewGemini()
	g.Bin = bin

	res, err := g.Run(context.Background(), RunRequest{Prompt: "x"})
	if err != nil {
		t.Fatalf("error=null 不是失败: %v", err)
	}
	if res == nil || res.Text != "ok" {
		t.Fatalf("Text = %+v", res)
	}
}

// TestGeminiStdoutTruncatedNotFallback 回归（tools#110）：stdout 触发 procx
// 8MB 截断 ⇒ 单对象 JSON 必然残缺 ⇒ 拒绝把截断残片（含截断留痕尾注）当最终
// 回答兜底——对齐 mimo/codex/kimi 的截断守门纪律（tools#94，PR #109）。
func TestGeminiStdoutTruncatedNotFallback(t *testing.T) {
	bin := fakeCLI(t, `printf '{"response":"'; yes A | tr -d '\n' | head -c 9000000; printf '"}'`)
	g := NewGemini()
	g.Bin = bin

	res, err := g.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "截断") {
		t.Fatalf("stdout 截断应报错而非兜底残片: res 长度=%d err=%v", len(res.Text), err)
	}
	if res != nil {
		t.Fatalf("截断跑不得返回结果（8MB 残缺 JSON 会按 token 账单全款走账）")
	}
}

// TestGeminiEnvelopeDriftWithErrorStillFails 回归（tools#110 修法 2）：主解析
// 失败（其它字段形态漂移）但 error 信封在场时，兜底前须复查 error 形态——
// 不得借「解析失败 → 全文兜底」把错误原文当成功答案。
func TestGeminiEnvelopeDriftWithErrorStillFails(t *testing.T) {
	bin := fakeCLI(t, `echo '{"error":"quota exceeded","response":123,"stats":"drifted"}'`)
	g := NewGemini()
	g.Bin = bin

	res, err := g.Run(context.Background(), RunRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("解析失败但 error 在场应报错而非兜底: res=%+v err=%v", res, err)
	}
}

package acpx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oagentkit/runtime/procx"
)

func TestKimiStreamJSON(t *testing.T) {
	// 官方协议：--print -p + --output-format=stream-json → JSONL 消息流
	bin := fakeCLI(t, `cat <<'EOF'
{"role":"assistant","content":"先看一下目录结构"}
{"role":"tool","content":"$ ls"}
{"role":"assistant","content":"共有 3 个 Python 文件"}
EOF
`)
	k := NewKimi()
	k.Bin = bin

	var events []Event
	res, err := k.Run(context.Background(), RunRequest{
		Prompt:  "列出 Python 文件",
		OnEvent: func(e Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatal(err)
	}
	// 最终文本 = 最后一条 assistant 消息（跳过中间 tool 消息）
	if res.Text != "共有 3 个 Python 文件" {
		t.Fatalf("Text = %q", res.Text)
	}
	// 事件只有 assistant 消息（tool 消息不回调）
	if len(events) != 2 {
		t.Fatalf("事件数 = %d", len(events))
	}
}

func TestKimiNonJSONFallback(t *testing.T) {
	bin := fakeCLI(t, `echo "纯文本输出"`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "纯文本输出" {
		t.Fatalf("Text = %q", res.Text)
	}
}

func TestKimiArgBuilding(t *testing.T) {
	bin := fakeCLI(t, `echo "$@" > "$FAKE_ARGS_FILE"; echo '{"role":"assistant","content":"ok"}'`)
	k := NewKimi()
	k.Bin = bin

	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	if _, err := k.Run(context.Background(), RunRequest{
		Prompt: "hi", Model: "k2", SessionID: "sess-7",
		Env: []string{"FAKE_ARGS_FILE"},
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(argsFile)
	args := string(raw)
	for _, want := range []string{"-p", "hi", "--output-format", "stream-json", "--model", "k2", "--session", "sess-7"} {
		if !strings.Contains(args, want) {
			t.Errorf("参数缺 %q: %s", want, args)
		}
	}
}

// TestKimiExitNonZeroWithResult 已收到 assistant 消息（终态）但进程退出码
// 非零：以结果为准（终态优先，与 claude/codex/pi/mimo 同契约）。
func TestKimiExitNonZeroWithResult(t *testing.T) {
	bin := fakeCLI(t, `cat <<'JSONL'
{"role":"assistant","content":"kimi 部分完成"}
JSONL
exit 2
`)
	k := NewKimi()
	k.Bin = bin
	res, err := k.Run(context.Background(), RunRequest{Prompt: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "kimi 部分完成" || res.ExitCode != 2 {
		t.Fatalf("res: %+v", res)
	}
}

// TestKimiInterruptedPropagatesError 回归（tools#46）：超时被杀但被杀前吐过
// assistant 消息——err 曾被吞、整体按成功返回（ExitCode 非 0 而 error 为 nil，
// 调用方拿半截任务文本无从察觉）。中断类执行错现在如实上抛，部分文本挂
// RunResult 且 Interrupted=true；「跑完了但退出码非零」的终态优先路径
// （TestKimiExitNonZeroWithResult）不受影响。
func TestKimiInterruptedPropagatesError(t *testing.T) {
	bin := fakeCLI(t, `echo '{"role":"assistant","content":"半截任务的部分输出"}'
sleep 30`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x", Timeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("超时被杀不得因有部分文本而当成功返回")
	}
	if !errors.Is(err, procx.ErrTimeout) {
		t.Fatalf("超时应可 errors.Is 分类: %v", err)
	}
	if res == nil || res.Text != "半截任务的部分输出" || !res.Interrupted {
		t.Fatalf("部分文本应挂 RunResult 且 Interrupted=true: %+v", res)
	}
}

// TestKimiSignalKillPropagatesError 信号杀死（非超时路径）同样不得当成功：
// 外部 kill/OOM 的 ExitError 信号形态也是「没跑完」。
func TestKimiSignalKillPropagatesError(t *testing.T) {
	bin := fakeCLI(t, `echo '{"role":"assistant","content":"被杀前的部分输出"}'
kill -TERM $$
`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x", Timeout: 30 * time.Second})
	if err == nil {
		t.Fatal("信号杀死不得当成功返回")
	}
	if res == nil || !res.Interrupted || res.Text != "被杀前的部分输出" {
		t.Fatalf("部分文本应挂 RunResult 且 Interrupted=true: %+v", res)
	}
}

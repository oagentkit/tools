package acpx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgprocx "github.com/oekit/pkg/procx"
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
//
// 预算必须主导 spawn（tools#206 根因）：本用例断言「超时前输出已捕获」，
// 脆面是 **预算 < 子进程 spawn+产出耗时**——负载/-race 下 fork/exec+shebang
// 超过 300ms 时 shell 还没跑到 echo 就被杀，Raw/Text 双空（4 路 -race 并行
// 实测复现 8 红，签名 `Text:"" Raw:[] ExitCode:-1`）。Raw 也空是关键证据：
// 若是「杀与读」的管道竞态，lineW.Write 先于行解析写 buf，Raw 必有内容——
// 故 tools#206 猜想的读取侧竞态被证伪，真机制是预算过紧。2s 预算 = spawn
// 抖动的充分余量（实测最坏 spawn 远小于 1s），sleep 30 保证超时路径必然走到。
func TestKimiInterruptedPropagatesError(t *testing.T) {
	bin := fakeCLI(t, `echo '{"role":"assistant","content":"半截任务的部分输出"}'
sleep 30`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x", Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("超时被杀不得因有部分文本而当成功返回")
	}
	if !errors.Is(err, pkgprocx.ErrTimeout) {
		t.Fatalf("超时应可 errors.Is 分类: %v", err)
	}
	if res == nil || res.Text != "半截任务的部分输出" || !res.Interrupted {
		// 诊断分叉：Raw 空 = 预算早于产出（spawn 未及执行 echo，见根因注释）；
		// Raw 非空但 Text 空 = 行解析问题——两种失败机制修法不同，报文必须分开。
		if res != nil && len(res.Raw) == 0 {
			t.Fatalf("部分文本缺失且 Raw 为空：预算早于子进程产出（预算过紧/负载过高），非读取竞态: %+v", res)
		}
		t.Fatalf("部分文本应挂 RunResult 且 Interrupted=true: %+v", res)
	}
}

// TestKimiInterruptedBudgetBeforeProduce 反向对照（tools#206 验收的适配版）：
// 把「预算早于产出」钉成**确定性**用例——sleep 前置于 echo，300ms 预算必然
// 杀在产出之前，输出必空。它证明主用例的断言不是空转（缺输出真的会被抓），
// 并把 #206 猜想的「先读后杀」竞态替换为真实机制「预算先于产出」：
// Raw/Text 双空 + ErrTimeout + Interrupted 就是预算过紧的完整签名。
func TestKimiInterruptedBudgetBeforeProduce(t *testing.T) {
	bin := fakeCLI(t, `sleep 1
echo '{"role":"assistant","content":"不该被看到"}'`)
	k := NewKimi()
	k.Bin = bin

	res, err := k.Run(context.Background(), RunRequest{Prompt: "x", Timeout: 300 * time.Millisecond})
	if !errors.Is(err, pkgprocx.ErrTimeout) {
		t.Fatalf("预算早于产出应报 ErrTimeout: %v", err)
	}
	if res == nil || !res.Interrupted {
		t.Fatalf("仍应是中断形态（Interrupted=true）: %+v", res)
	}
	if len(res.Raw) != 0 || res.Text != "" {
		t.Fatalf("预算杀在产出之前：Raw/Text 必须双空（预算过紧签名），实得 Raw=%q Text=%q", res.Raw, res.Text)
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

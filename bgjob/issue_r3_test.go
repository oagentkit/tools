package bgjob

// 2026-11-13 第三轮清零回归（#161）：marker 加任务身份——同路径复用时
// **在飞**旧任务的迟到写回不得被新任务认领为终态（#135 的 rm -f 只覆盖
// 「已落盘」形态）。端到端依赖 Linux setsid（macOS 无此工具），与既有测试
// 同用 setsidShim + 真 shell。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestMarkerPathCarriesTaskIdentity_Issue161 两次 Start 同 OutputPath →
// marker 路径必须带互不相同的身份；启动/探测/清理命令只引用自己的 marker。
func TestMarkerPathCarriesTaskIdentity_Issue161(t *testing.T) {
	out := "/var/log/build.out"
	rs := &recordingSession{next: func(string) (string, int, error) { return "", 0, nil }}
	j1, err := Start(rs, "sleep 1", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	j2, err := Start(rs, "sleep 1", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(j1.MarkerPath, out+".rc.") {
		t.Fatalf("marker 应带任务身份（输出路径+.rc.+身份）: %s", j1.MarkerPath)
	}
	if j1.MarkerPath == j2.MarkerPath || j1.MarkerPath == out+".rc" {
		t.Fatalf("两次 Start 的 marker 必须互不相同（#135 的固定路径正是缺陷面）: %s vs %s",
			j1.MarkerPath, j2.MarkerPath)
	}
	// 启动链只动自己的 marker：j2 的启动命令含 j2 的 marker（rm -f + echo 各一
	// 次），不含 j1 的——旧 wrapper 写回的路径不会被新任务的启动链清掉/认领。
	start2 := rs.commands[1]
	if got := strings.Count(start2, j2.MarkerPath); got != 2 {
		t.Fatalf("启动链应恰好引用自己的 marker 两次（rm -f + echo $?，got %d）: %s", got, start2)
	}
	if strings.Contains(start2, j1.MarkerPath) {
		t.Fatalf("启动链不得引用上一任务的 marker: %s", start2)
	}
	// 探测命令同样只认自己的 marker。
	rsProbe := &recordingSession{next: func(string) (string, int, error) {
		return lineFinished + " 0\n", 0, nil
	}}
	j2.sess = rsProbe
	if _, _, err := j2.Status(); err != nil {
		t.Fatal(err)
	}
	probe := rsProbe.commands[0]
	if !strings.Contains(probe, j2.MarkerPath) || strings.Contains(probe, j1.MarkerPath) {
		t.Fatalf("探测命令应只引用自己的 marker: %s", probe)
	}
}

// TestInFlightPredecessorNotClaimed_Issue161 端到端：A 在飞时 Start B（同
// OutputPath），A 结束（旧 wrapper 写 A 的 marker）后——B 不得被判 finished、
// Cleanup 不得把仍在写的输出当终态删掉；B 跑完用自己的 marker 收终态。
func TestInFlightPredecessorNotClaimed_Issue161(t *testing.T) {
	setsidShim(t)
	out := filepath.Join(t.TempDir(), "out.log")
	a, err := Start(localSession{}, "sleep 0.2", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Start(localSession{}, "sleep 2", WithOutputPath(out)) // A 仍在跑时 Start
	if err != nil {
		t.Fatal(err)
	}
	// A 结束：其 wrapper 已把退出码写进 A 的 marker（固定路径下这一刻 B 会被
	// 误报 finished——issue 实测 t=3.1s 形态）。
	if code := waitState(t, a, StateFinished); code != "0" {
		t.Fatalf("A 应 finished/0: got %q", code)
	}
	// B 仍在跑：状态与清理都不得认领 A 的终态。
	if st, _, err := b.Status(); err != nil || st != StateRunning {
		t.Fatalf("A 结束后 B 仍应 running（在飞旧任务写回不被认领）: %s %v", st, err)
	}
	if ok, err := b.Cleanup(); ok || err == nil {
		t.Fatalf("B 在跑应拒绝清理（不得删仍在写的输出）: %v %v", ok, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("拒绝清理后输出文件应原样: %v", err)
	}
	// B 自己跑完 → 用自己的 marker 报终态，随后可正常收尾。
	if code := waitState(t, b, StateFinished); code != "0" {
		t.Fatalf("B 应 finished/0: got %q", code)
	}
	if ok, err := b.Cleanup(); err != nil || !ok {
		t.Fatalf("B 收尾: %v %v", ok, err)
	}
}

// TestSnapKeepsRuneBoundary_Issue165 输出快照按 rune 边界截断（≤200 字节
// 预算）：3 字节汉字跨 200 边界时按字节切片会劈半字符，尾部必现 U+FFFD。
func TestSnapKeepsRuneBoundary_Issue165(t *testing.T) {
	// 100 个「中」= 300 字节；200 落点切进第 67 个字（200 = 3×66+2）。
	cn := strings.Repeat("中", 100)
	got := snap(cn)
	if strings.ContainsRune(got, '�') {
		t.Fatalf("截断点劈开多字节字符（U+FFFD 乱码尾巴，tools#165）: %q", got)
	}
	body, ok := strings.CutSuffix(got, "…")
	if !ok {
		t.Fatalf("超预算应带省略号: %q", got)
	}
	if !strings.HasPrefix(cn, body) || !utf8.ValidString(body) {
		t.Fatalf("截断体必须是原串的完整 rune 前缀: %q", body)
	}
	// 预算语义不变：截断体 ≤200 字节（省略号另计）。
	if len(body) > 200 {
		t.Fatalf("截断体应 ≤200 字节: got %d", len(body))
	}
	// 无需截断时原样；纯 ASCII 形态长度不变。
	if got := snap("短输出"); got != "短输出" {
		t.Fatalf("预算内应原样: %q", got)
	}
	if got := snap(strings.Repeat("a", 300)); got != strings.Repeat("a", 200)+"…" {
		t.Fatalf("ASCII 截断应恰为 200 字节+省略号: len=%d", len(got))
	}
}

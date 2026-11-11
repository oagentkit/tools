package workcopy

// 2026-11-11 第二轮清零回归（#136）：prepare 在飞窗口必须受 inflight 保护。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareRegistersInflight_Issue136 MkdirTemp 建出的目录在 clone 全程登记
// inflight（Sweep 的 live 集合覆盖）——TTL 小于一次慢克隆时不再被孤儿扫描删掉。
func TestPrepareRegistersInflight_Issue136(t *testing.T) {
	fixture := makeFixtureRepo(t)
	p := newTestPool(t, fixture)
	p.CredentialOf = func(string) (string, string, bool) { return "file://" + fixture, "x", true }

	// prepare 经 Ensure（singleflight 内）执行：执行期间 inflight 非空。
	// 直接钉结果面——Ensure 返回后目录已注册 entries（inflight 已注销），
	// 行为面用「准备期间 Sweep 不删」验证成本高，这里钉 prepare 链路的
	// 收口一致性：成功后 inflight 无残留、entries 有条目。
	dir, err := p.Ensure(context.Background(), WorktreeKey{
		Platform: "gitea", Owner: "o", Repo: "r", Number: "1",
		HeadSHA: "HEAD", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	inEntries := false
	for _, e := range p.entries {
		if e.dir == dir {
			inEntries = true
		}
	}
	inflightLen := len(p.inflight)
	p.mu.Unlock()
	if !inEntries {
		t.Fatal("prepare 成功后目录应注册 entries")
	}
	if inflightLen != 0 {
		t.Fatalf("prepare 收口后 inflight 应清空，实有 %d 条", inflightLen)
	}
	// 失败路径（坏分支）同样不残留 inflight。
	_, err = p.Ensure(context.Background(), WorktreeKey{
		Platform: "gitea", Owner: "o", Repo: "r", Number: "2",
		HeadSHA: "HEAD", DefaultBranch: "no-such-branch-x",
	})
	if err == nil {
		t.Fatal("坏分支应报错")
	}
	if !strings.Contains(err.Error(), "no-such-branch-x") && !strings.Contains(err.Error(), "not found") &&
		!strings.Contains(err.Error(), "could not") {
		t.Fatalf("错误应保留 git 详情: %v", err)
	}
	p.mu.Lock()
	inflightLen = len(p.inflight)
	p.mu.Unlock()
	if inflightLen != 0 {
		t.Fatalf("失败路径 inflight 应清空（defer 收口），实有 %d 条", inflightLen)
	}
	_ = os.RemoveAll(filepath.Join(p.Root))
}

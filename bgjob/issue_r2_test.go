package bgjob

// 2026-11-11 第二轮清零回归（#135）：同路径复用时陈旧 marker 必须被启动链清掉。
// 端到端行为依赖 Linux setsid（macOS 无此工具），与既有测试同用命令形态钉。

import (
	"strings"
	"testing"
)

func TestStartRemovesStaleMarker_Issue135(t *testing.T) {
	cmd := startCommand("sleep 1", "/var/log/build.out", "/var/log/build.out.rc")
	if !strings.Contains(cmd, "rm -f '/var/log/build.out.rc'") {
		t.Fatalf("启动链应在 mkdir 后、截断输出前 rm -f 陈旧 marker（tools#135）: %s", cmd)
	}
	// 顺序不变量：rm -f 在 `: > out` 之前（先清终态凭证再开始写输出）。
	if i := strings.Index(cmd, "rm -f "); i >= 0 {
		if j := strings.Index(cmd, ": > "); j >= 0 && j < i {
			t.Fatalf("rm -f 应先于输出截断: %s", cmd)
		}
	}
	// wrapper 写 marker 的 echo 仍在用户命令之外（内层整段再包引号）。
	if got := strings.Count(cmd, "'/var/log/build.out.rc'"); got != 2 {
		t.Fatalf("marker 路径应出现两次（清陈旧 + 写终态，got %d）: %s", got, cmd)
	}
}

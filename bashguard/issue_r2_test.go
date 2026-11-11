package bashguard

// 2026-11-11 第二轮清零回归（#132 #133 #134）。

import "testing"

// TestDestructiveViaAbsolutePathKernel_Issue132 绝对路径内核（顶层/剥壳后/
// find 载荷）必须命中规则——destructiveRule 单源 basename 归一。
func TestDestructiveViaAbsolutePathKernel_Issue132(t *testing.T) {
	cases := []struct{ cmd string }{
		{"/bin/rm -rf /data"},
		{"xargs /bin/rm -rf"},
		{"env /bin/rm -rf /"},
		{"timeout 5 /usr/bin/rm -rf /data"},
		{"sudo /bin/rm -rf /data"},
		{"find . -exec /bin/rm -rf {} +"},
		{"/usr/bin/mkfs.ext4 /dev/sda"},
		{"./rm -rf /data"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——绝对路径内核免确认直跑（tools#132）", c.cmd)
		}
	}
}

// TestDestructiveByRuleKeepsFlagsInFallback_Issue133 兜底递归不再丢 flag 判据
// ——未登记全局旗退兜底后，子命令的 --hard 照常命中。
func TestDestructiveByRuleKeepsFlagsInFallback_Issue133(t *testing.T) {
	d := Analyze("git --no-literal-pathspecs reset --hard HEAD~3", nil)
	if !d.Destructive {
		t.Fatal("git 未登记全局旗 + reset --hard 应判不可逆（tools#133）")
	}
	// 对照：SafeFlags 白名单内的前置旗走正路，同样命中。
	if d2 := Analyze("git -C /repo reset --hard", nil); !d2.Destructive {
		t.Fatal("正路形态回归")
	}
}

// TestDestructiveThroughShellTextDispatchers_Issue134 sh/bash/zsh/dash -c 与
// eval 的内层命令文本必须整行再判。
func TestDestructiveThroughShellTextDispatchers_Issue134(t *testing.T) {
	cases := []struct{ cmd string }{
		{"bash -c 'rm -rf /'"},
		{"sh -c 'dd if=/dev/zero of=/dev/sda'"},
		{"zsh -c 'mkfs.ext4 /dev/sda'"},
		{"dash -c 'rm -rf /data'"},
		{"ksh -c 'rm -rf /data'"},
		{"eval 'rm -rf /data'"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——shell 文本派发器藏住内核（tools#134）", c.cmd)
		}
	}
	// 对照：无害内核不被误判。
	if d := Analyze("bash -c 'ls -la /tmp'", nil); d.Destructive {
		t.Error("bash -c ls 不应误判 destructive")
	}
}

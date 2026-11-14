package bashguard

// tools#175 回归钉：写重定向到块设备此前不进不可逆判定——destructiveScan
// 只消费调用集（AllCalls）不消费 Redirs 事实，`echo x > /dev/sda` 与
// `dd of=/dev/sda`（destructive 档）同破坏面分两档。攻击样本前/后对照：
// 修前 echo hi > /dev/sda → Destructive=false, risk=write；修后 → destructive。

import "testing"

// TestRedirectDeviceWrite_Issue175 攻击样本：重定向写设备须与 dd of= 同档。
func TestRedirectDeviceWrite_Issue175(t *testing.T) {
	cases := []string{
		"echo hi > /dev/sda",
		"cat disk.img > /dev/sda",
		"echo x > /dev/nvme0n1",
		"echo x > /dev/mapper/vg-lv",
		"echo x >> /dev/sda",       // 追加形态同是设备写
		"echo x 2> /dev/sda",       // stderr 通道写设备同判
		"echo x > /dev/sda1",       // 分区节点
		"echo x > /dev/mem",        // 字符设备（内存映像）
		"true && cat x > /dev/sda", // 复合语句里的设备写不漏
		"(echo x > /dev/sda)",      // 子 shell 里的设备写（全 AST 事实面）
		"echo x > /dev/sda # trailing comment",
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false, risk=%s——重定向设备写落 write 档（tools#175）", cmd, d.Risk())
			continue
		}
		if d.Risk() != RiskDestructive {
			t.Errorf("%q Risk=%s，应为 destructive（tools#175）", cmd, d.Risk())
		}
	}
}

// TestRedirectDeviceWriteOverlong_Issue175 超长路径（头/尾窗 best-effort）的
// 重定向设备写同样命中：Redirs 事实面在超长路径上与 AllCalls 同法收集。
func TestRedirectDeviceWriteOverlong_Issue175(t *testing.T) {
	cmd := ""
	for len(cmd) <= MaxCommandLen {
		cmd += "# padding line for overlong window\n"
	}
	cmd += "echo x > /dev/sda"
	d := Analyze(cmd, nil)
	if !d.Destructive {
		t.Fatalf("超长命令的重定向设备写判 Destructive=false（Unevaluated=%v）——窗口路径漏判（tools#175）", d.Unevaluated)
	}
}

// TestRedirectHarmlessSinkNotDestructive_Issue175 对照：无害汇点不得误报
// ——`> /dev/null` 等日常形态仍走 write 档，不进二次确认门。
func TestRedirectHarmlessSinkNotDestructive_Issue175(t *testing.T) {
	cases := []string{
		"echo x > /dev/null",
		"ls 2> /dev/null",
		"echo x > /dev/stdout",
		"echo x > /dev/stderr",
		"echo x > /dev/tty",
		"echo x > /dev/pts/0",
		"echo x > /dev/fd/3",
		"echo x > /dev/shm/tmpfile",
		"echo x > /dev/urandom",
		"echo x > ./localfile",
		"cat < /dev/sda", // 读设备不是写设备
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if d.Destructive {
			t.Errorf("%q 判 Destructive=true——无害汇点/读侧重定向被误报（tools#175）", cmd)
		}
	}
}

// TestRedirectDynamicDeviceTargetBoundary_Issue175 动态目标不可静态判定：
// 不入设备写判定（与 Invocation.Dynamic 同法），仍由只读门保守拒为 write 档
// ——Redirs 事实面留给调用方细分（tools#175 修法选项 1 的文档化边界）。
func TestRedirectDynamicDeviceTargetBoundary_Issue175(t *testing.T) {
	d := Analyze("echo x > $DEV", nil)
	if d.Destructive {
		t.Fatal("动态目标判 Destructive=true 与文档边界不符")
	}
	if d.ReadOnly {
		t.Fatal("动态目标的重定向仍应保守拒为非只读")
	}
}

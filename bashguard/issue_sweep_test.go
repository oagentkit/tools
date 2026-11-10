package bashguard

// 2026-11-11 issue 清零战回归测试（#125）：每条用例钉一个失效形态，
// 标题带 issue 号——回归时按号检索。

import "testing"

// TestDestructiveScanIndependentOfReadOnlyBreak_Issue125 不可逆判定吃全量
// AllCalls，不被只读判定的提前 break 截断——前置触礁语句（touch/echo 不在
// 只读表）不得藏住后置的 rm -rf /。
func TestDestructiveScanIndependentOfReadOnlyBreak_Issue125(t *testing.T) {
	cases := []struct{ cmd string }{
		{"touch /tmp/a && rm -rf /"},
		{"echo x > out && mkfs.ext4 /dev/sda"},
		{"git push && dd if=/dev/zero of=/dev/sda"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——不可逆门静默不触发（tools#125）", c.cmd)
		}
	}
}

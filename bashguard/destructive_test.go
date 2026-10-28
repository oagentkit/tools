package bashguard

import (
	"strings"
	"testing"
)

// ── 不可逆判定：命令名 + flag + 参数三档 ──

// TestDestructiveVerdict 钉住三档判定的核心表。
//
// 「只读 / 写 / 不可逆」是三件事：rm file 不是只读但**可逆**（还能捞回来），
// rm -rf / 不可逆。判错方向就是命令真跑了。
func TestDestructiveVerdict(t *testing.T) {
	cases := []struct {
		cmd         string
		destructive bool
	}{
		// —— 只读：既不写也不可逆 ——
		{"ls -la", false},
		{"echo hi", false},
		{"git status", false},
		// —— 写但可逆 ——
		{"rm file.txt", false},
		{"mv a b", false},
		{"chmod 644 file", false},
		{"chown user file", false},
		// —— 不可逆：递归删除 ——
		{"rm -rf /", true},
		{"rm -r dir", true},
		{"rm -R dir", true},
		{"rm --recursive dir", true},
		{"rm --recursive=always dir", true},
		// —— 不可逆：写设备 ——
		{"dd if=/dev/zero of=/dev/sda", true},
		{"mkfs.ext4 /dev/sda1", true},
		{"mkfs /dev/sda", true},
		{"mke2fs /dev/sda", true},
		{"mkswap /dev/sda2", true},
		// —— 不可逆：关机 ——
		{"shutdown -h now", true},
		{"reboot", true},
		{"poweroff", true},
		// —— 不可逆：子命令级 ——
		{"systemctl disable nginx", true},
		{"systemctl mask nginx", true},
		{"kubectl delete pod foo", true},
		// —— 不可逆：递归改权限 / 强杀 / 清日志 ——
		{"chmod -R 755 dir", true},
		{"chown -R user dir", true},
		{"kill -9 1234", true},
		{"journalctl --vacuum-time=7d", true},
		// —— 不是不可逆的 systemctl 形态 ——
		{"systemctl status nginx", false},
		{"kubectl get pods", false},
		{"journalctl --since=1h", false},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if d.Destructive != c.destructive {
			t.Errorf("%q：不可逆应为 %v，实得 %v（原因 %q）", c.cmd, c.destructive, d.Destructive, d.DestructiveReason)
		}
	}
}

// TestDestructiveIndependentOfReadOnly 钉住两条判定**正交**。
//
// 不可逆的命令必然不是只读，但**不是只读**不代表不可逆（rm file 可逆）。
// 把两者合成一档，就会把「可逆的写」也报成不可逆——过度确认同样是坏结果。
func TestDestructiveIndependentOfReadOnly(t *testing.T) {
	d := Analyze("rm file.txt", nil)
	if d.ReadOnly {
		t.Fatal("rm 不是只读")
	}
	if d.Destructive {
		t.Fatal("删单个文件可逆，不该报不可逆")
	}

	d = Analyze("rm -rf /", nil)
	if d.ReadOnly {
		t.Fatal("rm -rf 不是只读")
	}
	if !d.Destructive {
		t.Fatal("rm -rf 不可逆")
	}
	if d.DestructiveReason == "" {
		t.Fatal("不可逆必须给出依据（审计要能回答「凭什么说它不可逆」）")
	}
}

// TestDestructiveThroughWrapper 钉住包装剥壳。
//
// env rm -rf / 的不可逆性在 rm 上，不在 env 上——只看最外层命令名会漏。
func TestDestructiveThroughWrapper(t *testing.T) {
	d := Analyze("env rm -rf /", nil)
	if !d.Destructive {
		t.Fatalf("包装形态也要判出不可逆，实得 %+v", d)
	}
}

// TestDestructiveReasonMentionsEvidence 钉住依据可读。
//
// 审计要回答「凭什么说它不可逆」——依据里必须有命令名与命中的 flag/参数。
func TestDestructiveReasonMentionsEvidence(t *testing.T) {
	d := Analyze("rm -rf /", nil)
	if !strings.Contains(d.DestructiveReason, "rm") || !strings.Contains(d.DestructiveReason, "-r") {
		t.Fatalf("依据应含命令名与命中的 flag，实得 %q", d.DestructiveReason)
	}

	d = Analyze("dd if=/dev/zero of=/dev/sda", nil)
	if !strings.Contains(d.DestructiveReason, "dd") || !strings.Contains(d.DestructiveReason, "of=") {
		t.Fatalf("依据应含命令名与命中的参数，实得 %q", d.DestructiveReason)
	}
}

// TestDestructiveCustomRule 钉住调用方能自带规则表。
//
// 内置表是起步集；各宿主的危险命令不一样，表必须是可扩展的。
func TestDestructiveCustomRule(t *testing.T) {
	tbl := DefaultTable()
	tbl.Destructive["mytool"] = Policy{Destructive: true}

	d := Analyze("mytool --force deploy", tbl)
	if !d.Destructive {
		t.Fatalf("自建规则应生效，实得 %+v", d)
	}

	// 自建表不影响默认表
	if Analyze("mytool --force deploy", nil).Destructive {
		t.Fatal("自建规则不得污染缺省表")
	}
}

// TestDestructiveUnknownMeansNotMatched 钉住语义边界。
//
// Destructive=false 只意味着「没命中已知规则」，**不意味着安全**——
// 未知命令仍按非只读处理（两档各自的边界）。
func TestDestructiveUnknownMeansNotMatched(t *testing.T) {
	d := Analyze("some-unknown-tool --wipe", nil)
	if d.Destructive {
		t.Fatal("未知命令不该被判不可逆（只是没命中已知规则）")
	}
	if d.ReadOnly {
		t.Fatal("未知命令仍按非只读处理")
	}
}

// TestDestructiveStillDetectedWhenArgsDynamic 钉住动态参数不影响判定。
//
// rm -rf $DIR 的目标不可静态判定，但**动作形态**（递归删除）是确定的——
// 不可逆性来自形态，不是来自目标路径。
func TestDestructiveStillDetectedWhenArgsDynamic(t *testing.T) {
	d := Analyze("rm -rf $TARGET", nil)
	if !d.Destructive {
		t.Fatalf("递归形态应判出不可逆，实得 %+v", d)
	}
}

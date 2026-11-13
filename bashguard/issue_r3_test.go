package bashguard

// 2026-11-13 第三轮清零回归（#158 #159 #162）：#134/#117 的残留形态。
// 每条用例钉一个失效形态，标题带 issue 号——回归时按号检索。

import "testing"

// TestDestructiveEvalJoinsAllOperands_Issue158 eval 把全部参数用空格拼接后
// 整体求值（POSIX：eval a b c ≡ eval "a b c"）——多操作数无引号形态必须与
// 单操作数引号包裹形态同判（此前只取首个操作数，-rf / 整体丢掉）。
func TestDestructiveEvalJoinsAllOperands_Issue158(t *testing.T) {
	cases := []struct{ cmd string }{
		{"eval rm -rf /"},
		{"eval rm -rf /data"},
		{"eval mkfs.ext4 /dev/sda"},
		{"eval dd if=/dev/zero of=/dev/sda"},
		// 对照：单操作数引号包裹形态（#134 已修，成对钉住）
		{"eval 'rm -rf /'"},
		{"eval 'mkfs.ext4 /dev/sda'"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——eval 操作数整体是命令行（tools#158）", c.cmd)
		}
	}
	// 对照：无害内核不被误判。
	for _, cmd := range []string{"eval echo hello", "eval git status"} {
		if d := Analyze(cmd, nil); d.Destructive {
			t.Errorf("%q 不应误判 destructive（tools#158 对照）", cmd)
		}
	}
}

// TestDestructiveCombinedShortFlagCValue_Issue159 联合短旗里的 -c 载荷必须
// 定位得到：bash -lc / sh -lc / su -lc 按 POSIX 即 -l -c，-c 的值同样是
// 命令行——此前只认 -c 打头的拼写，#134 的绕过形态在最常用变体下原样存活。
func TestDestructiveCombinedShortFlagCValue_Issue159(t *testing.T) {
	cases := []struct{ cmd string }{
		{"bash -lc 'rm -rf /'"},
		{"bash -ec 'rm -rf /'"},
		{"sh -lc 'rm -rf /'"},
		{"su -lc 'rm -rf /'"},
		{"zsh -lc 'mkfs.ext4 /dev/sda'"},
		// 对照：既有形态保持命中
		{"bash -c 'rm -rf /'"},
		{"bash --login -c 'rm -rf /'"},
		{"su -c 'rm -rf /'"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——联合短旗 -c 载荷（tools#159）", c.cmd)
		}
	}
	// 对照：无害载荷不误判（联合短旗定位到了值，值本身无害）。
	for _, cmd := range []string{"bash -lc 'ls -la /tmp'", "sh -c 'ls -la /tmp'"} {
		if d := Analyze(cmd, nil); d.Destructive {
			t.Errorf("%q 不应误判 destructive（tools#159 对照）", cmd)
		}
	}
}

// TestDestructiveSSHInnerShellText_Issue162 ssh 登记 InnerCmd：内层命令与
// 裸命令同效——与 docker exec 用例成对（同族只差表登记）。
func TestDestructiveSSHInnerShellText_Issue162(t *testing.T) {
	cases := []struct{ cmd string }{
		{"ssh host 'rm -rf /data'"},
		{"ssh host 'rm -rf /'"},
		{"ssh -o X=1 host 'mkfs.ext4 /dev/sda'"},
		// 多词形态（未引号包裹的内层命令段）
		{"ssh host rm -rf /data"},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——ssh 内层与裸命令同效（tools#162）", c.cmd)
		}
	}
	// 对照 #117 已修形态保持命中（成对钉住）。
	if d := Analyze("docker exec web rm -rf /data", nil); !d.Destructive {
		t.Error("docker exec 对照形态回归（tools#117）")
	}
	// 对照：无害内层不误判。
	for _, cmd := range []string{"ssh host 'ls -la'", "ssh host echo hi"} {
		if d := Analyze(cmd, nil); d.Destructive {
			t.Errorf("%q 不应误判 destructive（tools#162 对照）", cmd)
		}
	}
	// rsync --delete 同批探针形态：目标侧删除面 = 不可逆。
	if d := Analyze("rsync -a --delete src/ dst/", nil); !d.Destructive {
		t.Error("rsync --delete 应判不可逆（tools#162 探针 F4）")
	}
	if d := Analyze("rsync -a src/ dst/", nil); d.Destructive {
		t.Error("无 --delete 的 rsync 不应误判 destructive")
	}
}

package bashguard

// tools#183 回归钉：统一派发器解包单源（UnwrapDispatch）。guardx#181 的
// splitCommand 只覆盖 eval 与「shell + 连续选项簇」两族，以下形态不解包、
// 外壳包一层后内层命令在段面上不可见（写档判定被外壳漂白）：
//   - `bash --rcfile x -c 'cmd'`（选项间夹裸 operand）
//   - `env bash -c 'cmd'` / `sudo bash -c 'cmd'`（前缀派发）
// 修后三形态全部下钻；bashguard 自身判定面（Analyze 只读/不可逆两档）对同
// 三形态本就有内核兜底，一并钉住防止回归。深度上限 3 沿用 guardx#181。

import (
	"reflect"
	"testing"
)

// segsEqual 段集对照（含顺序：原段在前、内层按解包序并入——消费方按序匹配
// 的前提是产出确定）。
func segsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("段集不符\n got: %#v\nwant: %#v", got, want)
	}
}

// TestUnwrapDispatchIssueForms_Issue183 三形态对照表：修前不解包（guardx 侧
// 已钉 --rcfile 形态保持原样），修后内层脚本体独立成段。
func TestUnwrapDispatchIssueForms_Issue183(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want []string
	}{
		// 形态①：bash 选项簇夹裸 operand（--rcfile x 是 bash 的 operand，
		// 不再卡住 -c 脚本体的定位）
		{"rcfile 夹裸 operand", `bash --rcfile x -c 'git push --force origin main'`,
			[]string{`bash --rcfile x -c 'git push --force origin main'`, `git push --force origin main`}},
		// 形态②③：前缀派发
		{"env bash -c", `env bash -c 'rm -rf /data'`,
			[]string{`env bash -c 'rm -rf /data'`, `rm -rf /data`}},
		{"sudo bash -c", `sudo bash -c 'rm -rf /data'`,
			[]string{`sudo bash -c 'rm -rf /data'`, `rm -rf /data`}},
		// 前缀旗带值 / 自带赋值 / 链式叠加
		{"sudo 值旗", `sudo -u nobody bash -c 'rm -rf /data'`,
			[]string{`sudo -u nobody bash -c 'rm -rf /data'`, `rm -rf /data`}},
		{"nohup 前缀", `nohup bash -c 'echo x > /dev/sda'`,
			[]string{`nohup bash -c 'echo x > /dev/sda'`, `echo x > /dev/sda`}},
		{"setsid 前缀", `setsid bash -c 'dd of=/dev/sda'`,
			[]string{`setsid bash -c 'dd of=/dev/sda'`, `dd of=/dev/sda`}},
		{"前缀链 + 赋值 + 含 c 选项簇", `sudo -u nobody env FOO=1 bash -lc 'git push --force'`,
			[]string{`sudo -u nobody env FOO=1 bash -lc 'git push --force'`, `git push --force`}},
		// 既有已覆盖形态随单源一并钉住：路径 shell / 含 c 选项簇 / eval 多操作数
		{"路径 shell", `/bin/bash -c 'rm -rf /data'`,
			[]string{`/bin/bash -c 'rm -rf /data'`, `rm -rf /data`}},
		{"eval 多操作数拼接", `eval rm -rf /data`,
			[]string{`eval rm -rf /data`, `rm -rf /data`}},
		// 复合命令：原段逐段保留，派发器段并入内层
		{"复合命令拆段", `git status && sudo bash -c 'git push --force origin main'`,
			[]string{`git status`, `sudo bash -c 'git push --force origin main'`, `git push --force origin main`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segsEqual(t, UnwrapDispatch(tc.cmd), tc.want)
		})
	}
}

// TestUnwrapDispatchDepthLimit_Issue183 深度上限 3（沿用 guardx#181）：第 3 层
// 解包后剩余段保持原样不再下钻——`eval eval eval eval 'echo deep'` 的最内层
// "echo deep" 不作为独立段出现。
func TestUnwrapDispatchDepthLimit_Issue183(t *testing.T) {
	if MaxDispatchDepth != 3 {
		t.Fatalf("深度上限应为 3（沿用 guardx#181）: %d", MaxDispatchDepth)
	}
	segsEqual(t, UnwrapDispatch(`eval eval eval eval 'echo deep'`), []string{
		`eval eval eval eval 'echo deep'`,
		`eval eval eval 'echo deep'`,
		`eval eval 'echo deep'`,
		`eval 'echo deep'`,
	})
}

// TestUnwrapDispatchNonDispatchStays_Issue183 非派发形态原样保留（不解包、
// 不多并段）：allow 侧恒失配语义不依赖解包，deny 侧整段前缀匹配照常。
func TestUnwrapDispatchNonDispatchStays_Issue183(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"grep -c 不是派发", `grep -c pattern file`},
		{"shell 无含 c 旗标", `bash --rcfile x setup`},
		{"裸脚本 operand", `bash script.sh`},
		{"sudo 内核非派发器", `sudo ls -la /data`},
		{"-- 终止后 -c 是 operand", `bash -- -c 'rm -rf /data'`},
		{"引号成组不误判派发头", `echo 'bash -c "rm -rf /"'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segsEqual(t, UnwrapDispatch(tc.cmd), []string{tc.cmd})
		})
	}
	segsEqual(t, UnwrapDispatch(""), []string{})
	segsEqual(t, UnwrapDispatch("   "), []string{})
}

// TestAnalyzeIssueFormsKernelScan_Issue183 判定面对照：bashguard 自身的只读/
// 不可逆两档对三形态本就有内核兜底（InnerFlagValue 定位 -c 载荷、kernelScan
// 对未注册调度器全 token 扫描）——外壳不得把内层写动作漂白成普通档。
func TestAnalyzeIssueFormsKernelScan_Issue183(t *testing.T) {
	for _, cmd := range []string{
		`bash --rcfile x -c 'rm -rf /data'`,
		`env bash -c 'rm -rf /data'`,
		`sudo bash -c 'rm -rf /data'`,
		`nohup bash -c 'rm -rf /data'`,
		`setsid bash -c 'rm -rf /data'`,
	} {
		d := Analyze(cmd, nil)
		if d.ReadOnly {
			t.Fatalf("%q 不得判只读（bash 不在只读表）", cmd)
		}
		if !d.Destructive {
			t.Fatalf("%q 的内层 rm -rf 必须命中不可逆档（外壳不得漂白）: %s", cmd, d.DestructiveReason)
		}
	}
}

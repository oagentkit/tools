package bashguard

import (
	"strings"
	"testing"
)

// TestSedScriptReadOnlyGate 回归（tools#81）：sed 脚本串里的 w/W/e/r/R——无论
// 由裸操作数、行地址还是 -e/--expression 承载——此前被当不透明数据整体放行
// （只读档内可写任意文件、s///e 可执行命令，实测文件真被写出）。现在脚本
// 必须整体过受限文法（s///[gpiIMm0-9] / y/// / p / d + 数字/$//re/地址），
// 文法外一律判非只读；-f/--file 的脚本内容静态不可证，直接拒。
func TestSedScriptReadOnlyGate(t *testing.T) {
	unsafe := []struct{ cmd, wantReason string }{
		// issue 复现四条 + 行地址形态
		{"sed -n 'w /tmp/x' /etc/hosts", "写文件"},
		{"sed -n '/localhost/w /tmp/x' /etc/hosts", "写文件"},
		{"sed -e 'w /tmp/x' /etc/hosts", "写文件"},
		{"sed '1w /tmp/x' /etc/hosts", "写文件"},
		{"sed '1e touch /tmp/x' /etc/hosts", "执行 shell 命令"},
		// 其余写/执行原语
		{"sed '/re/W /tmp/x' f", "写文件"},
		{"sed '1r /etc/passwd' f", "注入外部文件"},
		{"sed 's/a/b/e' f", "s///e 执行"},
		{"sed 's/a/b/w /tmp/x' f", "s///w 写文件"},
		{"sed 's/a/b/gw /tmp/x' f", "s///w 写文件"},
		{"sed -e 's/a/b/' -e 'e id' f", "执行 shell 命令"},
		{"sed -n 's/a/b/' -e '1w /tmp/x' f", "写文件"},
		{"sed --expression='w /tmp/x' f", "写文件"},
		{"sed -- 'w /tmp/x' f", "写文件"},
		{"env sed -n 'w /tmp/x' f", "写文件"}, // 安全包装剥壳后同判
		// -f/--file：脚本内容来自文件，静态不可证
		{"sed -f script.sed f", "来自文件"},
		{"sed --file=script.sed f", "来自文件"},
		// 文法外形态（保守拒，宁可误报）
		{"sed '1~2p' f", "不在受限只读文法"},      // ~ 步进地址
		{"sed -n '/x/{p}' f", "不在受限只读文法"}, // {} 块
		{"sed '2q' f", "不在受限只读文法"},
		{"sed -n 'p;=' f", "不在受限只读文法"},
		{"sed 'y/ab/xy' f", "未闭合"},
		{"sed 's/unclosed' f", "未闭合"},
		{"sed -e 's/a/' -e 'w /tmp/x' f", "未闭合"}, // 前段独立即未闭合
	}
	for _, tc := range unsafe {
		d := Analyze(tc.cmd, nil)
		if d.ReadOnly {
			t.Errorf("%q 不应判只读（sed 脚本面漏判）", tc.cmd)
			continue
		}
		if !strings.Contains(d.Reason, tc.wantReason) {
			t.Errorf("%q: 原因 %q 不含 %q", tc.cmd, d.Reason, tc.wantReason)
		}
	}

	readonly := []string{
		// issue 指定的对照：不能把整个 sed 判死
		"sed -n '1,2p' /etc/hosts",
		"sed -n '10,20p' main.go",
		// 受限文法内的常见只读脚本
		"sed 's/a/b/' f",
		"sed 's,a,b,g' f",    // 任意分隔符
		"sed 's/a\\/b/x/' f", // 转义分隔符
		"sed 's/./x/3' f",    // 数字 flag
		"sed 'y/abc/xyz/' f",
		"sed '/re/!d' f",
		"sed '1!d' f",
		"sed '1 ! d' f", // 地址/!/命令间空白
		"sed '$d' f",
		"sed -n '/foo/,$p' f",
		"sed -n '1,/bar/p' f",
		"sed '1,2d;3,4p' f",
		"sed -n 'p;p' f",
		"sed -n '1,2 p' f",
		"sed -e 's/a/b/' -e 'p' f",
		"sed -- 's/a/b/' f",
		"sed '# just comment' f",
		"sed '' f",                      // 空脚本=无命令
		"sed -n 'p' /tmp/new-world.txt", // 输入文件路径含 w 字母不是脚本命令
		"sed -n -e 's/a/b/' f1 f2",      // 其余裸操作数是输入文件
	}
	for _, cmd := range readonly {
		if d := Analyze(cmd, nil); !d.ReadOnly {
			t.Errorf("%q 应判只读: %s", cmd, d.Reason)
		}
	}
}

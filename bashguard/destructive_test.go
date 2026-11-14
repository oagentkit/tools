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

// TestDestructiveScanConsumesAllCalls 回归（tools#44）：不可逆扫描曾消费判定
// 路径提前返回的 Invocations——首个非只读语句触发 Analyze 的 break 后，其后
// 的调用根本不进扫描序列，前置触礁语句（未知命令/写重定向/动态展开/控制流）
// 可稳定藏住后置的 rm -rf /，等于给确认门禁开旁路。改扫全量 AllCalls 后
// issue 复现五例全部命中。
func TestDestructiveScanConsumesAllCalls(t *testing.T) {
	for _, cmd := range []string{
		"unknowncmd; rm -rf /",           // 未知命令触礁 → break，Invocations 截断
		"ls > /tmp/x; rm -rf /",          // 写重定向在 evalCall 之前 return，Invocations 为空
		"echo $(date); rm -rf /",         // 参数动态展开触礁
		"if true; then rm -rf /; fi",     // 控制流直接拒、不下探
		"for f in *; do rm -rf $f; done", // for 形态同上
	} {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 应判不可逆（全量 AllCalls 扫描）: %+v", cmd, d)
		}
	}
}

// TestRiskTierSingleSource 档位单源（tools#48）：per-tool 风险注册表与命令层
// 判定共用同一档位体系——档位从**一次** Decision 推导（Decision.Risk），
// 消费方不再自建平行枚举与命令层判定各判各的（双源漂移面归零）。
func TestRiskTierSingleSource(t *testing.T) {
	cases := []struct {
		cmd  string
		want RiskTier
	}{
		{"ls -la", RiskReadOnly},
		{"git status", RiskReadOnly},
		{"rm file.txt", RiskWrite},              // 写但可逆
		{"some-unknown-tool --wipe", RiskWrite}, // 未知命令：非只读、未命中不可逆
		{"rm -rf /", RiskDestructive},
		{"shutdown -h now", RiskDestructive},
		{strings.Repeat("a", MaxCommandLen+1), RiskUnevaluated}, // 超长未评估
		{"echo 'unclosed", RiskUnevaluated},                     // 解析失败未评估
	}
	for _, c := range cases {
		if got := Analyze(c.cmd, nil).Risk(); got != c.want {
			t.Errorf("Analyze(%q).Risk() = %q, want %q", c.cmd, got, c.want)
		}
	}
	// 超长但窗口命中不可逆：按更强的信号报不可逆档（Unevaluated 并存）
	d := Analyze(strings.Repeat("# padding comment\n", 800)+"rm -rf /data", nil)
	if d.Risk() != RiskDestructive || !d.Unevaluated {
		t.Fatalf("超长窗口命中不可逆应报不可逆档且保留未评估事实: %+v", d)
	}
}

// TestRiskTierSharedTableRegistration per-tool 注册面（tools#48）：宿主的工具
// 专属不可逆规则注册进同一张 Table（Destructive 面）即参与全部档位判定
// （含 AllCalls 扫描与内核兜底）——不需要在表外再叠一层平行判定。
func TestRiskTierSharedTableRegistration(t *testing.T) {
	tbl := DefaultTable()
	// 宿主工具的专属形态：堡垒机 agent 的 force-release 子命令恒不可逆
	tbl.Destructive["jumpctl"] = Policy{Subcommands: map[string]Policy{
		"force-release": {Destructive: true},
	}}

	for _, c := range []struct {
		cmd  string
		want RiskTier
	}{
		{"jumpctl force-release prod", RiskDestructive},      // 注册规则直接生效
		{"sudo jumpctl force-release prod", RiskDestructive}, // 内核兜底同样吃到注册规则
		{"jumpctl status", RiskWrite},                        // 未命中子命令：写档
	} {
		if got := Analyze(c.cmd, tbl).Risk(); got != c.want {
			t.Errorf("Analyze(%q).Risk() = %q, want %q", c.cmd, got, c.want)
		}
	}
	// 注册不影响缺省表（副本契约，既有 TestDestructiveCustomRule 同款）
	if Analyze("jumpctl force-release prod", nil).Destructive {
		t.Fatal("自建规则不得污染缺省表")
	}
}

// TestOverlongCommandNotFailOpen 回归（tools#45）：超长命令此前直接
// Destructive=false——一条 10K+1 字节、前面塞满注释、结尾 rm -rf 的命令据此
// 绕过不可逆二次确认（「未扫描」无任何显式标记）。现在：Unevaluated=true
// 显式标记「未评估」（调用方 fail-closed），且头/尾窗 best-effort 扫描把
// 实测攻击形态的 rm -rf 照常判出。
func TestOverlongCommandNotFailOpen(t *testing.T) {
	// 尾部 payload（issue 实测攻击形态：垃圾铺垫在前）：尾窗命中
	tail := strings.Repeat("# padding comment\n", 800) + "rm -rf /data"
	if len(tail) <= MaxCommandLen {
		t.Fatalf("测试输入应超长: %d", len(tail))
	}
	d := Analyze(tail, nil)
	if !d.Unevaluated || !d.Destructive {
		t.Fatalf("超长尾部 rm -rf 应未评估且判出不可逆: %+v", d)
	}

	// 头部 payload（payload 在前、铺垫在后）：头窗命中
	head := "rm -rf /data\n" + strings.Repeat("# padding comment\n", 800)
	d = Analyze(head, nil)
	if !d.Unevaluated || !d.Destructive {
		t.Fatalf("超长头部 rm -rf 应未评估且判出不可逆: %+v", d)
	}

	// 纯垃圾超长：无命中——但必须显式「未评估」，调用方按保守侧处理
	d = Analyze(strings.Repeat("a", MaxCommandLen+1), nil)
	if d.ReadOnly || !d.Unevaluated || d.Destructive {
		t.Fatalf("超长纯垃圾应非只读+未评估+无命中: %+v", d)
	}

	// 正常长度命令不带未评估标记；解析失败同样属未评估
	if d := Analyze("ls -la", nil); d.Unevaluated {
		t.Fatalf("正常命令不应标记未评估: %+v", d)
	}
	if d := Analyze("echo 'unclosed", nil); !d.Unevaluated {
		t.Fatalf("解析失败应标记未评估: %+v", d)
	}
}

// TestDestructiveThroughUnregisteredDispatcher 回归（tools#44）：全量集合不剥
// 包装——sudo/doas 这类不在 Commands 表的调度器，内核命令是参数不是语法调用，
// 扫描须按内核再判一次（sudo/xargs 形态）；已注册命令（echo）的参数是数据，
// 不参与内核兜底（不误报）。
func TestDestructiveThroughUnregisteredDispatcher(t *testing.T) {
	for _, cmd := range []string{
		"sudo rm -rf /data",    // 内核在首位
		"sudo -n rm -rf /data", // 跳过调度器前置 flag 定位内核
		"doas chmod -R 755 /data",
	} {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 应按内核判出不可逆: %+v", cmd, d)
		}
	}
	// echo 已注册：参数是数据，内核兜底不得把 echo rm 当真调用
	if d := Analyze("echo rm -rf /", nil); d.Destructive {
		t.Errorf("echo 的参数是数据，不参与内核兜底: %+v", d)
	}
	// 未注册调度器 + 无不可逆内核：不误报
	if d := Analyze("sudo ls -la", nil); d.Destructive {
		t.Errorf("sudo ls 无不可逆内核，不应误报: %+v", d)
	}
}

// TestDestructiveThroughSchedulerValueFlags 回归（tools#100）：sudo -u/-g、
// doas -u 这类**带值**前置旗——旗的值（nobody/docker/root）曾被称为内核候选，
// 查不到规则即整体放行，真正的内核 rm 再也轮不到判定；同族命令只差一个
// `-u <user>` 就从 destructive（要确认）掉到 write（可直跑）。
func TestDestructiveThroughSchedulerValueFlags(t *testing.T) {
	for _, cmd := range []string{
		"sudo -u nobody rm -rf /", // 对照：sudo rm -rf / 本就判出
		"sudo -u nobody mkfs.ext4 /dev/sdb1",
		"sudo -g docker dd if=x of=/dev/sda",
		"doas -u root mkfs.ext4 /dev/sdb1",
	} {
		if d := Analyze(cmd, nil); !d.Destructive {
			t.Errorf("%q 应判不可逆（旗值不得顶替内核）: %+v", cmd, d)
		}
	}
	// 反证：旗值形态 + 无不可逆内核照旧不误报
	for _, cmd := range []string{
		"sudo -u nobody ls -la",
		"sudo -g docker ps -a",
	} {
		if d := Analyze(cmd, nil); d.Destructive {
			t.Errorf("%q 无不可逆内核，不应误报: %+v", cmd, d)
		}
	}
}

// TestDestructiveThroughWrapperUnregisteredFlags 回归（tools#101）：注册过的
// 包装（env/timeout）遇到表外前置 token（GNU env -S、POSIX env -/env --、
// timeout --kill-after）时剥壳提前 break，rest[0] 变成 -S/--/10，判不出内核；
// 而 destructiveKernel 又因包装已注册而不兜底 → 不可逆漏判。
func TestDestructiveThroughWrapperUnregisteredFlags(t *testing.T) {
	for _, cmd := range []string{
		"env rm -rf /",        // 对照：注册形态本就判出
		"timeout 10 rm -rf /", // 对照：ConsumeBare 吃时长本就判出
		"env -S 'rm -rf /'",   // GNU env -S：值本身就是命令行
		"env - rm -rf /",      // POSIX env -：忽略环境
		"env -- rm -rf /",     // POSIX env --：选项终止
		"env -S 'rm -rf' /",   // -S 值与后续参数拼接
		"timeout --kill-after=5 10 rm -rf /",
		"timeout -k5 10 rm -rf /", // -k5 联合内联形态
	} {
		if d := Analyze(cmd, nil); !d.Destructive {
			t.Errorf("%q 应判不可逆（表外前置旗不得吞内核）: %+v", cmd, d)
		}
	}
	// 反证：表外前置旗 + 无不可逆内核照旧不误报
	for _, cmd := range []string{
		"env -S 'ls -la'",
		"env - ls -la",
		"timeout --kill-after=5 10 ls -la",
	} {
		if d := Analyze(cmd, nil); d.Destructive {
			t.Errorf("%q 无不可逆内核，不应误报: %+v", cmd, d)
		}
	}
	// 内核可判时走精确递归，不进兜底：内核后的参数是内核的数据
	//（env ls rm -rf / 的 rm 是 ls 的操作数不是内核，不误报）
	if d := Analyze("env ls rm -rf /", nil); d.Destructive {
		t.Errorf("内核可判时后续参数是数据，不应误报: %+v", d)
	}
}

// TestDestructiveFindBatchDelete 回归（tools#107）：find 的批量删除形态
// （-delete / -exec rm -rf）与同效果的 rm -rf 必须同档（destructive）——
// 此前 find 登记在只读表、不在不可逆表，判定链两处对「已注册命令」早退，
// -exec 段里的 rm -rf 整体漏出（Risk=write 免确认直接执行）。
//
// 负例同钉：-exec 段里是只读内核（grep）不得整段误报——保守多报的边界
// 在「内核不可逆」，不在「带 -exec」。
func TestDestructiveFindBatchDelete(t *testing.T) {
	cases := []struct {
		cmd         string
		destructive bool
	}{
		// —— 漏报面（tools#107 主张）：批量删除形态 ——
		{"find . -delete", true},
		{"find / -name '*.log' -delete", true},
		{"find . -exec rm -rf {} +", true},
		{"find . -exec rm -rf {} \\;", true},
		{"find . -exec rm -fr {} +", true},    // 联合短 flag 换拼写
		{"find . -execdir rm -rf {} +", true}, // 换子目录为工作目录，破坏面更隐蔽
		{"find . -ok rm -rf {} \\;", true},
		{"find . -name '*.log' -exec rm -rf {} \\;", true},
		// 包装不洗白内核：-exec 段经调度器再进 find
		{"sudo find . -delete", true},
		{"xargs find . -exec rm -rf {} +", true},
		// —— 负例：-exec 段内核只读 / find 纯查找 ——
		{"find . -name '*.log'", false},
		{"find . -maxdepth 2 -type f", false},
		{"find . -exec grep -r TODO {} \\;", false},
		{"find . -exec rm {} \\;", false}, // rm 无递归 flag：同 rm 自身判据（单删可逆）
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if d.Destructive != c.destructive {
			t.Errorf("%q：不可逆应为 %v，实得 %v（原因 %q）", c.cmd, c.destructive, d.Destructive, d.DestructiveReason)
		}
		if c.destructive {
			if got := d.Risk(); got != RiskDestructive {
				t.Errorf("%q：Risk 应为 destructive，实得 %s", c.cmd, got)
			}
		}
	}
}

// TestDestructiveThroughRemoteWrappers 回归（tools#117）：容器/命名空间/切用户/
// 瞬态服务类「在别处执行」包装器的内层命令与裸命令同效同档——
// docker exec web rm -rf /data 与 rm -rf /data 必须同判 destructive。
// 此前这类包装器不在判定链上（docker 不在 Commands 表走 kernelScan 兜底但
// 内层取不到；kubectl 登记在不可逆表只认 delete 子命令，exec 早退；chroot/
// systemd-run 的内层是绝对路径 /bin/rm，kernelScan 只认裸命令名；su -c 的
// 内核是 shell 文本），同一效果两档判定。
//
// 前 6 例是名单内包装器对照组（tools#44/#100/#101 已判到，不得退化）；后 7 例
// 是 issue 复现的漏报形态；再后是各家族变体（内层换绝对路径/换不可逆命令/
// 叠调度器——包装不得洗白内核，两层都要判到）。
func TestDestructiveThroughRemoteWrappers(t *testing.T) {
	cases := []struct {
		cmd         string
		destructive bool
	}{
		// —— 对照组：名单内包装器（既有判定，正例不得退化）——
		{"rm -rf /data", true},
		{"sudo -n rm -rf /data", true},
		{"xargs rm -rf", true},
		{"env rm -rf /data", true},
		{"nohup rm -rf /data", true},
		{"timeout 5 rm -rf /data", true},
		// —— issue #117 复现的 7 行漏报形态 ——
		{"docker exec web rm -rf /data", true},
		{"podman exec web rm -rf /data", true},
		{"kubectl exec pod-1 -- rm -rf /data", true},
		{"chroot /host /bin/rm -rf /data", true},
		{"nsenter -t 1 -m -- rm -rf /data", true},
		{"su - root -c 'rm -rf /data'", true},
		{"systemd-run --unit=x /bin/rm -rf /data", true},
		// —— 家族变体 ——
		{"docker run alpine rm -rf /data", true},       // run：image 之后的操作数
		{"podman run alpine rm -rf /data", true},       // podman run 同型
		{"docker exec -u root web rm -rf /data", true}, // 前置旗值不得顶替内核
		{"kubectl exec -c agent pod-1 -- rm -rf /data", true},
		{"kubectl exec pod-1 -- mkfs.ext4 /dev/sda", true},         // 内层换不可逆命令
		{"chroot /host rm -rf /data", true},                        // 内层裸命令名
		{"nsenter -t 1 -m rm -rf /data", true},                     // 缺 -- 兜底形态
		{"su root -c 'rm -rf /data'", true},                        // 无 login 破折号
		{"su -c 'rm -rf /data'", true},                             // 最短形态
		{"systemd-run --unit=x rm -rf /data", true},                // 内层裸命令名
		{"systemd-run --unit=x /usr/bin/mkfs.ext4 /dev/sda", true}, // 绝对路径 + 前缀族规则
		{"sudo docker exec web rm -rf /data", true},                // 外层调度器不洗白内层
		{"docker exec web sudo rm -rf /data", true},                // 内层再包调度器
		// —— 反证：内层非不可逆内核不得误报 ——
		{"docker exec web grep -r x /", false},
		{"podman exec web grep -r x /", false},
		{"kubectl exec pod-1 -- grep -r x /", false},
		{"chroot /host grep -r x /", false},
		{"nsenter -t 1 -m -- grep -r x /", false},
		{"su -c 'grep -r x /'", false},
		{"systemd-run --unit=x grep -r x /", false},
		{"docker exec web ls -la", false},      // 只读内核
		{"docker exec web rm file.txt", false}, // rm 无递归 flag：可逆，同裸 rm 判据
		{"docker run alpine rm file.txt", false},
		{"docker ps", false}, // 无内层载荷的子命令
		{"su root", false},   // 交互 shell，无 -c 命令文本
		{"kubectl get pods", false},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if d.Destructive != c.destructive {
			t.Errorf("%q：不可逆应为 %v，实得 %v（原因 %q）", c.cmd, c.destructive, d.Destructive, d.DestructiveReason)
			continue
		}
		if c.destructive && d.Risk() != RiskDestructive {
			t.Errorf("%q：Risk 应为 destructive，实得 %s", c.cmd, d.Risk())
		}
	}
}

// TestDestructiveGitFamily 回归（tools#107 同表缺口）：git 家族的不可逆形态
// ——历史改写/强推/强制清理。生态共识已有（下游 jumpserver-kit 自维护的
// destructiveGit 同判），上游缺位。
func TestDestructiveGitFamily(t *testing.T) {
	cases := []struct {
		cmd         string
		destructive bool
	}{
		// —— 不可逆 ——
		{"git reset --hard HEAD~1", true},
		{"git reset --hard", true},
		{"git clean -fdx", true},
		{"git clean -fd", true},
		{"git clean -f", true},
		{"git clean --force", true},
		{"git push --force origin main", true},
		{"git push -f origin main", true},
		{"git push --force-with-lease origin main", true},
		{"git filter-branch -- --all", true},
		// 全局前置旗不歪子命令定位（-C/-c 吃参后子命令照常落位）
		{"git -C /repo push --force origin main", true},
		{"git -c core.hooksPath=/dev/null reset --hard", true},
		// —— 写但可逆 / 只读 ——
		{"git status", false},
		{"git log --oneline", false},
		{"git reset --soft HEAD@{1}", false},
		{"git clean -n", false}, // dry-run
		{"git push origin main", false},
		// tools#174 起 branch -D 属引用删除类——「reflog 捞回」的豁免判法被
		// issue 推翻：reflog 有有效期，强删未合并分支不可逆
		{"git branch -D feature", true},
		{"git branch -d feature", true},
	}
	for _, c := range cases {
		d := Analyze(c.cmd, nil)
		if d.Destructive != c.destructive {
			t.Errorf("%q：不可逆应为 %v，实得 %v（原因 %q）", c.cmd, c.destructive, d.Destructive, d.DestructiveReason)
		}
	}
}

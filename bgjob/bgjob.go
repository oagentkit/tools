package bgjob

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/oekit/pkg/stringx"
)

// Session 宿主既有 SSH 会话的最小收窄（tools#106）：bgjob 只需要「跑一条
// 命令、拿 stdout 与退出码」。宿主各自适配（jumpserver-exec 的 session、
// golang.org/x/crypto/ssh 的 Session 等各自包一层）；本包不做连接管理。
type Session interface {
	// Exec 在远端 shell 执行 command，返回 stdout 与退出码。stderr 由宿主
	// 自行处置——本原语的启动/探测/清理输出全部走 stdout。
	Exec(command string) (stdout string, code int, err error)
}

// State 任务状态（探测结论，marker 协议的三态）。
type State string

const (
	// StateRunning 任务在跑：marker 未落、输出文件在场（输出文件由 Start
	// 同步建出，任务未起不会误报 running）。
	StateRunning State = "running"
	// StateFinished 任务已结束：marker 已落，退出码见 Status 的第二返回值。
	StateFinished State = "finished"
	// StateRemoved 已收尾：输出与 marker 均不在（Cleanup 之后的稳态）。
	StateRemoved State = "removed"
)

// 探测/清理命令的协议字面（逐行识别，容忍宿主会话往 stdout 混入横幅类噪音）。
const (
	lineFinished    = "BGJOB_FINISHED"     // 后跟空格与退出码（cat marker 的内容）
	lineRunning     = "BGJOB_RUNNING"      // marker 未落且输出在场
	lineRemoved     = "BGJOB_REMOVED"      // 双缺：已收尾 / 清理自证通过
	lineCleanFailed = "BGJOB_CLEAN_FAILED" // 清理后 [ -e ] 自证仍有文件
)

// Job 一个远程后台作业（Start 构造；零值不可用）。
type Job struct {
	// OutputPath 输出文件（远端绝对路径，调用方经 WithOutputPath 给定）：
	// 任务的 stdout/stderr 落此文件。
	OutputPath string
	// MarkerPath 退出码 marker = OutputPath + ".rc." + 任务身份（Go 侧单源
	// 一次算定，契约见 doc.go 第 3 条）。身份每次 Start 唯一（tools#161）：
	// 同路径复用时上一轮**在飞**任务的 wrapper 在新 Start 之后才写回，固定
	// 路径下旧退出码会落成新任务的终态凭证——身份分开后旧写回落旧路径，
	// 本任务的探测/清理只认自己的路径。
	MarkerPath string

	sess Session
}

// StartOption 启动选项（XxxOption 命名纪律，AGENTS.md）。
type StartOption func(*startConfig)

type startConfig struct {
	outputPath string
}

// WithOutputPath 指定远端输出文件路径（**必填**、远端绝对路径）：任务的
// stdout/stderr 落此文件，退出码 marker 为其 + ".rc." + 本次任务身份
// （markerPathOf 单源，tools#161）。输出重定向由调用方给——本原语不做
// 日志轮转、不替调用方选路径（提案边界声明）。
func WithOutputPath(path string) StartOption {
	return func(c *startConfig) { c.outputPath = path }
}

// Start 经既有会话在远端起脱离任务（setsid + nohup + 内层 shell + 外层
// marker），命令立即返回——等待/轮询是调用方对 Status 的事。
//
// 结构即协议（doc.go 契约 2）：
//
//		umask 077 && mkdir -p 'DIR' && : > 'OUT' && { setsid nohup sh -c 'INNER' > 'OUT' 2>&1 </dev/null & }
//		INNER = sh -c 'USERCMD'; echo $? > 'MARKER'
//
//	  - setsid + nohup + </dev/null：新会话、抗 HUP、stdin 摘钩——SSH 会话关闭
//	    不带走任务（脱离会话的判据，宿主断连不影响任务存活）；
//	  - 用户命令在内层 shell（sh -c 'USERCMD'）里跑，marker 由**外层** wrapper
//	    写——内层任何 exit 只死自己，$? 照常落 marker（裸 exit 不会永远 RUNNING）；
//	  - `: > 'OUT'` 同步建输出文件：Start 返回即在场——探测的 REMOVED 判据
//	    （输出+marker 双缺）不会在任务尚未起来时误报；
//	  - mkdir/建文件与 spawn 同链但 spawn 花括号后台化：前置步骤失败时 Exec
//	    退出码非零，Start 如实报错（若整链后台化，失败会被吞成启动成功）；
//	  - umask 077：输出与 marker 0600 起（共享主机上的路径卫生）。
//
// 用户命令与路径经 shellQuote 单源包裹后拼进命令串，**不做**任何转义改写：
// cmd 在远端 shell 里逐字执行（含管道/重定向/引号——那是调用方的命令本体）。
func Start(sess Session, cmd string, opts ...StartOption) (*Job, error) {
	if sess == nil {
		return nil, errors.New("bgjob: Session 为 nil")
	}
	if strings.TrimSpace(cmd) == "" {
		return nil, errors.New("bgjob: 命令为空")
	}
	var cfg startConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if cfg.outputPath == "" {
		return nil, errors.New("bgjob: 缺少 WithOutputPath（输出重定向由调用方给，见 WithOutputPath）")
	}
	if !filepath.IsAbs(cfg.outputPath) {
		return nil, fmt.Errorf("bgjob: 输出路径须为远端绝对路径: %q", cfg.outputPath)
	}
	job := &Job{OutputPath: cfg.outputPath, MarkerPath: markerPathOf(cfg.outputPath), sess: sess}
	out, code, err := sess.Exec(startCommand(cmd, job.OutputPath, job.MarkerPath))
	if err != nil {
		return nil, fmt.Errorf("bgjob: 启动失败: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("bgjob: 启动命令退出码 %d: %s", code, strings.TrimSpace(out))
	}
	return job, nil
}

// Status 探测任务状态：marker 在场 = finished（+ 退出码）；否则输出文件在场
// = running；双缺 = removed。探测失败（连接错/退出码非零/输出不可识别）如实
// 上抛——状态判不了就是判不了，不猜。
//
// 退出码仅在 finished 时非空（marker 内容首字段，`echo $?` 的十进制形态；
// 信号杀死为 128+N）。
func (j *Job) Status() (state State, exitCode string, err error) {
	out, code, err := j.sess.Exec(probeCommand(j.OutputPath, j.MarkerPath))
	if err != nil {
		return "", "", fmt.Errorf("bgjob: 探测失败: %w", err)
	}
	if code != 0 {
		return "", "", fmt.Errorf("bgjob: 探测命令退出码 %d: %s", code, strings.TrimSpace(out))
	}
	return parseProbe(out)
}

// Cleanup 就地收尾：RUNNING 拒绝（任务还在写输出，且文件原样——契约 4 的
// 验收项）；FINISHED 后 rm -f 输出与 marker，删完 [ -e ] 自证 REMOVED 才报
// (true, nil)；REMOVED 幂等（(false, nil)）。自证不过（rm 后文件仍在）如实
// 报错——「报已清理」必须是事实。
//
// Status→rm 之间存在固有的窄竞态（探测后任务恰好结束）：拒绝保护的是
// 「RUNNING 强删」的常态事故，终态边界上的一次性收尾不在此防线内。
func (j *Job) Cleanup() (removed bool, err error) {
	state, _, err := j.Status()
	if err != nil {
		return false, err
	}
	switch state {
	case StateRunning:
		return false, fmt.Errorf("bgjob: 任务仍在运行（输出文件 %s 在写），拒绝清理", j.OutputPath)
	case StateRemoved:
		return false, nil
	}
	out, code, err := j.sess.Exec(cleanupCommand(j.OutputPath, j.MarkerPath))
	if err != nil {
		return false, fmt.Errorf("bgjob: 清理失败: %w", err)
	}
	if code != 0 {
		return false, fmt.Errorf("bgjob: 清理命令退出码 %d: %s", code, strings.TrimSpace(out))
	}
	for _, l := range strings.Split(out, "\n") {
		switch strings.TrimSpace(l) {
		case lineRemoved:
			return true, nil
		case lineCleanFailed:
			return false, fmt.Errorf("bgjob: 清理自证失败（rm 后文件仍在）: %s", j.OutputPath)
		}
	}
	return false, fmt.Errorf("bgjob: 清理输出不可识别（协议漂移?）: %q", snap(out))
}

// startCommand 组启动命令（结构即协议，见 Start）。
// 启动链先 rm -f 上一轮残留的 marker（tools#135）：marker 是终态唯一凭证，
// 同路径复用 OutputPath（固定日志路径是常态）时旧 marker 让新任务被探测
// 立即判 FINISHED（旧退出码）、Cleanup 把仍在写的输出当终态删掉。
// rm -f 只能清 Start 时刻**已落盘**的形态：上一轮任务还在飞时，旧 wrapper 的
// `echo $? > MARKER` 在新 Start 之后才执行，固定路径下照样把旧退出码写成新
// 任务的终态凭证——「新任务的 marker 只能由新任务的 wrapper 写出」由此保证
// 靠的是 marker 路径带任务身份（markerPathOf，tools#161），rm -f 是身份
// 碰撞与跨版本残留的纵深。
func startCommand(cmd, outPath, markerPath string) string {
	inner := "sh -c " + shellQuote(cmd) + "; echo $? > " + shellQuote(markerPath)
	return "umask 077 && mkdir -p " + shellQuote(filepath.Dir(outPath)) +
		" && rm -f " + shellQuote(markerPath) +
		" && : > " + shellQuote(outPath) +
		" && { setsid nohup sh -c " + shellQuote(inner) +
		" > " + shellQuote(outPath) + " 2>&1 </dev/null & }"
}

// markerPathOf marker 路径单源（doc.go 契约 3）：输出路径 + ".rc." + 本次
// Start 的任务身份（crypto/rand 文本 ≥128 位，跨进程/跨会话碰撞可忽略）。
// 身份进文件名是 tools#161 的修法：在飞旧任务的写回落旧路径，新任务的
// 探测/清理只认自己的路径——固定路径下旧 wrapper 的迟到写回会被新任务
// 认领为终态（误报 FINISHED + Cleanup 删仍在写的输出）。
func markerPathOf(outputPath string) string {
	return outputPath + ".rc." + rand.Text()
}

// probeCommand 组探测命令：marker 在场 = 终态（printf 前缀 + cat 拼一行，
// 退出码随行返回）；输出在场 = running（Start 同步建文件，未起不误报）；
// 双缺 = removed。
func probeCommand(outPath, markerPath string) string {
	return "if [ -f " + shellQuote(markerPath) + " ]; then printf '" + lineFinished +
		" '; cat " + shellQuote(markerPath) +
		"; elif [ -e " + shellQuote(outPath) + " ]; then printf '" + lineRunning + "\\n'; else printf '" + lineRemoved + "\\n'; fi"
}

// cleanupCommand 组清理命令：rm 后 [ -e ] 自证（契约 4）——删完才是删掉。
func cleanupCommand(outPath, markerPath string) string {
	return "rm -f " + shellQuote(outPath) + " " + shellQuote(markerPath) +
		" && if [ -e " + shellQuote(outPath) + " ] || [ -e " + shellQuote(markerPath) +
		" ]; then printf '" + lineCleanFailed + "\\n'; else printf '" + lineRemoved + "\\n'; fi"
}

// parseProbe 逐行解析探测输出（BGJOB_ 前缀行——容忍宿主会话在 stdout 混入
// 横幅/告警行，协议字面行照常识别）。
func parseProbe(stdout string) (State, string, error) {
	for _, l := range strings.Split(stdout, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == lineRunning:
			return StateRunning, "", nil
		case l == lineRemoved:
			return StateRemoved, "", nil
		case strings.HasPrefix(l, lineFinished+" "):
			fields := strings.Fields(strings.TrimPrefix(l, lineFinished+" "))
			if len(fields) == 0 {
				return "", "", fmt.Errorf("bgjob: marker 无退出码: %q", l)
			}
			return StateFinished, fields[0], nil
		}
	}
	return "", "", fmt.Errorf("bgjob: 探测输出不可识别（协议漂移?）: %q", snap(stdout))
}

// shellQuote POSIX 单引号转义单源：路径/命令进远端 shell 前逐字包裹。单引号
// 内除单引号本身外全部字面——含空格/引号/$ 的路径逐字抵达远端；唯一转义点
// 收敛在这一个函数（契约 3 的「单源」）。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// snap 错误信息里的输出快照（防超长噪音进错误文本）。截断走矩阵同款
// stringx.TruncBytes（tools#165）：按字节切片曾把多字节字符劈半，中文输出
// >200 字节后尾部必现 U+FFFD——预算仍是 200 字节，落点回退到 rune 边界。
func snap(s string) string {
	const snapBudget = 200
	out, truncated := stringx.TruncBytes(s, snapBudget)
	if truncated {
		return out + "…"
	}
	return out
}

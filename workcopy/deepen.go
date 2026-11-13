// deepen.go 浅克隆工作副本的按需历史补全（收编自 argus internal/gitdeepen，
// v0.10.39）。
//
// 背景：依赖 merge-base 的 range 审查在 workcopy 的"默认分支浅克隆 + head
// checkout"下必挂（"cannot find merge-base"）。全量 `git fetch --unshallow`
// 把完整历史拉进关键路径，慢且重。本文件先本地探测 merge-base 可达性，
// 不可达再 `--deepen` 指数加深（64 起步逐轮 ×2）：多数 PR 一两轮即收敛，
// 代价从"全仓历史"降到"base↔head 的实际距离"。
//
// refspec 保持显式全量：clone --depth 1 隐含 --single-branch，裸 fetch 只补
// 默认分支的远端引用，PR base 非默认分支时 origin/<base> 不存在（真件实测
// 复现）。deepen 不移除 .git/shallow 标记——守卫因此从"标记存在"改为
// "merge-base 可达探测"，已补全的工作副本（标记可能仍在）探测通过即零开销。
package workcopy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oagentkit/guardx/logredact"
	"github.com/oagentkit/runtime/procx"
)

// fullRefspec 显式全量 refspec（--single-branch 克隆下唯一可靠的补全入口）。
const fullRefspec = "+refs/heads/*:refs/remotes/origin/*"

// deepenStart / maxDeepenRounds / probeTimeout 指数加深参数缺省值：64 起步
// 逐轮 ×2，8 轮覆盖 8k+ 提交；真实上限由时间预算兜底——大仓耗尽预算即报错
// 降级（审查主链路不阻断）。要按仓规模调整（单体大仓放宽轮数、CI 紧张收紧）
// 用 DeepenOptions + EnsureMergeBaseOpts（oagentkit/tools#36）。
const (
	deepenStart     = 64
	maxDeepenRounds = 8
	probeTimeout    = 30 * time.Second
)

// DeepenOptions 加深循环的可调参数（零值字段取包级缺省）。
type DeepenOptions struct {
	// StartDepth 起步加深层数（--deepen=<N> 首轮值；缺省 64）。
	StartDepth int
	// MaxRounds 加深轮数上限（每轮层数 ×2；缺省 8）。
	MaxRounds int
	// ProbeTimeout 单轮 merge-base 探测超时（缺省 30s）。
	ProbeTimeout time.Duration
	// Token deepen fetch 的鉴权 token（tools#160）：经进程级
	// `-c http.extraheader` 注入（authGitArgs 同款，不落 .git/config）——
	// tools#124 把 clone/PR fetch/refresh 三条路径改成无凭证 URL 后，deepen
	// 是唯一没注入凭证的 fetch，而 config 里的 origin URL 已按契约恒为无凭证
	// 形态：私有远端浅克隆加深必 401，EnsureMergeBase 每次报错降级、range
	// 审查必然丢失。调用方从 Pool.CredentialOf 解析后经 WithToken 传入（与
	// prepare/refresh 同一凭证源）。零值 = 无鉴权面（公有远端 / file:// fixture）。
	Token string
}

// DeepenOption EnsureMergeBase/EnsureMergeBaseTLS 的可选项（XxxOption 命名
// 纪律，AGENTS.md）：对 DeepenOptions 的函数式补充；EnsureMergeBaseOpts
// 直接吃 DeepenOptions 结构体，不重复本形态。
type DeepenOption func(*DeepenOptions)

// WithToken 深度补全的鉴权 token（tools#160）：deepen fetch 经进程级
// `-c http.extraheader` 注入 Basic 凭证（token 不进 URL、不落 .git/config，
// 与包头承诺一致）。
func WithToken(token string) DeepenOption {
	return func(o *DeepenOptions) { o.Token = token }
}

// applyDeepenOpts 折叠可选项（nil 项跳过，容忍误传）。
func applyDeepenOpts(opts []DeepenOption) DeepenOptions {
	var o DeepenOptions
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}

func (o DeepenOptions) startDepth() int {
	if o.StartDepth > 0 {
		return o.StartDepth
	}
	return deepenStart
}

func (o DeepenOptions) maxRounds() int {
	if o.MaxRounds > 0 {
		return o.MaxRounds
	}
	return maxDeepenRounds
}

func (o DeepenOptions) probeTimeout() time.Duration {
	if o.ProbeTimeout > 0 {
		return o.ProbeTimeout
	}
	return probeTimeout
}

// EnsureMergeBase 确保 origin/<baseBranch> 与 HEAD 的 merge-base 可达。
//   - 探测通过（完整克隆、已补全、或 base 与 head 重合等价场景）零 fetch 返回；
//   - 探测失败且 .git/shallow 存在 → deepen 指数加深循环（每轮后重探）；
//   - 探测失败但非浅克隆 → 静默返回：merge-base 失败另有原因（base 分支缺失、
//     非 git 目录等），交由后续 OCR 执行报真实错误——与旧"无 shallow 标记即
//     跳过"语义一致；
//   - 加深轮失败/预算耗尽仍不可达 → 报错降级。整条消息过 logredact（git 传输
//     失败回显带凭证 clone URL——ocrshim 通道此错误经 MCP 原样回传 argus）。
//
// 私有远端经可选项 WithToken 注入鉴权（tools#160）。
func EnsureMergeBase(ctx context.Context, dir, baseBranch string, timeout time.Duration, opts ...DeepenOption) error {
	return ensureMergeBase(ctx, dir, baseBranch, timeout, false, applyDeepenOpts(opts))
}

// EnsureMergeBaseTLS EnsureMergeBase 的 TLS 形态：insecureTLS=true 时对 deepen
// fetch 注入 GIT_SSL_NO_VERIFY（与 Pool.runGit 同款纪律——自签 https 平台的
// clone/PR fetch 均走该环境，deepen fetch 漏注会让补全在此类平台确定性失败
// 且静默降级丢失 range 审查；第八轮审计 C 级修复）。
func EnsureMergeBaseTLS(ctx context.Context, dir, baseBranch string, timeout time.Duration, insecureTLS bool, opts ...DeepenOption) error {
	return ensureMergeBase(ctx, dir, baseBranch, timeout, insecureTLS, applyDeepenOpts(opts))
}

// EnsureMergeBaseOpts EnsureMergeBase 的参数化形态（oagentkit/tools#36）：
// DeepenOptions 调起步层数/轮数上限/单轮探测超时——单体大仓放宽（万级提交
// 8 轮可能不够）、CI 资源紧张的宿主收紧；零值字段取包级缺省。整体时间预算
// 仍由 timeout 统一兜底。鉴权 token 同在 DeepenOptions.Token（tools#160）。
func EnsureMergeBaseOpts(ctx context.Context, dir, baseBranch string, timeout time.Duration, opts DeepenOptions) error {
	return ensureMergeBase(ctx, dir, baseBranch, timeout, false, opts)
}

func ensureMergeBase(ctx context.Context, dir, baseBranch string, timeout time.Duration, insecureTLS bool, opts DeepenOptions) error {
	if baseBranch == "" {
		return errors.New("gitdeepen: 缺少 base 分支")
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	base := "origin/" + baseBranch

	if probeMergeBase(ctx, dir, base, opts.probeTimeout()) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "shallow")); err != nil {
		return nil // 完整克隆：merge-base 失败非浅历史问题
	}
	// 鉴权参数（tools#160）：#124 把三条 git 远程命令中的两条改成进程级注入后，
	// deepen 是唯一漏注的一条——config 的 origin URL 此时已按契约改成无凭证
	// 形态，deepen 拿不到任何凭证，私有远端必 401。scheme 从 origin URL 反推
	//（refresh 同款——file:// fixture 无鉴权面，authGitArgs 返回 nil）；无
	// token 时不发额外 git 调用，公有远端路径零开销。
	var auth []string
	if opts.Token != "" {
		scheme := "https://"
		if out, _, _, err := procx.Run(ctx, procx.RunRequest{
			Argv: []string{"git", "-C", dir, "remote", "get-url", "origin"}, Timeout: 10 * time.Second,
		}); err == nil {
			scheme = schemeOf(strings.TrimSpace(out))
		}
		auth = authGitArgs(scheme, opts.Token)
	}
	for i := range opts.maxRounds() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		fetchReq := procx.RunRequest{
			Argv: append(append([]string{"git"}, auth...),
				"-C", dir, "fetch",
				"--deepen="+strconv.Itoa(opts.startDepth()<<i),
				"origin", fullRefspec),
			Timeout: remaining,
		}
		if insecureTLS {
			fetchReq.Env = []string{"GIT_SSL_NO_VERIFY=true"}
		}
		if _, stderr, _, err := procx.Run(ctx, fetchReq); err != nil {
			return errors.New(logredact.Redact(fmt.Sprintf(
				"浅克隆加深失败（git fetch --deepen）: %v；stderr: %s", err, tail(stderr, 400))))
		}
		if probeMergeBase(ctx, dir, base, opts.probeTimeout()) {
			return nil
		}
	}
	return errors.New(logredact.Redact(fmt.Sprintf(
		"浅克隆加深后 merge-base 仍不可达（origin/%s ↔ HEAD，%s 内 deepen %d 轮）——OCR range 审查降级",
		baseBranch, timeout.Round(time.Second), opts.maxRounds())))
}

// probeMergeBase 本地探测 merge-base 可达（无网络 IO，秒级）。exit 非零一律视为
// 不可达（含 base 远端引用不存在——由后续 deepen 轮经全量 refspec 创建）。
func probeMergeBase(ctx context.Context, dir, base string, timeout time.Duration) bool {
	_, _, _, err := procx.Run(ctx, procx.RunRequest{
		Argv:    []string{"git", "-C", dir, "merge-base", base, "HEAD"},
		Timeout: timeout,
	})
	return err == nil
}

// tail 取 stderr 尾部（诊断用，限长防爆）。
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

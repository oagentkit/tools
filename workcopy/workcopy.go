// Package workcopy Git 工作副本沙箱服务。
// 浅克隆 base 默认分支 + fetch PR head + checkout，供 cli 插件在真实工作树上执行。
// 凭证经进程级 `-c http.extraheader` 注入（tools#124）：token 不进 clone URL、
// 不落 .git/config——config 里的 origin URL 恒为无凭证形态（`git clone <带
// userinfo 的 URL>` 会把 URL 逐字写进 config，而目录按包头承诺保留至 TTL
// Sweep，CLI agent 在工作树里 `cat .git/config` 即可取走宿主凭证）。
// 代价与边界：`-c` 参数在 git 进程执行窗口内经 argv 可见（同 UID /proc），
// 秒级窗口远小于原形态的小时/天级持久落盘面。singleflight 防并发重复 clone。
// 引用归零后目录**保留**（供同 PR 下次审查增量复用，省 base 分支重克隆），
// TTL 由 Sweep 兜底回收。
package workcopy

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oagentkit/guardx/logredact"
	"github.com/oagentkit/runtime/procx"
	"github.com/oekit/concurrency/singleflight"
	"github.com/oekit/logx"
)

// WorktreeKey 工作副本标识（平台/仓库/PR/SHA）。
type WorktreeKey struct {
	Platform      string
	Owner         string
	Repo          string
	Number        string
	HeadSHA       string
	DefaultBranch string
}

func (k WorktreeKey) String() string {
	return fmt.Sprintf("%s/%s/%s#%s@%s", k.Platform, k.Owner, k.Repo, k.Number, k.HeadSHA)
}

// Pool 工作副本沙箱池。
type Pool struct {
	Root string
	Log  logx.Logger

	// CredentialOf 返回 (cloneBaseURL, token)：由调用方按平台注入。
	CredentialOf func(platform string) (baseURL, token string, ok bool)
	// InsecureTLSOf 平台是否免 TLS 校验。
	InsecureTLSOf func(platform string) bool

	mu sync.Mutex
	// entries 当前登记的工作副本（key → 条目）。
	entries map[string]*wcEntry
	// inflight 领用刷新中的目录集合（tools#108）：claimStale 摘牌即登记、
	// refreshClaim 返回（成功重挂/失败回落两路）注销。刷新窗口内条目不在
	// entries，目录对「只按 entries 构建保护集合」的 Sweep 孤儿扫描不可见，
	// ModTime 过 TTL 即被 os.RemoveAll——正在刷新的工作副本被静默删掉，
	// fetch 半路报错、Ensure 回落整次全新浅克隆。保护集合跟着领用走：
	// Sweep 把在飞目录并入 live。
	inflight map[string]bool
	group    singleflight.Flight[string]
}

type wcEntry struct {
	key      WorktreeKey // 当前 checkout 的 key（同 PR 换 head 后随 refresh 前移）
	dir      string
	refCount int
	lastUsed time.Time
}

// NewPool 创建工作副本沙箱池。log 为 nil 时走 logx.Nop 静默兜底（tools#103，
// 与 sast.Scan 的 tools#61 修法同一 nil-safe 约定）：logger 是可选形象，传 nil
// 不得 panic——曾建仓成功后的首条日志即 nil 指针解引用（目录已建、条目未登记）。
func NewPool(root string, log logx.Logger) *Pool {
	if log == nil {
		log = logx.Nop()
	}
	return &Pool{Root: root, Log: log, entries: map[string]*wcEntry{}, inflight: map[string]bool{}}
}

// registerRetryMax "登记后丢失"重试上限：条目只可能被 Sweep/refresh 重挂删除，
// 重建一次后必命中；更多次说明存在未知并发竞争，报错优于死循环。
const registerRetryMax = 1

// Ensure 返回就绪的工作副本目录（浅克隆 base + checkout PR head）。
// 同 key 并发调用共享同一目录（引用计数）。
func (p *Pool) Ensure(ctx context.Context, key WorktreeKey) (string, error) {
	if p.CredentialOf == nil {
		return "", fmt.Errorf("workcopy: 未配置 CredentialOf")
	}
	baseURL, token, ok := p.CredentialOf(key.Platform)
	if !ok || token == "" {
		return "", fmt.Errorf("workcopy: 平台 %s 缺少 clone 凭证", key.Platform)
	}
	if key.HeadSHA == "" {
		return "", fmt.Errorf("workcopy: PR %s/%s#%s 无 head SHA", key.Owner, key.Repo, key.Number)
	}
	// Number 同样进 git refspec（refs/pull/<N>/head）：非纯数字（如
	// "1/head:refs/heads/x"）可拼出 src:dst 形态（refs/pull/1/head:refs/heads/
	// x/head），git fetch 会把它当 refspec 在沙箱克隆内写本地引用——与
	// DefaultBranch 的引用名卫生同一防线，此前两行守卫只写了一行（tools#47）。
	// 无 shell 注入面（argv 直传），危害限于克隆内引用伪造，但守卫必须齐。
	// GitHub/GitLab/Gitea 的 PR/MR 号均为十进制，纯数字白名单即合法形态全集。
	if !isPrNumber(key.Number) {
		return "", fmt.Errorf("workcopy: 非法 PR 号 %q（须为纯数字）", key.Number)
	}

	k := key.String()
	if dir, ok := p.hitExisting(k); ok {
		return dir, nil
	}
	// 同 PR 换 head（新推送）：领用引用归零的保留目录做增量刷新——只 fetch 新的
	// PR refspec，base 分支浅对象已在库，省掉整次浅克隆。领用（rc++）阻断 Sweep
	// 与并发二次领用；刷新失败回落全新克隆。
	if claim := p.claimStale(key); claim != nil {
		if dir, ok := p.refreshClaim(ctx, k, key, claim, token); ok {
			return dir, nil
		}
	}
	return p.buildAndRegister(ctx, k, key, baseURL, token)
}

// hitExisting 同 key 快路径：命中即领用（rc++）返回目录。
func (p *Pool) hitExisting(k string) (dir string, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[k]
	if !ok {
		return "", false
	}
	e.refCount++
	e.lastUsed = time.Now()
	return e.dir, true
}

// claimStale 领用同 PR 引用归零的保留目录（换 head 增量刷新候选）。
// 领用即摘牌（rc++ 与 delete 同临界区原子完成）：刷新窗口内旧 key 不可再命中——
// 否则并发 Ensure(旧head) 会拿到正被 checkout --force 重写的工作树（撕裂读），
// 且其 Release 因条目已重挂新 key 而 no-op，rc 永久泄漏、Sweep 永不回收。
// 摘牌的同时登记 in-flight（tools#108）：条目离开了 entries，目录对 Sweep 的
// 保护必须换载体——否则孤儿扫描把在飞刷新当泄漏目录删掉。
func (p *Pool) claimStale(key WorktreeKey) *wcEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.entries {
		if e.refCount <= 0 &&
			e.key.Platform == key.Platform && e.key.Owner == key.Owner &&
			e.key.Repo == key.Repo && e.key.Number == key.Number &&
			e.key.HeadSHA != key.HeadSHA {
			e.refCount++
			delete(p.entries, k)
			p.inflight[e.dir] = true
			return e
		}
	}
	return nil
}

// refreshClaim 对领用的保留目录做增量刷新。ok=false 表示刷新失败（已释放领用，
// 调用方回落全新克隆）。返回即注销 in-flight（tools#108）：成功路径条目已重挂
// entries（保护回归 entries 载体），失败路径目录交回 Sweep/孤儿扫描按 TTL 回收。
func (p *Pool) refreshClaim(ctx context.Context, k string, key WorktreeKey, claim *wcEntry, token string) (dir string, ok bool) {
	defer func() {
		p.mu.Lock()
		delete(p.inflight, claim.dir)
		p.mu.Unlock()
	}()
	if err := p.refresh(ctx, claim.dir, key, token); err != nil {
		// 刷新失败（force push 抹掉旧引用等）→ 回落全新克隆。条目已随领用摘牌，
		// 不回挂——checkout 中途失败的工作树可能撕裂，回挂旧 key 会让后续同
		// head 的 Ensure 静默命中混合态目录（目录交 Sweep/孤儿扫描按 TTL 回收）
		return "", false
	}
	p.mu.Lock()
	if e, dup := p.entries[k]; dup {
		// 竞争窗口内同 key 已被常规建仓登记：既有条目胜出。claim 条目已摘牌
		// （领用即摘牌，刷新期间无并发持有者），整条作废安全
		e.refCount++
		e.lastUsed = time.Now()
		dir = e.dir
		p.mu.Unlock()
		_ = os.RemoveAll(claim.dir)
		return dir, true
	}
	claim.key = key
	claim.lastUsed = time.Now()
	p.entries[k] = claim
	dir = claim.dir
	p.mu.Unlock()
	return dir, true
}

// buildAndRegister 建仓与登记同在 singleflight 内完成（条目 rc=0，引用由 Do 返回后
// 统一加）：共享者退出时若条目已消失（同 flight 先到者的条目被 Sweep 回收或 refresh
// 重挂 key），重走一次建仓（上限 registerRetryMax），杜绝拿到已删除目录。
// oekit Flight.Do（v0.6.0 起，x/sync 退役）：leader 以自身 ctx 执行 prepare（与此前
// 闭包首到者 ctx 语义一致），等待方 ctx 取消时各自提前退出而非干等 leader。
func (p *Pool) buildAndRegister(ctx context.Context, k string, key WorktreeKey, baseURL, token string) (string, error) {
	for round := 0; ; round++ {
		// 返回值不消费：建仓与登记同在 flight 内，Do 返回后统一从 entries 取
		_, err := p.group.Do(ctx, k, func(ctx context.Context) (string, error) {
			dir, err := p.prepare(ctx, key, baseURL, token)
			if err != nil {
				return "", err
			}
			p.mu.Lock()
			defer p.mu.Unlock()
			if e, ok := p.entries[k]; ok { // 双检：并发建仓时保留先到者
				_ = os.RemoveAll(dir)
				return e.dir, nil
			}
			p.entries[k] = &wcEntry{key: key, dir: dir, lastUsed: time.Now()}
			return dir, nil
		})
		if err != nil {
			return "", err
		}

		p.mu.Lock()
		e, ok := p.entries[k]
		if ok {
			e.refCount++
			e.lastUsed = time.Now()
			dir := e.dir
			p.mu.Unlock()
			return dir, nil
		}
		p.mu.Unlock()
		if round >= registerRetryMax {
			return "", fmt.Errorf("workcopy: %s 工作副本登记后丢失（并发释放竞争）", k)
		}
	}
}

// Release 任务结束后释放。引用归零**不再立即删目录**：保留供同 PR 下次审查
// 增量复用（换 head 只 fetch 新 refspec），TTL 由 Sweep 兜底回收。
func (p *Pool) Release(key WorktreeKey) {
	k := key.String()
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[k]
	if !ok {
		return
	}
	e.refCount--
	if e.refCount <= 0 {
		e.lastUsed = time.Now()
	}
}

func prRefspec(platform, number string) string {
	switch strings.ToLower(platform) {
	case "gitlab":
		return fmt.Sprintf("refs/merge-requests/%s/head", number)
	default:
		return fmt.Sprintf("refs/pull/%s/head", number)
	}
}

// isPrNumber PR 号形态守卫：非空纯数字（三大平台的 PR/MR 号都是十进制）。
// prepare 与 refresh 两条路径的 refspec 都由 Ensure 入口的这道守卫覆盖。
func isPrNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// cloneURL 无凭证 clone URL（tools#124）：token 不再嵌 URL——带 userinfo 的
// URL 会被 git 逐字写进 .git/config（remote.origin.url），而目录保留至 TTL
// Sweep，token 即持久落盘。URL 只保留 host/path 形态；鉴权由 authGitArgs 的
// 进程级 -c 参数承载（不落盘）。file:// 直通（本地 fixture 无鉴权面）。
func cloneURL(baseURL, owner, repo, token string) string {
	u := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if strings.HasPrefix(u, "file://") {
		return u
	}
	for _, suffix := range []string{"/api/v1", "/api/v4", "/api/v3", "/api"} {
		u = strings.TrimSuffix(u, suffix)
	}
	// 保留原有 scheme——内网 http://gitea 被强制升 https 会导致克隆失败
	scheme := "https://"
	if i := strings.Index(u, "://"); i >= 0 {
		scheme = u[:i+3]
		u = u[i+3:]
	}
	return fmt.Sprintf("%s%s/%s/%s.git", scheme, u, owner, repo)
}

// authGitArgs http(s) git 操作的鉴权参数（tools#124）：Basic 凭证经
// `http.extraheader` 进程级注入——git 只在本进程命令行里持有 token，不写
// .git/config、不落盘。file:// 与无 http(s) 传输的命令返回 nil（无鉴权面）。
//
// 为什么是 extraheader 而不是 insteadOf/credential helper：insteadOf 仍要把
// 凭证写进某处 config 才能生效（同样落盘）；credential helper 需要外部二进制
// 与 helper 自身的存储纪律。`-c http.extraheader=Authorization: Basic …` 是
// git 文档明确支持的进程级注入形态，argv 里的瞬时可见性（同 UID /proc）远
// 小于原形态的小时/天级持久落盘。
func authGitArgs(scheme, token string) []string {
	if token == "" || (scheme != "http://" && scheme != "https://") {
		return nil
	}
	auth := base64.StdEncoding.EncodeToString([]byte("oauth2:" + token))
	return []string{
		"-c", "http.extraheader=Authorization: Basic " + auth,
	}
}

// schemeOf clone URL 的 scheme 前缀（无 scheme 视作 https——cloneURL 已补齐）。
func schemeOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		return u[:i+3]
	}
	return "https://"
}

func (p *Pool) prepare(ctx context.Context, key WorktreeKey, baseURL, token string) (string, error) {
	// branch 来自外部输入（DefaultBranch）：以 "-" 开头会被 git 当选项解析
	// （`--branch --upload-pack=…` 即 argument injection），含空白/控制字符则
	// 本身不是合法引用名——在建任何目录之前 fail-fast
	if b := key.DefaultBranch; strings.HasPrefix(b, "-") || strings.ContainsAny(b, " \t\r\n\x7f") {
		return "", fmt.Errorf("workcopy: 非法引用名 %q", b)
	}
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return "", fmt.Errorf("workcopy: 根目录创建失败: %w", err)
	}
	dir, err := os.MkdirTemp(p.Root, "wc-")
	if err != nil {
		return "", fmt.Errorf("workcopy: 建沙箱失败: %w", err)
	}
	// 在飞保护（tools#136，tools#108 的另一半）：prepare 的 wc-* 目录从建出
	// 到注册 entries 之间（clone/fetch/checkout 至多 3×5min）既不在 entries
	// 也不在 inflight——TTL 小于一次慢克隆时 Sweep 的孤儿扫描会把它当泄漏
	// 目录删掉（clone 只写 .git/ 子目录，顶层 mtime 停在起点，ModTime 兜底
	// 判不出「正在建」）。登记 inflight，失败/成功两路都注销（defer 原子收口）。
	p.mu.Lock()
	p.inflight[dir] = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.inflight, dir)
		p.mu.Unlock()
	}()
	branch := key.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	url := cloneURL(baseURL, key.Owner, key.Repo, token)
	insecureTLS := p.InsecureTLSOf != nil && p.InsecureTLSOf(key.Platform)
	// 鉴权参数（tools#124）：进程级 -c 注入，token 不进 URL/config。
	auth := authGitArgs(schemeOf(url), token)

	steps := [][]string{
		// clone 支持 `--` 终止选项解析：url/dir 此后恒为位置参数
		append(append([]string{"git"}, auth...), "clone", "--depth", "1", "--branch", branch, "--", url, dir),
		append(append([]string{"git"}, auth...), "-C", dir, "fetch", "--depth", "1", "origin", prRefspec(key.Platform, key.Number)),
		{"git", "-C", dir, "checkout", "--force", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if err := runGit(ctx, args, insecureTLS); err != nil {
			_ = os.RemoveAll(dir)
			// msg 为脱敏载体，token 必须作为 secret 传入（此前参数顺序传反，
			// git 失败详情整体丢失、脱敏形同虚设）
			return "", fmt.Errorf("workcopy: %s#%s 准备失败: %s",
				key.Owner, key.Repo, logredact.RedactSecrets(err.Error(), token))
		}
	}
	p.Log.Info("agentkit.workcopy.ready", "dir", dir, "pr", key.Owner+"/"+key.Repo+"#"+key.Number)
	return dir, nil
}

// refresh 将保留目录增量推进到新 PR head：只 fetch 新的 PR refspec（base 分支
// 浅对象已在库——这是"第二次审查少拉取"的收益来源），checkout --force 与
// prepare 末步同款。origin/<默认分支> 不随刷新前移，base 端新鲜度以首次克隆
// 为界（由保留 TTL 限定窗口）；默认分支大跨度强推场景由调用方全新克隆兜底
// （refresh 失败即回落 prepare）。fetch 鉴权同 prepare（tools#124）：origin
// URL 恒为无凭证形态，token 经进程级 -c 参数临时注入。
func (p *Pool) refresh(ctx context.Context, dir string, key WorktreeKey, token string) error {
	insecureTLS := p.InsecureTLSOf != nil && p.InsecureTLSOf(key.Platform)
	// 刷新路径没有现成 baseURL——从 config 的 origin URL 反推 scheme
	// （file:// fixture 无鉴权面，authGitArgs 返回 nil）。
	out, _, _, oerr := procx.Run(ctx, procx.RunRequest{
		Argv: []string{"git", "-C", dir, "remote", "get-url", "origin"}, Timeout: 10 * time.Second,
	})
	scheme := "https://"
	if oerr == nil {
		scheme = schemeOf(strings.TrimSpace(out))
	}
	auth := authGitArgs(scheme, token)
	steps := [][]string{
		append(append([]string{"git"}, auth...), "-C", dir, "fetch", "--depth", "1", "origin", prRefspec(key.Platform, key.Number)),
		{"git", "-C", dir, "checkout", "--force", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if err := runGit(ctx, args, insecureTLS); err != nil {
			return fmt.Errorf("workcopy: %s#%s 增量刷新失败: %s",
				key.Owner, key.Repo, logredact.RedactSecrets(err.Error(), token))
		}
	}
	p.Log.Info("agentkit.workcopy.refreshed", "dir", filepath.Base(dir), "pr", key.Owner+"/"+key.Repo+"#"+key.Number)
	return nil
}

// gitExecTimeout 单步 git 命令超时（clone/fetch/checkout 各自独立计时）。
const gitExecTimeout = 5 * time.Minute

// runGit 经 procx 进程组纪律执行 git：Setpgid 建组 + 超时 TERM 整组（ssh/askpass
// 孙进程不泄漏）、环境白名单（clone URL 内嵌 token——全量继承会把宿主凭证透传给
// 子进程）、stderr 截尾随错误返回。GIT_TERMINAL_PROMPT=0 禁交互由 procx 统一注入。
func runGit(ctx context.Context, args []string, insecureTLS bool) error {
	var env []string
	if insecureTLS {
		env = append(env, "GIT_SSL_NO_VERIFY=true")
	}
	_, _, _, err := procx.Run(ctx, procx.RunRequest{
		Argv:    args,
		Env:     env,
		Timeout: gitExecTimeout,
	})
	return err
}

// Sweep 清理泄漏的沙箱目录（兜底 TTL 扫描）。
// 只回收引用归零且超 TTL 的条目与孤儿目录——rc>0 的在用目录一律保留
// （长任务超 TTL 时被删即是生产事故），因此 TTL 应配置为大于最长任务时长；
// 在飞刷新的目录（claimStale 已摘牌、refreshClaim 未返回，tools#108）同样
// 一律保留——「TTL 大于最长任务时长」挡不住这个窗口：被领用的目录是上一次
// 克隆留下的，隔天再审早已过 TTL 而刷新本身还在飞。进程崩溃导致的泄漏目录
// （条目随进程消失）由下方孤儿扫描兜底。
func (p *Pool) Sweep(ttl time.Duration) {
	p.mu.Lock()
	var stale []string
	live := make(map[string]bool, len(p.entries)+len(p.inflight))
	for k, e := range p.entries {
		live[e.dir] = true
		if e.refCount <= 0 && time.Since(e.lastUsed) > ttl {
			stale = append(stale, e.dir)
			delete(p.entries, k)
		}
	}
	for d := range p.inflight {
		live[d] = true
	}
	p.mu.Unlock()
	for _, d := range stale {
		_ = os.RemoveAll(d)
		p.Log.Warn("agentkit.workcopy.swept", "dir", filepath.Base(d))
	}
	orphans, _ := filepath.Glob(filepath.Join(p.Root, "wc-*"))
	for _, d := range orphans {
		if live[d] {
			continue
		}
		info, err := os.Stat(d)
		if err != nil || time.Since(info.ModTime()) <= ttl {
			continue
		}
		_ = os.RemoveAll(d)
		p.Log.Warn("agentkit.workcopy.orphan_swept", "dir", filepath.Base(d))
	}
}

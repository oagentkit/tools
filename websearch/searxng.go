package websearch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/oagentkit/guardx/fence"
	"github.com/oekit/pkg/httpjson"
)

// Searxng SearXNG 自建实例客户端（GET /search?format=json——实例 settings.yml 须启用
// formats: [html, json]）。零 API key、零调用成本；商业后端后续按同 Service 接口接入。
type Searxng struct {
	BaseURL     string
	Language    string // 缺省 "zh-CN"；空串=不传 language 参数
	DefaultTopK int    // AsTool 用；≤0 兜底 5
	HTTP        *http.Client
}

// NewSearxng 构造（timeout 为单次检索上限；≤0 用 10s 兜底）。
func NewSearxng(baseURL string, timeout time.Duration) *Searxng {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Searxng{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Language: "zh-CN",
		HTTP:     &http.Client{Timeout: timeout},
	}
}

// searxngResp SearXNG JSON API 响应（只取用到的字段）。
type searxngResp struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// Search 实现 Service。HTTP 非 2xx / 响应非 JSON 一律报错——实例未开
// formats: [json] 时 /search 返回 HTML，报错形态即「响应解析失败」。
func (s *Searxng) Search(ctx context.Context, query string, topK int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("searxng: 空 query")
	}
	if topK <= 0 {
		topK = 5
	}
	hc := s.HTTP
	if hc == nil {
		hc = fallbackHTTP() // 连接复用（此前每次调用现场 new client）
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "json")
	if s.Language != "" {
		q.Set("language", s.Language)
	}
	var parsed searxngResp
	if err := httpjson.DoJSON(ctx, hc, httpjson.Request{
		URL: s.BaseURL + "/search?" + q.Encode(),
	}, &parsed); err != nil {
		return nil, fmt.Errorf("searxng: %w", err)
	}
	out := make([]Result, 0, topK)
	for _, r := range parsed.Results {
		if r.Title == "" || r.URL == "" {
			continue
		}
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

// searchIn web_search 工具入参（utils.InferTool 自动推导 schema）。
type searchIn struct {
	Query string `json:"query" jsonschema:"检索 query，用领域/技术术语（可中英文）"`
}

// AsTool 把检索包成 web_search 内置工具（与 rag.AsTool 同形态）。
// AsTool 把检索包成 web_search 工具。
//
// 结果拼接前所有攻击者可影响字段过结构化消毒（tools#49、tools#63）：检索
// 结果是半可信外网内容（敌意等级高于仓库内内容），未经消毒拼进编号列表，
// 字段内嵌换行即可伪造条目与指令。Title 折叠单行、URL 单行化（控制字符转
// %XX 可见转义）后再过 fence.EscapeUntrusted——「承重保留原样」由消毒的
// 内容保持性兑现（中和结构不删内容），不构成豁免消毒的理由。
//
// ⚠️ panic 契约（oagentkit/rag#31 同族对齐）：schema 反射构造失败时 panic
// （接口签名无 error 通道，「静态 schema 即编程错误」在此升级为显式 API
// 契约声明）。
func (s *Searxng) AsTool() tool.BaseTool {
	t, err := utils.InferTool("web_search",
		"检索公开资料（SearXNG）。返回按相关度排序的结果列表（标题/URL/摘要）。用于概念、方法论类问题的事实核查。",
		func(ctx context.Context, in *searchIn) (string, error) {
			results, err := s.Search(ctx, in.Query, s.DefaultTopK)
			if err != nil {
				// 执行失败返回模型可读文本（agent 可自行降级/换路），不上抛框架
				// 错误通道中止整个 agent 运行——与 rag.AsTool 同一错误契约
				return "检索失败：" + err.Error() + "。可稍后重试或改用已有信息作答。", nil
			}
			if len(results) == 0 {
				return "未检索到相关公开资料。", nil
			}
			// Title/URL/Snippet 都是攻击者可影响的外网内容（SEO 投毒/被抓取
			// 页面），注入 agent 提示词前必须过结构化消毒（tools#49，对齐
			// skillkit#34 的同威胁模型口径）：输出是「N. 标题\n URL\n 摘要」
			// 的编号列表文本，字段内嵌换行即可伪造编号条目与真实 URL 并列、
			// 夹带伪指令。
			// Title 先折叠单行再消毒（skillkit#34 同款）；URL 先单行化（控制
			// 字符转 %XX 可见转义，tools#63：#49 只消毒 Title/Snippet 是残余
			// 面——多行 URL 越出「   <url>」行，后续文本顶格即伪造条目）再消毒
			// ——承重声明由消毒的内容保持性兑现，不豁免。
			var b strings.Builder
			for i, r := range results {
				b.WriteString(strconv.Itoa(i+1) + ". " +
					fence.EscapeUntrusted(strings.Join(strings.Fields(r.Title), " ")) +
					"\n   " + fence.EscapeUntrusted(sanitizeURL(r.URL)) +
					"\n   " + fence.EscapeUntrusted(r.Snippet) + "\n\n")
			}
			return b.String(), nil
		})
	if err != nil {
		panic(fmt.Sprintf("searxng: 构建 web_search 工具失败: %v", err)) // panic 契约见 AsTool doc（rag#31 同族对齐）
	}
	return t
}

// fallbackHTTP nil-HTTP 兜底 client（进程级单例——连接池复用）。
var fallbackHTTP = sync.OnceValue(func() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
})

// sanitizeURL URL 单行化（tools#63）：JSON 字符串合法携带 \n 等控制字符，
// 裸拼进编号列表会折断「   <url>」行，后续文本顶格渲染即伪造条目/伪指令。
// 控制字符（C0/C1、DEL、Unicode 行/段分隔符）转 URL 原生可见转义形态
// （%XX 百分号编码——RFC 3986 中控制字符属非法字符，百分号编码是其唯一
// 合法形态）：单行化的同时保留原文供模型裁量；正形 URL 逐字节不变。
func sanitizeURL(u string) string {
	if !strings.ContainsFunc(u, urlControlRune) {
		return u
	}
	var b strings.Builder
	b.Grow(len(u))
	for _, r := range u {
		if !urlControlRune(r) {
			b.WriteRune(r)
			continue
		}
		for _, c := range []byte(string(r)) {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// urlControlRune 行级渲染中会折行/越界的控制字符判定（含不可见形态：
// C1 与 Unicode 行/段分隔符在部分渲染器同样断行）。
func urlControlRune(r rune) bool {
	switch {
	case r < 0x20 || r == 0x7f: // C0 控制（\r \n \t …）+ DEL
	case r >= 0x80 && r <= 0x9f: // C1 控制（含 U+0085 NEL）
	case r == 0x2028 || r == 0x2029: // Unicode 行/段分隔符
	default:
		return false
	}
	return true
}

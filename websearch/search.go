// Package websearch 公开资料检索抽象：接口可插拔，缺省实现 SearXNG 自建实例
// （免 API key、零调用成本）。与 knowledge/rag 互补——本包面向外网公开资料。
package websearch

import (
	"context"
)

// Result 单条检索结果。
//
// ⚠️ 威胁等级与消毒义务（tools#180）：三个字段都是**半可信外网内容**
// （SEO 投毒 / 被抓取页面的攻击者可控字段，敌意等级高于仓库内内容）。进
// prompt / 编号列表等任何文本拼装面前必须过结构化消毒——Title 折叠单行、
// URL 单行化（sanitizeURL 同款）、各字段过 `guardx/fence.EscapeUntrusted`
// ——或直接使用 Searxng.AsTool（结果拼接已内置消毒，tools#49/#63）。
// 未经消毒原样拼接：字段内嵌换行即可伪造条目与伪指令（与 skillkit#34
// 同威胁模型）。
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Service 检索服务抽象。失败返回 error（调用方按降级收口）；无命中返回空切片 + nil error。
//
// ⚠️ 返回值是**半可信外网内容**（见 Result）：Search 各实现返回原文、不承担
// 消毒（原文语义供调用方按各自输出形态整形）；把 Title/Snippet/URL 拼进
// prompt/编号列表前必须过结构化消毒（`fence.EscapeUntrusted` + URL 单行化），
// 或直接使用 Searxng.AsTool。按包头指引接入新后端（商业搜索 API）时同约束
// ——消毒义务在**拼装出口**，不在实现（tools#180：#49 的修复此前只覆盖
// AsTool 一个出口）。
type Service interface {
	Search(ctx context.Context, query string, topK int) ([]Result, error)
}

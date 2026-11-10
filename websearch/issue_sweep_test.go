package websearch

// 2026-11-11 issue 清零战回归测试（#128）：每条用例钉一个失效形态，
// 标题带 issue 号——回归时按号检索。

import (
	"context"
	"strings"
	"testing"
)

// TestSearchInDescriptionReachesSchema_Issue128 query 参数的 jsonschema
// description 必须真的进 schema——裸标签被 eino 静默丢弃时构造期 panic 守卫
// 兜底，description 键值形态照常生效。
func TestSearchInDescriptionReachesSchema_Issue128(t *testing.T) {
	s := &Searxng{BaseURL: "http://127.0.0.1:1", DefaultTopK: 3}
	// AsTool 内含构造期守卫：description 缺失即 panic——跑通即证明在。
	tk := s.AsTool()
	info, err := tk.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.ParamsOneOf == nil {
		t.Fatal("工具应有参数 schema")
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil || js == nil || js.Properties == nil {
		t.Fatalf("应解出 JSON schema: %v", err)
	}
	if q, ok := js.Properties.Get("query"); !ok || q == nil || !strings.Contains(q.Description, "领域/技术术语") {
		t.Fatalf("query description 应含调用指引，实际 %+v", q)
	}
}

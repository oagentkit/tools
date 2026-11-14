package websearch

// tools#179 回归钉：AsTool 的构造期守卫此前由多层 `&&` 条件包裹，验证链路
// 自身拿不到可判定结论（Info 报错 / info 缺席 / ParamsOneOf 缺席 /
// ToJSONSchema 报错 / Properties 缺席）任一层即**整段静默跳过**——#128 要
// 消灭的静默失效从守卫自身的暗道回来（description 缺失被静默放行）。
// 修后「拿不到可判定结论 = panic」：验证链路自身出错不再静默。

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// mustPanic179 断言 f panic 且 panic 值（string）含 want 子串。
func mustPanic179(t *testing.T, name, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("%s 未 panic——守卫静默放行（tools#179）", name)
		}
		s, ok := r.(string)
		if !ok {
			t.Fatalf("%s panic 值非 string: %v", name, r)
		}
		if want != "" && !strings.Contains(s, want) {
			t.Fatalf("%s panic 文案不含 %q: %s", name, want, s)
		}
	}()
	f()
}

// TestGuardFailClosedWhenChainUndecidable_Issue179 验证链路自身拿不到结论的
// 各条路径：全部 panic（修前静默跳过）。
func TestGuardFailClosedWhenChainUndecidable_Issue179(t *testing.T) {
	mustPanic179(t, "Info 报错", "tools#179", func() {
		guardQueryDescription(nil, errors.New("boom"))
	})
	mustPanic179(t, "info 缺席", "tools#179", func() {
		guardQueryDescription(nil, nil)
	})
	mustPanic179(t, "ParamsOneOf 缺席", "tools#179", func() {
		guardQueryDescription(&schema.ToolInfo{}, nil)
	})
}

// TestGuardQueryDescriptionMissing_Issue179 验证完成但 description 为空：
// tools#128 原始文案照常 panic（语义不变）。
func TestGuardQueryDescriptionMissing_Issue179(t *testing.T) {
	info := &schema.ToolInfo{ParamsOneOf: schema.NewParamsOneOfByParams(
		map[string]*schema.ParameterInfo{
			"query": {Type: schema.String}, // 无 Desc
		})}
	mustPanic179(t, "description 缺失", "tools#128", func() {
		guardQueryDescription(info, nil)
	})
}

// TestGuardHappyPath_Issue179 对照：description 在场 → 零 panic 返回原文。
func TestGuardHappyPath_Issue179(t *testing.T) {
	info := &schema.ToolInfo{ParamsOneOf: schema.NewParamsOneOfByParams(
		map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "检索 query"},
		})}
	if got := guardQueryDescription(info, nil); got != "检索 query" {
		t.Fatalf("守卫应放行并返回 description: %q", got)
	}
}

// TestAsToolGuardEndToEnd_Issue179 真装配路径：Searxng.AsTool 的守卫在真
// Info/ToJSONSchema 链路上拿得到可判定结论（修前静默暗道在此形态不可达，
// 修后断言不 panic 即通过——eino 反射链路稳定时守卫照常工作）。
func TestAsToolGuardEndToEnd_Issue179(t *testing.T) {
	s := NewSearxng("http://127.0.0.1:1", 0)
	_ = s.AsTool() // 未 panic = 验证链路拿得到结论且 description 在场
}

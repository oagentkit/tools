package sast

// tools#177 回归钉：FocusPaths 此前只剔 ".." 段不拒绝对路径——`/etc/passwd`
// 原样通过并作为 semgrep 的位置参数目标，扫描面逃出工作副本（semgrep 的位置
// 参数不受 Dir 约束），与「防路径穿越出工作副本」的注释承诺不符。
// 攻击样本前/后对照：修前 FocusPaths(["/etc/passwd","ok.go"]) → ["/etc/passwd"
// "ok.go"]；修后绝对路径剔除，只剩 ["ok.go"]。

import "testing"

// TestFocusPathsRejectsAbsolutePaths_Issue177 攻击样本：绝对路径（含 Windows
// 盘符形态）一律剔除——扫描面不得越出工作副本。
func TestFocusPathsRejectsAbsolutePaths_Issue177(t *testing.T) {
	got := FocusPaths([]string{
		"/etc/passwd",
		"/home/user/.ssh/id_rsa",
		"C:/Windows/system32/config",
		`C:\Windows\system32\config`,
		"c:\\users\\x\\file.go",
		"a/../b",
		"ok.go",
		"",
		"  /tmp/leak.go  ",
	})
	want := []string{"ok.go"}
	if len(got) != len(want) {
		t.Fatalf("FocusPaths = %v，想要 %v——绝对路径未被剔除（tools#177）", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FocusPaths[%d] = %q，想要 %q（tools#177）", i, got[i], want[i])
		}
	}
}

// TestFocusPathsRelativeLegitKept_Issue177 对照：相对路径不误伤——含 ".."
// 字面的合法文件名（v1..v2.go）、"./" 前缀、普通相对路径照常保留。
func TestFocusPathsRelativeLegitKept_Issue177(t *testing.T) {
	got := FocusPaths([]string{"v1..v2.go", "./src/main.go", "pkg/inner/x.go", "a.go"})
	want := []string{"v1..v2.go", "./src/main.go", "pkg/inner/x.go", "a.go"}
	if len(got) != len(want) {
		t.Fatalf("FocusPaths = %v，想要 %v——相对路径被误伤（tools#177）", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FocusPaths[%d] = %q，想要 %q（tools#177）", i, got[i], want[i])
		}
	}
}

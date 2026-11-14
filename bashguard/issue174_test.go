package bashguard

// tools#174 回归钉：git 不可逆表此前只认 -f/--force 旗形态——`+refspec`
// 同效强推与引用删除类（branch -D / tag -d / remote remove / push --delete）
// 全部落 write 档，与表注释自述覆盖范围（「强推」）不符，二次确认门静默不响。
// 每条用例钉一个失效形态，标题带 issue 号——回归时按号检索。

import "testing"

// TestGitForcePushPlusRefspec_Issue174 `+refspec` 与 --force 同效强推：
// push 的 DestructiveArgs 以 "+" 前缀命中（前缀匹配覆盖整段 refspec）。
func TestGitForcePushPlusRefspec_Issue174(t *testing.T) {
	cases := []string{
		"git push +refs/heads/main:refs/heads/main",
		"git push origin +main:main",
		"git -C /repo push origin +feature:main", // 全局前置旗不歪子命令定位
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——`+refspec` 强推落 write 档（tools#174）", cmd)
		}
	}
}

// TestGitPushDeleteForms_Issue174 push 的远端删除形态：--delete 旗与空 src
// refspec（`:refspec`）删远端引用，与 force 类同属不可逆。
func TestGitPushDeleteForms_Issue174(t *testing.T) {
	cases := []string{
		"git push origin --delete feature",
		"git push origin :refs/heads/feature",
		"git push origin :feature",
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——远端引用删除落 write 档（tools#174）", cmd)
		}
	}
}

// TestGitRefDeleteFamily_Issue174 引用删除类：branch/tag/remote。
func TestGitRefDeleteFamily_Issue174(t *testing.T) {
	cases := []string{
		"git branch -D feature",
		"git branch -d feature",
		"git branch --delete feature",
		"git tag -d v1.0",
		"git tag --delete v1.0",
		"git remote remove origin",
		"git remote rm origin",
		"git remote delete origin",
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if !d.Destructive {
			t.Errorf("%q 判 Destructive=false——引用删除类落 write 档（tools#174）", cmd)
		}
	}
}

// TestGitRefDeleteFamilyNoFalsePositive_Issue174 对照：非删除形态不得被
// 新判据误伤（列表/查看/改名/常规 push 仍走原档位）。
func TestGitRefDeleteFamilyNoFalsePositive_Issue174(t *testing.T) {
	cases := []string{
		"git branch -a",
		"git branch --list v1*",
		"git tag -l v1*",
		"git remote -v",
		"git remote show origin",
		"git remote get-url origin",
		"git remote rename origin up",
		"git push origin main",
		"git push --atomic origin main",
		"git push --porcelain origin main",
	}
	for _, cmd := range cases {
		d := Analyze(cmd, nil)
		if d.Destructive {
			t.Errorf("%q 判 Destructive=true——非删除形态被误报（tools#174）", cmd)
		}
	}
}

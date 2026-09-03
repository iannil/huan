package render

import (
	"strings"
	"testing"
)

func TestTypographCJK(t *testing.T) {
	cases := []struct{ in, want string }{
		{`他说"你好"`, `他说“你好”`}, // 直引号成对 → 弯引号
		{`他说"你好`, `他说“你好`},   // 单个开引号 → 左弯
		// NOTE: brief expected 你好”他说 (right curly), but a lone quote with
		// CJK on BOTH sides (好…他) is structurally indistinguishable from the
		// opening case above (说…你); the state machine must pick one. Default
		// to left curly — deviation from the brief, documented in the report.
		{`你好"他说`, `你好“他说`},
		{`他说"你好"然后走了`, `他说“你好”然后走了`},               // 成对且后续还有正文
		{`维度(通常是 10 或 11 维)`, `维度（通常是 10 或 11 维）`}, // CJK 紧邻半角括号 → 全角
		{`abc(def)ghi`, `abc(def)ghi`},             // 纯拉丁上下文不动
		{`一个—破折号`, `一个——破折号`},                      // CJK 间单破折号 → 双破折号
		{`A—B`, `A—B`},                                                 // 拉丁间不动
		{`冒号:中文`, `冒号：中文`},                                             // CJK 后半角冒号 → 全角
		{`http://example.com:a`, `http://example.com:a`},               // URL 不动
		{`代码 ` + "`a\"b:c(1)`" + ` 结束`, `代码 ` + "`a\"b:c(1)`" + ` 结束`}, // 代码 span 内容跳过
		{`见 [链接](http://x.com/a:b) 尾`, `见 [链接](http://x.com/a:b) 尾`},   // 链接 URL 跳过
		{`中文 latin "quoted" tail`, `中文 latin "quoted" tail`},           // 引号两侧纯拉丁 → 不动
		{`中文 "引文" 结束`, `中文 “引文” 结束`},                                   // 引号前有一个空格也归一
		{`"In an absurd world"`, `“In an absurd world”`},               // 纯拉丁成对双引号 → 弯引号（英文排版）
		{`it's fine, isn't it`, `it's fine, isn't it`},                 // 撇号不动
		{`un "paired quote`, `un "paired quote`},                       // 奇数引号不动
		{`"live joyfully." — Camus`, `“live joyfully.” — Camus`},       // 含 em dash 的拉丁句仍走拉丁路径
		{`"CRM 的 VIP 用户列表"`, `“CRM 的 VIP 用户列表”`},                       // 串首/串尾引号（表格单元格）
	}
	// Latin path must protect verbatim regions exactly like the CJK path.
	latin := []struct{ in, want string }{
		{"call `console.log(\"here\")` now", "call `console.log(\"here\")` now"},         // 代码 span 内引号不动
		{"`trace_id: \"abc-123\"` value", "`trace_id: \"abc-123\"` value"},               // 代码 span（无 CJK）
		{"see [x](https://e.com/a?b=\"c\") end", "see [x](https://e.com/a?b=\"c\") end"}, // 链接 URL 不动
		{`outside "only" these change`, `outside “only” these change`},                   // 代码 span 外的成对引号仍转换
	}
	for _, c := range append(cases, latin...) {
		if got := TypographCJK(c.in); got != c.want {
			t.Errorf("TypographCJK(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// Idempotency: applying twice must equal applying once (titles are
	// normalized again at backend entry points).
	for _, c := range append(cases, latin...) {
		if again := TypographCJK(TypographCJK(c.in)); again != TypographCJK(c.in) {
			t.Errorf("not idempotent for %q: %q", c.in, again)
		}
	}
}

func TestParseChapterAppliesTypograph(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "他说\"你好\"。\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(du.Blocks) != 1 || du.Blocks[0].Kind != BlockParagraph {
		t.Fatalf("blocks: %+v", du.Blocks)
	}
	if strings.Contains(du.Blocks[0].Text, `"`) || !strings.Contains(du.Blocks[0].Text, "“") {
		t.Fatalf("typography not applied: %q", du.Blocks[0].Text)
	}
}

func TestParseChapterCodeBlockUntouched(t *testing.T) {
	du, err := ParseChapter(writeMD(t, "```python\ns = \"他说\"\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range du.Blocks {
		if b.Kind == BlockCode {
			if strings.Contains(b.Text, "“") {
				t.Fatalf("code block must stay verbatim: %q", b.Text)
			}
			return
		}
	}
	t.Fatal("no code block found")
}

# meta description plainify 对齐 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 huan `plainify` 模板函数：让它 strip HTML tags 后折叠所有空白为单空格并 trim，对齐 Hugo plainify 行为；消除 zhurongshuo 565 个文件的 meta description / og:description / twitter:description 多行换行差异。

**Architecture:** 改一处（`internal/template/funcs.go:28` plainify 函数体）—— 让它链式调用已存在的 `stripTags` + `collapseWhitespace`（line 174）+ `strings.TrimSpace`。TDD：先写覆盖 strip / collapse / trim 三个行为的失败测试，再改函数体，最后用 zhurongshuo 实际页面验证 byte-match。

**Tech Stack:** Go 1.x / `regexp`（已在 funcs.go imports）/ `strings`（已导入）/ 无新增依赖。

---

## File Structure

**修订文件**：
- `internal/template/funcs.go` — 改 `plainify` 函数体（line 28），让它接 `collapseWhitespace` + `strings.TrimSpace`
- `internal/template/funcs_test.go` — 追加 `TestPlainify_*` 测试（覆盖 strip tags / collapse whitespace / trim leading-trailing / CJK content / nested tags）

**修订文档（实施后）**：
- `docs/progress/CURRENT_STATE.md` — Stage 2 候选清单移除 #1（meta plainify 已修），其余 4 项保留
- `memory/MEMORY.md` — 经验教训追加"plainify 漏调 collapseWhitespace 是 stage 1 收尾误判的根因之一"
- `memory/daily/2026-06-12.md` — 追加 stage 2 phase 1 完成记录

---

# Phase 1 — TDD 修复 plainify

## Task 1.1：写 plainify 失败测试

**Files**:
- Test: `internal/template/funcs_test.go`（已存在，追加；如不存在则新建）

- [ ] **Step 1: 确认 funcs_test.go 是否存在**

Run: `test -f internal/template/funcs_test.go && echo exists || echo missing`

如果 missing，新建文件，写入（注意 package 名）：

```go
package template

import "testing"
```

如果 exists，读现有内容确认 package / import 块：`head -10 internal/template/funcs_test.go`

- [ ] **Step 2: 追加 plainify 测试到 funcs_test.go**

把以下测试函数追加到文件末尾（不要重复 package 声明）：

```go
func TestPlainify_StripsHTMLTags(t *testing.T) {
	in := "<p>hello <strong>world</strong></p>"
	want := "hello world"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify(%q) = %q, want %q", in, got, want)
	}
}

func TestPlainify_CollapsesWhitespace(t *testing.T) {
	// Hugo plainify 折叠所有空白（含 \n \t 多空格）为单空格
	in := "<p>first</p>\n<p>second</p>"
	want := "first second"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify(%q) = %q, want %q (newline not collapsed)", in, got, want)
	}
}

func TestPlainify_TrimsLeadingTrailingWhitespace(t *testing.T) {
	in := "  \n  <p>content</p>  \n  "
	want := "content"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify(%q) = %q, want %q (leading/trailing not trimmed)", in, got, want)
	}
}

func TestPlainify_PreservesCJKContent(t *testing.T) {
	// 中文场景：多个 <p> 拼接 + \n 分隔
	in := "<p>法不净空，觉无性也。</p>\n<p>一、存在</p>\n<p>1.1、动态存在</p>"
	want := "法不净空，觉无性也。 一、存在 1.1、动态存在"
	got := plainify(in)
	if got != want {
		t.Errorf("plainify CJK:\n  in:   %q\n  got:  %q\n  want: %q", in, got, want)
	}
}

func TestPlainify_HandlesEmptyInput(t *testing.T) {
	got := plainify("")
	if got != "" {
		t.Errorf("plainify(\"\") = %q, want empty", got)
	}
	got2 := plainify(nil)
	if got2 != "" {
		t.Errorf("plainify(nil) = %q, want empty", got2)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/template/ -run TestPlainify -v`

Expected: 至少 3 个测试 FAIL：
- `TestPlainify_CollapsesWhitespace` FAIL（当前 plainify 只 stripTags，输出 `"firstsecond"` 没空格）
- `TestPlainify_TrimsLeadingTrailingWhitespace` FAIL（输出 `"    content  "` 没 trim）
- `TestPlainify_PreservesCJKContent` FAIL（多段 `\n` 没折叠）

`TestPlainify_StripsHTMLTags` 可能 PASS（当前实现已经 strip tags）。
`TestPlainify_HandlesEmptyInput` 可能 PASS（边界）。

- [ ] **Step 4: Commit failing tests**

```bash
git add internal/template/funcs_test.go
git commit -m "test(template): add failing tests for plainify (collapse whitespace + trim)"
```

## Task 1.2：修 plainify 函数体

**Files**:
- Modify: `internal/template/funcs.go:28`

- [ ] **Step 1: 读当前 plainify 行**

Run: `sed -n '28p' internal/template/funcs.go`

应看到：
```go
		"plainify":    func(v interface{}) string { return stripTags(toString(v)) },
```

- [ ] **Step 2: 替换 plainify 函数体**

用 Edit 工具把 line 28 替换为：

```go
		"plainify": func(v interface{}) string {
			s := stripTags(toString(v))
			s = collapseWhitespace(s)
			return strings.TrimSpace(s)
		},
```

注意：
- 缩进与文件其他条目对齐（一个 tab 缩进 FuncMap 内容，两个 tab 缩进 map key-value）
- `stripTags`、`collapseWhitespace`、`strings.TrimSpace` 都是 package-level 已存在的标识符，无需新增 import
- `collapseWhitespace` 在 line 174 已定义；`stripTags` 在 line 167 已定义；`strings` 在 import 块已导入

- [ ] **Step 3: 运行 plainify 测试**

Run: `go test ./internal/template/ -run TestPlainify -v`

Expected: 5 个测试全 PASS。

- [ ] **Step 4: 运行 template 包全部测试确认无回归**

Run: `go test ./internal/template/ -v`

Expected: 所有测试 PASS（含 math funcs、其他 funcs、template 渲染）。

- [ ] **Step 5: 运行全 repo 测试确认无回归**

Run: `go test ./...`

Expected: 所有包 PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/template/funcs.go internal/template/funcs_test.go
git commit -m "fix(template): plainify chains stripTags + collapseWhitespace + TrimSpace (Hugo-aligned)"
```

---

# Phase 2 — zhurongshuo 实证验证

## Task 2.1：rebuild + 抽样验证 meta description

**Files**:
- 无（命令行验证）

- [ ] **Step 1: rebuild huan**

```bash
cd /Users/rong.zhu/Code/huan
go build -o huan ./cmd/huan
rm -rf /tmp/huan-output /Users/rong.zhu/Code/zhurongshuo/docs
./huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null
cp -r /Users/rong.zhu/Code/zhurongshuo/docs /tmp/huan-output
```

Expected: 无错误，`/tmp/huan-output/general/index.html` 存在。

- [ ] **Step 2: 抽 3 篇典型页面验证 meta description byte-match**

```bash
for f in general/index.html practices/season-4/data-as-the-boundary/part-05/chapter-10/index.html books/volume-1/god-beyond-observation/index.html; do
  echo ">>> $f"
  diff <(grep -oE '<meta name=[^>]*description[^>]*>' "/tmp/huan-output/$f" | head -3) \
       <(grep -oE '<meta name=[^>]*description[^>]*>' "/tmp/hugo-baseline/$f" | head -3) \
    && echo "  [MATCH]" || echo "  [STILL DIFFERS]"
done
```

Expected: 3 篇都 `[MATCH]`（huan 与 hugo 的 meta name=description、twitter:description 完全 byte-identical）。

注：`<meta property="og:description">` 也应该 MATCH——可额外验证：
```bash
for f in general/index.html practices/season-4/data-as-the-boundary/part-05/chapter-10/index.html; do
  echo ">>> $f og:description"
  diff <(grep -oE '<meta property="og:description"[^>]*>' "/tmp/huan-output/$f") \
       <(grep -oE '<meta property="og:description"[^>]*>' "/tmp/hugo-baseline/$f") \
    && echo "  [MATCH]" || echo "  [STILL DIFFERS]"
done
```

- [ ] **Step 3: 跑 equiv-check SEO 维度看 differing 数下降**

```bash
/tmp/equiv-check -a /tmp/huan-output -b /tmp/hugo-baseline --mode seo 2>&1 | head -3
```

Expected: `mode=seo differing=N` 数字相比修复前（983）**显著下降**（预期下降 500+，因 565 文件受 plainify 影响）。

注：因 huan build 非确定性（±75 噪声），具体数字会浮动；判断"显著下降"看是否落到 < 500 范围。

- [ ] **Step 4: 跑完整 diff-build.sh Step 5 看四模式数字**

```bash
./scripts/diff-build.sh 2>&1 | tail -10
```

Expected:
- `mode=normalized` differing 下降（565 文件影响）
- `mode=seo` differing 显著下降
- `mode=ai` differing 下降（部分 JSON-LD description 也用 plainify）
- `mode=byte` 下降（因为 byte 现在也对齐了）

记录四个数字（N_normalized / N_seo / N_ai / N_byte）用于后续文档更新。

---

# Phase 3 — 文档更新

## Task 3.1：更新 CURRENT_STATE.md 移除已修项

**Files**:
- Modify: `docs/progress/CURRENT_STATE.md`

- [ ] **Step 1: 读 Stage 2 候选清单段**

Run: `grep -n "Stage 2 候选工作清单\|^\(1\|2\|3\|4\|5\)\. \*\*" docs/progress/CURRENT_STATE.md | head -10`

定位候选清单的 5 项条目（之前 grill-me 修订时写入）。

- [ ] **Step 2: 把原 #1 项标记为已修**

把原条目 1（meta description 多行换行）整段替换为：

```markdown
1. **meta description 多行换行**（原影响 565 文件）→ ✅ **已修（stage 2 phase 1，2026-06-12）**
   - 根因：`internal/template/funcs.go:28` 的 `plainify` 只调 `stripTags`，未调 `collapseWhitespace`
   - 修复：plainify 链式 `stripTags → collapseWhitespace → TrimSpace`
   - 验证：3 篇典型页面 meta name=description / twitter:description / og:description 全部 byte-identical；diff-build.sh mode=seo 从 983 → N（见下）
```

(N 用 Task 2.1 Step 4 跑出的实际数字填入)

- [ ] **Step 3: 在 Stage 2 候选清单顶部加 stage 2 进度小节**

在 "## Stage 2 候选工作清单（...）" 标题下、第一项条目之前，插入：

```markdown
### Stage 2 进度

| Phase | 项 | 状态 | 完成日期 |
|---|---|---|---|
| 1 | meta description plainify | ✅ 已完成 | 2026-06-12 |
| 2 | RSS 中文 URL 编码 | 待启动 | — |
| 3 | books section part 顺序 | 待启动 | — |
| 4 | body 渲染细节 | 待启动 | — |
| 5 | minify artifacts | 待启动 | — |

---
```

- [ ] **Step 4: 验证修改**

Run: `grep -c "已修（stage 2 phase 1\|Stage 2 进度" docs/progress/CURRENT_STATE.md`

Expected: ≥ 2（"Stage 2 进度" 小节标题 + "已修 stage 2 phase 1" 标记）。

- [ ] **Step 5: Commit**

```bash
git add docs/progress/CURRENT_STATE.md
git commit -m "docs(progress): mark stage 2 phase 1 (meta plainify) complete"
```

## Task 3.2：MEMORY.md 追加经验教训

**Files**:
- Modify: `memory/MEMORY.md`

- [ ] **Step 1: 定位"经验教训"段**

Run: `grep -n "^## 经验教训" memory/MEMORY.md`

- [ ] **Step 2: 在经验教训段末尾追加新条目**

在 "## 经验教训" 段最后一行之后追加：

```markdown
- **plainify 漏调 collapseWhitespace 是 stage 1 收尾误判的根因之一**（2026-06-12 stage 2 phase 1 修复）：`internal/template/funcs.go:28` 的 plainify 只调 stripTags，但 `collapseWhitespace` 函数（line 174）已存在且注释明确说 "matching Hugo's plainify behavior"——只是没人把它接进 plainify。**含义**：当某函数的辅助函数已存在且有"匹配 X 行为"的注释时，必须验证它真的被调用；"已存在 = 已使用"是常见误判。修复后 zhurongshuo 565 个文件的 meta description 多行换行差异消除。
```

- [ ] **Step 3: 验证修改**

Run: `grep -c "plainify 漏调 collapseWhitespace" memory/MEMORY.md`

Expected: ≥ 1。

- [ ] **Step 4: Commit**

```bash
git add memory/MEMORY.md
git commit -m "docs(memory): record plainify-missed-collapseWhitespace lesson"
```

## Task 3.3：daily 笔记追加 stage 2 phase 1 完成记录

**Files**:
- Modify: `memory/daily/2026-06-12.md`

- [ ] **Step 1: 在文件末尾追加新段**

```markdown

---

## stage 2 phase 1：meta description plainify（已落地）

按"文档 → plan → 实施"路径完成 stage 2 phase 1（修 plainify 漏调 collapseWhitespace）。

### 落地内容

- `internal/template/funcs.go:28` plainify 函数体改为 `stripTags → collapseWhitespace → strings.TrimSpace`
- `internal/template/funcs_test.go` 追加 5 个测试（Strip / Collapse / Trim / CJK / Empty），全 PASS
- zhurongshuo 3 篇典型页面 meta name=description / twitter:description / og:description 全部 byte-identical 于 Hugo

### diff-build.sh 四模式数字（修复前 → 修复后）

- byte：从 N_before → N_after
- normalized：从 N_before → N_after
- seo：从 983 → N_after（显著下降）
- ai：从 N_before → N_after

（用 Task 2.1 Step 4 跑出的实际数字填入）

### 下一步

stage 2 phase 2：RSS 中文 URL 编码（影响 464 文件）。详见 CURRENT_STATE.md Stage 2 候选清单 #2。
```

(用实际数字替换 N_before / N_after)

- [ ] **Step 2: Commit**

```bash
git add memory/daily/2026-06-12.md
git commit -m "docs(daily): record stage 2 phase 1 meta plainify completion"
```

## Task 3.4：归档 plan + 写完成报告

**Files**:
- Move: `docs/superpowers/plans/2026-06-12-meta-plainify.md` → `docs/reports/completed/2026-06-12-meta-plainify-plan.md`
- Create: `docs/reports/completed/2026-06-12-meta-plainify-report.md`

- [ ] **Step 1: 移动 plan 到 completed**

```bash
mkdir -p docs/reports/completed
git mv docs/superpowers/plans/2026-06-12-meta-plainify.md docs/reports/completed/2026-06-12-meta-plainify-plan.md
# 清理空目录
rmdir docs/superpowers/plans docs/superpowers 2>/dev/null || true
```

- [ ] **Step 2: 写完成报告**

Create `docs/reports/completed/2026-06-12-meta-plainify-report.md`：

````markdown
# meta description plainify 完成报告

> 完成日期：2026-06-12 · 关联 plan：[2026-06-12-meta-plainify-plan.md](2026-06-12-meta-plainify-plan.md)
> 关联上一阶段：[三维度等价标准落地完成报告](2026-06-12-redefine-equivalence-report.md)

## 落地内容

### 代码（1 commit）
- `internal/template/funcs.go:28` plainify 函数体改为链式 `stripTags → collapseWhitespace → strings.TrimSpace`
- 无新增依赖；`collapseWhitespace` 复用已存在函数（line 174）

### 测试（1 commit）
- `internal/template/funcs_test.go` 追加 5 个 plainify 测试（Strip / Collapse / Trim / CJK / Empty）
- 全 PASS，无回归

### 验证结果

zhurongshuo 实际页面 byte-match 验证：
- `general/index.html`：meta name=description / twitter:description / og:description 全部 byte-identical
- `practices/season-4/data-as-the-boundary/part-05/chapter-10/index.html`：同上
- `books/volume-1/god-beyond-observation/index.html`：同上

diff-build.sh 四模式数字（修复前 → 修复后）：

| 模式 | 修复前 differing | 修复后 differing |
|---|---|---|
| byte（雷达） | N_before | N_after |
| normalized（肉眼） | N_before | N_after |
| seo | 983 | N_after |
| ai | N_before | N_after |

（用实际数字替换）

### Stage 2 路线图进度

| Phase | 项 | 状态 |
|---|---|---|
| **1** | meta description plainify | ✅ 已完成 |
| 2 | RSS 中文 URL 编码 | 待启动 |
| 3 | books section part 顺序 | 待启动 |
| 4 | body 渲染细节 | 待启动 |
| 5 | minify artifacts | 待启动 |

## 关键发现

- `collapseWhitespace` 函数自项目初期就存在且注释明确说 "matching Hugo's plainify behavior"，但 plainify 从未调用它——这是 stage 1 收尾时把 meta description 方向描述反了的根因。
- 教训：辅助函数已存在 + 有"匹配 X 行为"注释 ≠ 已被使用。必须 grep 调用点验证。
````

(用实际数字替换 N_before / N_after)

- [ ] **Step 3: Commit 归档**

```bash
git add docs/reports/completed/ docs/superpowers/
git commit -m "docs(reports): archive meta-plainify plan + completion report"
```

---

# Self-Review Notes

完成全部 task 后自检：

**Spec 覆盖**：
- ✅ 修 plainify（funcs.go:28）→ Task 1.2
- ✅ 测试覆盖（含 strip / collapse / trim / CJK / empty）→ Task 1.1
- ✅ zhurongshuo byte-match 验证 → Task 2.1
- ✅ 文档更新（CURRENT_STATE / MEMORY / daily / 完成报告）→ Task 3.1-3.4

**Placeholder 扫描**：
- 无 TBD/TODO — 每步都有具体代码或命令
- N_before / N_after 是占位符但明确说明"用实际数字填入"——这是合理的（数字在 Task 2.1 跑出后才知道）

**类型一致性**：
- `plainify` 函数签名不变（`func(v interface{}) string`），只是函数体改
- `stripTags` / `collapseWhitespace` / `strings.TrimSpace` 都是 package-level 已存在标识符
- 测试函数命名遵循 Go 惯例 `TestPlainify_<Behavior>`

**已验证未引入新问题**：
- funcs.go imports 已有 `strings`，无需新增
- `collapseWhitespace` 已在 funcs.go:174 定义，无需新建
- 修复只影响 plainify 调用点（zhurongshuo head.html 3 处），不影响其他模板函数

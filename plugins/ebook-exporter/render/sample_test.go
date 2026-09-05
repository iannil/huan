package render

// 样张生成器（手动触发，非常规测试）：
//
//	go test ./render -run '^TestGenerateStyleSamples$' -v
//
// 构造一本覆盖全部块类型与版式特征的"构造样张"书（zh + en 两版），
// 用三个后端渲染为 PDF/EPUB/DOCX，输出目录由 EBOOK_SAMPLE_OUT 指定
// （默认 developer/export/STYLE-SAMPLES，相对当前工作目录），字体由
// EBOOK_SAMPLE_FONT 指定（缺省走 style.FindCJKFont）。
//
// 样张内容为专用构造文本，刻意覆盖出版版式的全部模式，供正式修改前的
// 版式确认；见 scripts/gen_style_samples.sh 的对照清单。

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
	"github.com/iannil/huan-plugin-ebook-exporter/style"
)

// sampleOutDir resolves the sample output directory.
func sampleOutDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("EBOOK_SAMPLE_OUT"); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	return t.TempDir()
}

// sampleFontPath resolves the CJK font for PDF rendering.
func sampleFontPath(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("EBOOK_SAMPLE_FONT"); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("EBOOK_SAMPLE_FONT: %v", err)
		}
		return p
	}
	p, err := style.FindCJKFont("")
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	return p
}

// mkSampleBook writes the specially-constructed sample content to dir and
// returns the matching BookEntry. lang selects the zh or en edition; the
// en edition reuses the zh markdown files as its ENPath sidecars so the EN
// renderers exercise their path without duplicating content.
func mkSampleBook(t *testing.T, dir string, lang content.Lang) *content.BookEntry {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "part-01"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("---\ntitle: "+name+"\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 导言：段落 + 引用 + 列表 + 分隔线 + ASCII 省略号/夹空格连字符。
	write("introduction.md", `本书是"版式样张"：刻意收集全书系会用到的全部版式元素，供出版级排版确认。

直引号会转为弯引号 "像这样"，半角括号(如这个)贴 CJK 时转全角，冒号: 同理转全角；ASCII 省略号写作 ... 应归一为省略号，夹空格连字符 - 才转破折号，而 state-of-the-art 这种连字符必须保持原样。

> 引用块：缩进呈现，颜色比正文浅。
> 第二行引用，含 **加粗** 与 `+"`code`"+` 行内代码。

- 无序列表第一项，含 [链接](https://zhurongshuo.com) 行内元素
- 无序列表第二项

---

导言结束。
`)
	// 第一章：标题层级阶梯（h2/h3/h4/h5）+ 混排 + 长 URL + 表格。
	write("part-01/chapter-01.md", `## 二级标题：元素总览

正文段落：中文正文两端对齐，首行缩进两字符。中英混排如 AWS 的 S3 存储桶、Kubernetes 的 Ingress 资源、CI/CD 流水线等术语，中西文之间保留四分之一字宽间距。

### 三级标题：行内元素

**加粗**、*斜体*、`+"`行内代码`"+`、[链接](https://zhurongshuo.com/guide) 的混排示例。表 1 是表格版式：

| 概念 | 英文 | 说明 |
|------|------|------|
| 观测者 | Observer | 认知主体 |
| 实在政治学 | Realpolitik | 权力运作 |

#### 四级标题：长 URL 折行

超长 URL 需要折行而不能溢出页面右缘，例如 https://github.com/iannil/huan/blob/master/plugins/ebook-exporter/render/pdf.go 这样的仓库深链接，以及 https://developer.mozilla.org/en-US/docs/Web/CSS/hyphens 文档链接。

##### 五级标题：标点样张

直引号 "观测者" 转弯引号；省略号 ... 归一；破折号 - 如此；冒号: 转全角。

## 二级标题之二：引用与代码

> 多行引用块的第一段。
>
> 引用内的第二段，含嵌套元素 *斜体*。

`+"```go"+`
// 代码块必须 verbatim：URL https://example.com/a_b 与连字符 state-of-the-art
// 以及省略号 ... 都不得被标点归一改写。
func Observer() string {
	return "观测者"
}
`+"```"+`

正文收尾段落。
`)
	// 第二章：代码长行 + 大表格 + 列表 + 主题分隔。
	write("part-01/chapter-02.md", `## 代码块版式

`+"```bash"+`
# 长行代码：超过正文宽度的命令必须折行而不丢字符
huan export ebook --type books --level all --format all --jobs 8 --force --output developer/export
`+"```"+`

## 表格与列表版式

| 卷 | 书名 | 主题 |
|----|------|------|
| 第一卷 | 信任的根基 | 信任 |
| 第二卷 | 观测的诞生 | 观测 |
| 第三卷 | 评价的枷锁 | 评价 |

1. 有序列表第一项
2. 有序列表第二项

---

主题分隔线之后的收尾段落。
`)
	// 结语：极简。
	write("epilogue.md", `样张到此结束。以上元素覆盖了三种导出格式的全部块类型与行内版式。
`)
	write("introduction.en.md", `This is the typography specimen book: it deliberately collects every layout element used across the catalog, for publication-grade review.

Straight "quotes" become curly; ASCII ellipsis ... normalizes; a spaced hyphen - becomes an em dash, while state-of-the-art hyphens must stay literal.

> A block quote: indented, lighter than body text.
> Second line with **bold** and `+"`code`"+`.

- Unordered item one with a [link](https://zhurongshuo.com)
- Unordered item two

---

End of introduction.
`)
	write("part-01/chapter-01.en.md", `## Heading Two: Element Overview

Justified body paragraph with first-line behavior per English convention. Mixed-script text like Kubernetes Ingress resources and CI/CD pipelines keeps a quarter-em gap between scripts.

### Heading Three: Inline Elements

**Bold**, *italic*, `+"`code`"+`, and [links](https://zhurongshuo.com/guide) mixed inline. Table 1 shows table layout:

| Concept | Chinese | Note |
|---------|---------|------|
| Observer | 观测者 | Subject |
| Realpolitik | 实在政治学 | Power |

#### Heading Four: Long URL Wrapping

Long URLs must wrap instead of overflowing the right margin, e.g. https://github.com/iannil/huan/blob/master/plugins/ebook-exporter/render/pdf.go and https://developer.mozilla.org/en-US/docs/Web/CSS/hyphens.

##### Heading Five: Punctuation Specimen

Straight "quotes" become curly; ellipsis ... normalizes; a spaced hyphen - becomes an em dash.

## Second Heading Two: Quote and Code

> First paragraph of a multi-paragraph quote.
>
> Second paragraph with *italic* inside.

`+"```go"+`
// Code must stay verbatim: URLs https://example.com/a_b, hyphens
// state-of-the-art, and ellipsis ... are never rewritten.
func Observer() string {
	return "observer"
}
`+"```"+`

Closing paragraph.
`)
	write("part-01/chapter-02.en.md", `## Code Block Layout

`+"```bash"+`
# Long command lines must wrap without losing characters
huan export ebook --type books --level all --format all --jobs 8 --force --output developer/export
`+"```"+`

## Table and List Layout

| Volume | Book | Theme |
|--------|------|-------|
| Vol. 1 | Foundation of Trust | Trust |
| Vol. 2 | Birth of Observation | Observation |

1. Ordered item one
2. Ordered item two

---

Closing paragraph after a thematic break.
`)
	write("epilogue.en.md", `End of specimen. The elements above cover every block type and inline layout across the three export formats.
`)

	chap := func(name, enName string) content.Chapter {
		ch := content.Chapter{SourcePath: filepath.Join(dir, name), Title: name}
		if lang == content.LangEN {
			ch.ENPath = filepath.Join(dir, enName)
		}
		return ch
	}
	book := &content.BookEntry{
		Slug: "style-sample", TitleZH: "版式样张", TitleEN: "Typography Specimen",
		SubtitleZH: "电子书出版级排版确认样张", Version: "rc", LastUpdated: "2026-09-05",
		Dir: dir, HasEN: true,
		Sections: []content.Section{
			{Type: "introduction", Title: "引言", Chapters: []content.Chapter{chap("introduction.md", "introduction.en.md")}},
			{Type: "part", ID: "part-01", Title: "第一部 · 版式元素", Chapters: []content.Chapter{
				chap("part-01/chapter-01.md", "part-01/chapter-01.en.md"),
				chap("part-01/chapter-02.md", "part-01/chapter-02.en.md"),
			}},
			{Type: "epilogue", Title: "结语", Chapters: []content.Chapter{chap("epilogue.md", "epilogue.en.md")}},
		},
	}
	return book
}

func TestGenerateStyleSamples(t *testing.T) {
	out := sampleOutDir(t)
	fontPath := sampleFontPath(t)
	monoFontPath := style.FindMonoFont("")

	for _, lang := range []content.Lang{content.LangZH, content.LangEN} {
		dir := filepath.Join(out, fmt.Sprintf(".src-%s", lang))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		book := mkSampleBook(t, dir, lang)

		pdfOut := filepath.Join(out, fmt.Sprintf("style-sample-%s.pdf", lang))
		if err := RenderPDF(book, lang, pdfOut, PDFOptions{FontPath: fontPath, MonoFontPath: monoFontPath}); err != nil {
			t.Errorf("render pdf %s: %v", lang, err)
		}

		epubOut := filepath.Join(out, fmt.Sprintf("style-sample-%s.epub", lang))
		if err := RenderEPUB(book, lang, epubOut, EPUBOptions{EmbedFont: true, FontPath: fontPath}); err != nil {
			t.Errorf("render epub %s: %v", lang, err)
		}

		docxOut := filepath.Join(out, fmt.Sprintf("style-sample-%s.docx", lang))
		if err := RenderDOCX(book, lang, docxOut, DOCXOptions{}); err != nil {
			t.Errorf("render docx %s: %v", lang, err)
		}
	}
	fmt.Printf("style samples written to %s\n", out)
}

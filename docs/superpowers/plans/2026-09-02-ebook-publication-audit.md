# 电子书出版级审计与修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立可重复的电子书出版级审计（自动化全量 + 抽样深审），并按批次修复 ebook-exporter 的排版缺陷，直至 P1 级归零。

**Architecture:** 审计脚本 `scripts/audit_ebooks.py` 落在 zhurongshuo（消费产物侧，PyMuPDF + zipfile + 正则），产出分级报告；修复全部改 huan worktree 的 `plugins/ebook-exporter/render/`，每批改完 `--force` 全量重生成、复跑审计验证归零后 commit。PDF 书签走 gpdf 渲染后 PyMuPDF 后处理（gpdf v1.0.11 无 outline API，已确认）；DOCX 页脚走 docxgo `SectionBuilder.Footer`（已确认存在）；eastAsia 字体走 `domain.Run.SetFont(domain.Font{Name, EastAsia})`（已确认 Font 含 EastAsia 字段）；EPUB 字体子集化用 fontTools（4.60.2 已装）。

**Tech Stack:** Python 3（PyMuPDF 1.26.5、fontTools 4.60.2）、Go（gpdf v1.0.11、docxgo v2.14.0、go-shiori/go-epub v1.2.1）、epubcheck（jar 下载到 developer/audit-tools/，gitignore）。

**Spec:** `docs/superpowers/specs/2026-09-02-ebook-publication-audit-design.md`（huan worktree）

## Global Constraints

- 修复代码全部在 huan worktree（`/Users/rong.zhu/Code/zhurong/huan/.claude/worktrees/ebook-exporter`，分支 feature/ebook-exporter）的 `plugins/ebook-exporter/` 内；不 import huan `internal/`
- 审计脚本 XML 解析一律用 `defusedxml.ElementTree`（不用 stdlib `xml.etree`，防 XXE）；defusedxml 本机已装
- 标点转换只在导出管线内存中做，**永不回写 zhurongshuo `content/` 源文件**
- zhurongshuo 侧新增文件只有：`scripts/audit_ebooks.py`（+测试）；产物与审计输出落 `developer/export/`；`developer/audit-tools/` 进 `.gitignore`
- 审计脚本风格对齐 `scripts/check_translation_quality.py`：`--json` / `--fail-on P0|P1|P2` / 非零退出码
- 每个修复批次完成后必须：`/tmp/huan-ebook export ebook --type all --format all --level all --force --jobs 8` 全量重生成 → `python3 scripts/audit_ebooks.py --fail-on <本批级别>` 归零 → commit（huan 侧）
- huan CLI 用 `/tmp/huan-ebook`（worktree 构建；每次 huan 代码改动后需重建：`cd <worktree> && go build -o /tmp/huan-ebook ./cmd/huan`，先确认 main 包路径）；.so 改动后重建并复制到 `~/.huan/plugins/`
- 修复后 `.so` 重建命令：`cd <worktree>/plugins/ebook-exporter && go build -buildmode=plugin -o ../../release/plugins/ebook-exporter.so . && cp ../../release/plugins/ebook-exporter.so ~/.huan/plugins/`

## 已确认的技术事实（实现者无需重新调研）

- gpdf v1.0.11：有 `Document.Header(fn func(*PageBuilder))` / `Footer` / `ColBuilder.PageNumber()` / `TotalPages()`（builder.go:126,132；grid.go:446,459）；**无 outline/bookmark API**（全库 grep `Outlines` 无果）
- gpdf 换行缺陷（P0 级 bug）：长不可断行拉丁串（如 `10^500` 前后的混合串）不被折行，span 右缘达 604.3 > 页宽 595.28，82 页中 17 页溢出——文字被裁切
- docxgo v2.14.0：`docx.NewTOCField(map[string]string)` 存在（docx.go:186）；`SectionBuilder.Footer(domain.FooterType) (domain.Footer, error)`（builder.go:133）；`domain.Font{Name, EastAsia, CS string}` + `Run.SetFont(Font)`（run.go:114, 21）；`docx.NewPageNumberField()` 存在（docx.go:153）
- 当前 EPUB 内嵌 Noto Sans CJK 全量 OTF ≈ 14MB/本，未子集化
- PDF 无 `/Outlines`、无页眉页码；DOCX 无 TOC 字段、无 eastAsia 声明（styles.xml 无 `w:eastAsia`）
- 源内容标点：正文大量半角 `"` 直引号（单章 95 处）与半角括号，Web 版靠 goldmark Typographer 处理，导出管线没有
- 审计基线数据（reality-construction.pdf）：82 页、17 页 span 溢出、toc=[]

---

### Task 1: 审计脚本骨架 + P0 检查项

**Files:**
- Create: `/Users/rong.zhu/Code/zhurong/zhurongshuo/scripts/audit_ebooks.py`
- Test: `/Users/rong.zhu/Code/zhurong/zhurongshuo/scripts/test_audit_ebooks.py`

**Interfaces:**
- Consumes: 无
- Produces: `audit_ebooks.py` 的 CLI（`--path`/`--json`/`--fail-on`）、检查函数注册结构 —— Task 2/3 扩展 P1/P2 检查；修复任务用它做回归验证

- [ ] **Step 1: 写失败测试（构造坏样本 + 断言命中）**

```python
#!/usr/bin/env python3
"""Tests for audit_ebooks.py — run from repo root: python3 -m pytest scripts/test_audit_ebooks.py -v
(or: python3 scripts/test_audit_ebooks.py)"""
import os, sys, tempfile, zipfile, unittest

sys.path.insert(0, os.path.dirname(__file__))
import audit_ebooks as ae

def make_bad_epub(path):
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("OEBPS/content.xhtml", "not well formed <p>unclosed")  # P0: bad xhtml
        z.writestr("OPF.opf", "<package></package>")  # P0: no metadata

def make_ok_epub(path):
    xhtml = '<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body><p>ok</p></body></html>'
    opf = ('<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="u">'
           '<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">'
           '<dc:title>T</dc:title><dc:creator>a</dc:creator><dc:language>zh</dc:language>'
           '<dc:identifier id="u">x</dc:identifier></metadata></package>')
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("mimetype", "application/epub+zip", compress_type=zipfile.ZIP_STORED)
        z.writestr("META-INF/container.xml", '<?xml version="1.0"?><container/>')
        z.writestr("OEBPS/content.xhtml", xhtml)
        z.writestr("OEBPS/content.opf", opf)

class TestP0(unittest.TestCase):
    def test_bad_epub_flags_p0(self):
        with tempfile.TemporaryDirectory() as d:
            bad = os.path.join(d, "bad.epub"); make_bad_epub(bad)
            findings = ae.audit_epub(bad)
            p0 = [f for f in findings if f.level == "P0"]
            self.assertTrue(len(p0) >= 1, f"expected P0 findings, got {findings}")

    def test_ok_epub_no_p0(self):
        with tempfile.TemporaryDirectory() as d:
            ok = os.path.join(d, "ok.epub"); make_ok_epub(ok)
            findings = ae.audit_epub(ok)
            p0 = [f for f in findings if f.level == "P0"]
            self.assertEqual(p0, [])

    def test_mimetype_must_be_first_and_stored(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "m.epub")
            with zipfile.ZipFile(p, "w") as z:
                z.writestr("a.txt", "x")  # mimetype not first
                z.writestr("mimetype", "application/epub+zip")
            findings = ae.audit_epub(p)
            self.assertTrue(any("mimetype" in f.message for f in findings if f.level == "P0"))

if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/rong.zhu/Code/zhurong/zhurongshuo && python3 scripts/test_audit_ebooks.py`
Expected: FAIL（`No module named 'audit_ebooks'` 或 AttributeError）

- [ ] **Step 3: 实现 audit_ebooks.py（骨架 + P0）**

```python
#!/usr/bin/env python3
"""
电子书出版级审计脚本（P0/P1/P2 分级）。

对 developer/export/ 下的 epub/pdf/docx 产物执行出版标准检查：
  P0 结构：epub 包完整性/OPF 元数据/XHTML well-formed/资源死链；
           pdf 头/页数/字体；docx document+styles 完整性
  P1 出版：（Task 2 填充）标点规范、书签、页码页眉、TOC、eastAsia、子集化、溢出
  P2 润色：（Task 3 填充）标题跳级、元数据丰富度

用法：
  python3 scripts/audit_ebooks.py                      # 默认扫 developer/export
  python3 scripts/audit_ebooks.py --path developer/export/pdf
  python3 scripts/audit_ebooks.py --json
  python3 scripts/audit_ebooks.py --fail-on P1         # P0/P1 任一命中即退出码 1

退出码：0 无问题或低于阈值；1 达到阈值。
"""
import argparse, dataclasses, json, os, re, sys, zipfile
from defusedxml import ElementTree as ET  # XXE-safe（审计对象是自产文件，但按安全规范统一 defusedxml）
import fitz  # PyMuPDF

@dataclasses.dataclass
class Finding:
    level: str      # "P0" | "P1" | "P2"
    check: str      # 检查项名，如 "epub.mimetype"
    file: str
    message: str

LEVEL_ORDER = {"P0": 0, "P1": 1, "P2": 2}

def audit_epub(path: str) -> list[Finding]:
    findings = []
    rel = os.path.relpath(path)
    try:
        zf = zipfile.ZipFile(path)
    except Exception as e:
        return [Finding("P0", "epub.zip", rel, f"unopenable: {e}")]
    names = zf.namelist()
    # P0: mimetype first + stored
    if not names or names[0] != "mimetype":
        findings.append(Finding("P0", "epub.mimetype", rel, "mimetype not first entry"))
    else:
        info = zf.getinfo("mimetype")
        if info.compress_type != zipfile.ZIP_STORED:
            findings.append(Finding("P0", "epub.mimetype", rel, "mimetype compressed"))
        elif zf.read("mimetype").decode("utf-8", "replace") != "application/epub+zip":
            findings.append(Finding("P0", "epub.mimetype", rel, "wrong mimetype content"))
    # P0: find OPF, check metadata
    opf_names = [n for n in names if n.endswith(".opf")]
    if not opf_names:
        findings.append(Finding("P0", "epub.opf", rel, "no .opf found"))
    else:
        try:
            root = ET.fromstring(zf.read(opf_names[0]))
            ns = {"dc": "http://purl.org/dc/elements/1.1/", "opf": "http://www.idpf.org/2007/opf"}
            meta = root.find("opf:metadata", ns) or root.find("metadata")
            for tag in ("title", "creator", "language", "identifier"):
                if meta is None or meta.find(f"dc:{tag}", ns) is None:
                    findings.append(Finding("P0", "epub.opf", rel, f"metadata missing dc:{tag}"))
        except ET.ParseError as e:
            findings.append(Finding("P0", "epub.opf", rel, f"opf not well-formed: {e}"))
    # P0: xhtml well-formed + referenced local resources exist
    for n in names:
        if n.endswith((".xhtml", ".html")):
            try:
                ET.fromstring(zf.read(n))
            except ET.ParseError as e:
                findings.append(Finding("P0", "epub.xhtml", rel, f"{n} not well-formed: {e}"))
    for n in names:
        if n.endswith(".css"):
            css = zf.read(n).decode("utf-8", "replace")
            base = os.path.dirname(n)
            for m in re.findall(r"url\(([^)]+)\)", css):
                target = m.strip("'\"").split("?")[0]
                if target.startswith(("http:", "https:", "data:")):
                    continue
                resolved = os.path.normpath(os.path.join(base, target))
                if resolved not in names:
                    findings.append(Finding("P0", "epub.deadlink", rel, f"{n} references missing {target}"))
    return findings

def audit_pdf(path: str) -> list[Finding]:
    findings = []
    rel = os.path.relpath(path)
    with open(path, "rb") as f:
        if f.read(5) != b"%PDF-":
            return [Finding("P0", "pdf.header", rel, "missing %PDF- header")]
    try:
        doc = fitz.open(path)
    except Exception as e:
        return [Finding("P0", "pdf.open", rel, f"unopenable: {e}")]
    if doc.page_count == 0:
        findings.append(Finding("P0", "pdf.pages", rel, "zero pages"))
    for i in range(doc.page_count):
        fonts = doc[i].get_fonts()
        if not fonts:
            findings.append(Finding("P0", "pdf.fonts", rel, f"page {i+1} has no font resources"))
            break
    return findings

def audit_docx(path: str) -> list[Finding]:
    findings = []
    rel = os.path.relpath(path)
    try:
        zf = zipfile.ZipFile(path)
    except Exception as e:
        return [Finding("P0", "docx.zip", rel, f"unopenable: {e}")]
    for required in ("word/document.xml", "word/styles.xml"):
        if required not in zf.namelist():
            findings.append(Finding("P0", "docx.parts", rel, f"{required} missing"))
    if "word/document.xml" in zf.namelist() and "word/styles.xml" in zf.namelist():
        try:
            doc_xml = ET.fromstring(zf.read("word/document.xml"))
            styles_xml = ET.fromstring(zf.read("word/styles.xml"))
            W = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
            defined = {s.get(f"{W}styleId") for s in styles_xml.findall(f"{W}style")}
            used = {p.get(f"{W}val") for p in doc_xml.iter(f"{W}pStyle")}
            missing = used - defined - {None}
            if missing:
                findings.append(Finding("P0", "docx.styles", rel, f"undefined styles: {sorted(missing)}"))
        except ET.ParseError as e:
            findings.append(Finding("P0", "docx.xml", rel, f"xml parse error: {e}"))
    return findings

AUDITORS = {".epub": audit_epub, ".pdf": audit_pdf, ".docx": audit_docx}

def audit_tree(root: str) -> list[Finding]:
    findings = []
    for dirpath, _dirs, files in os.walk(root):
        for name in sorted(files):
            ext = os.path.splitext(name)[1].lower()
            if ext in AUDITORS:
                findings.extend(AUDITORS[ext](os.path.join(dirpath, name)))
    return findings

def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--path", default="developer/export")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--fail-on", choices=["P0", "P1", "P2"], default="P0")
    args = ap.parse_args()
    findings = audit_tree(args.path)
    findings.sort(key=lambda f: (LEVEL_ORDER[f.level], f.file, f.check))
    if args.json:
        print(json.dumps([dataclasses.asdict(f) for f in findings], ensure_ascii=False, indent=2))
    else:
        # 人读摘要
        from collections import Counter
        by_level = Counter(f.level for f in findings)
        print(f"audited: {args.path}")
        print(f"P0={by_level['P0']} P1={by_level['P1']} P2={by_level['P2']}")
        for f in findings:
            print(f"  [{f.level}] {f.check} {f.file}: {f.message}")
    threshold = LEVEL_ORDER[args.fail_on]
    return 1 if any(LEVEL_ORDER[f.level] <= threshold for f in findings) else 0

if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 4: 跑测试确认通过**

Run: `python3 scripts/test_audit_ebooks.py`
Expected: 3/3 PASS

- [ ] **Step 5: 对真实产物冒烟（当前全量 P0 基线）**

Run: `python3 scripts/audit_ebooks.py --path developer/export | head -30`
Expected: 打印 P0/P1/P2 计数（P1/P2 暂为 0）；记录当前 P0 数作为基线。**已知 PDF span 溢出属 P1（Task 2 加检查）**

- [ ] **Step 6: 提交（zhurongshuo）**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && git add scripts/audit_ebooks.py scripts/test_audit_ebooks.py && git commit -m "feat(audit): ebook publication audit script — P0 structural checks"
```

---

### Task 2: P1 检查项（出版硬指标）

**Files:**
- Modify: `/Users/rong.zhu/Code/zhurong/zhurongshuo/scripts/audit_ebooks.py`
- Test: `/Users/rong.zhu/Code/zhurong/zhurongshuo/scripts/test_audit_ebooks.py`（追加）

**Interfaces:**
- Consumes: Task 1 的 Finding/audit_* 结构
- Produces: P1 检查函数（每格式新增）—— 修复任务 5-8 逐项用它验证归零

- [ ] **Step 1: 追加失败测试**

```python
class TestP1(unittest.TestCase):
    def test_pdf_span_overflow_detected(self):
        # 构造一个 span 越过右缘的 PDF：用 fitz 造一页放超宽文本
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "x.pdf")
            doc = fitz.open()
            page = doc.new_page(width=200, height=100)
            page.insert_text((10, 50), "A" * 120, fontsize=10)  # 120 字符远超 200pt 宽
            doc.save(p); doc.close()
            findings = ae.audit_pdf(p)
            self.assertTrue(any(f.check == "pdf.overflow" and f.level == "P1" for f in findings),
                            f"expected overflow finding, got {findings}")

    def test_pdf_no_outline_flagged(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "x.pdf")
            doc = fitz.open(); doc.new_page(); doc.save(p); doc.close()
            findings = ae.audit_pdf(p)
            self.assertTrue(any(f.check == "pdf.outlines" for f in findings))

    def test_epub_straight_quote_flagged(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "q.epub")
            make_ok_epub(p)
            # 注入含直引号中文正文
            import zipfile as zf2
            with zf2.ZipFile(p, "a") as z:
                pass
            # 重新构造带直引号的
            p2 = os.path.join(d, "q2.epub")
            xhtml = '<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body><p>他说"你好"之后</p></body></html>'
            with zf2.ZipFile(p2, "w") as z:
                z.writestr("mimetype", "application/epub+zip", compress_type=zf2.ZIP_STORED)
                z.writestr("META-INF/container.xml", '<?xml version="1.0"?><container/>')
                z.writestr("OEBPS/content.xhtml", xhtml)
                z.writestr("OEBPS/content.opf", '<?xml version="1.0"?><package/>')
            findings = ae.audit_epub(p2)
            self.assertTrue(any(f.check == "epub.punct" for f in findings), f"got {findings}")

    def test_docx_no_toc_no_eastasia_flagged(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "x.docx")
            doc_xml = '<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p/></w:body></w:document>'
            styles_xml = '<?xml version="1.0"?><w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:style w:styleId="Normal"/></w:styles>'
            with zipfile.ZipFile(p, "w") as z:
                z.writestr("word/document.xml", doc_xml)
                z.writestr("word/styles.xml", styles_xml)
            findings = ae.audit_docx(p)
            self.assertTrue(any(f.check == "docx.toc" for f in findings))
            self.assertTrue(any(f.check == "docx.eastasia" for f in findings))
```

- [ ] **Step 2: 跑测试确认失败**

Run: `python3 scripts/test_audit_ebooks.py`
Expected: 新增 4 条 FAIL（检查项不存在）

- [ ] **Step 3: 实现 P1 检查（追加到对应 audit_* 函数）**

各函数内追加（要点，写入时保持与 Task 1 代码风格一致）：

`audit_epub` 追加：
```python
    # P1: 直引号残留（中文正文中的 ASCII " 与 '）
    for n in names:
        if n.endswith((".xhtml", ".html")):
            text = zf.read(n).decode("utf-8", "replace")
            body = text.split("<body", 1)[-1]
            cjk_ctx = re.findall(r'[一-鿿]"[^<]{0,20}', body)
            if len(cjk_ctx) > 5:  # 阈值：>5 处判缺陷
                findings.append(Finding("P1", "epub.punct", rel, f"{n}: {len(cjk_ctx)} straight quotes near CJK"))
    # P1: CSS 避头尾
    css_all = "".join(zf.read(n).decode("utf-8", "replace") for n in names if n.endswith(".css"))
    if "line-break" not in css_all:
        findings.append(Finding("P1", "epub.css.linebreak", rel, "no line-break rule in CSS"))
    if "orphans" not in css_all or "widows" not in css_all:
        findings.append(Finding("P1", "epub.css.orphans", rel, "no orphans/widows in CSS"))
    # P1: 嵌入字体子集化（字体文件 > 8MB 视为未子集）
    for n in names:
        if "/fonts/" in n and n.endswith((".otf", ".ttf")):
            if zf.getinfo(n).file_size > 8 * 1024 * 1024:
                findings.append(Finding("P1", "epub.font.subset", rel, f"{n} not subsetted ({zf.getinfo(n).file_size//1024//1024}MB)"))
```

`audit_pdf` 追加：
```python
    # P1: 书签
    if not doc.get_toc():
        findings.append(Finding("P1", "pdf.outlines", rel, "no outline/bookmarks"))
    # P1: 页码（任一页脚区域含纯数字文本）
    has_pageno = any(
        re.search(r"^\s*\d{1,3}\s*$", doc[i].get_text("text", clip=fitz.Rect(0, page_h * 0.92, page_w, page_h)) or "")
        for i in range(min(10, doc.page_count))
        for page_w, page_h in [(doc[i].rect.width, doc[i].rect.height)]
    )
    if not has_pageno and doc.page_count > 3:
        findings.append(Finding("P1", "pdf.pageno", rel, "no page numbers detected in footers"))
    # P1: 页眉（顶部 8% 区域有文本）
    has_header = any(
        (doc[i].get_text("text", clip=fitz.Rect(0, 0, doc[i].rect.width, doc[i].rect.height * 0.08)) or "").strip()
        for i in range(2, min(12, doc.page_count))
    )
    if not has_header and doc.page_count > 5:
        findings.append(Finding("P1", "pdf.header", rel, "no running header detected"))
    # P1: span 溢出（右缘超出 MediaBox）
    over = 0
    for i in range(doc.page_count):
        pw = doc[i].rect.width
        for blk in doc[i].get_text("dict")["blocks"]:
            for line in blk.get("lines", []):
                for span in line["spans"]:
                    if span["bbox"][2] > pw + 0.5 or span["bbox"][0] < -0.5:
                        over += 1
    if over:
        findings.append(Finding("P1", "pdf.overflow", rel, f"{over} spans beyond page box"))
    # P1: 直引号（正文层）
    quotes = 0
    for i in range(min(20, doc.page_count)):
        quotes += len(re.findall(r'[一-鿿]"', doc[i].get_text()))
    if quotes > 10:
        findings.append(Finding("P1", "pdf.punct", rel, f"{quotes} straight quotes near CJK"))
```

`audit_docx` 追加：
```python
    doc_xml_s = zf.read("word/document.xml").decode("utf-8", "replace")
    styles_s = zf.read("word/styles.xml").decode("utf-8", "replace")
    # P1: TOC 字段
    if "TOC" not in doc_xml_s and 'w:instrText' not in doc_xml_s:
        findings.append(Finding("P1", "docx.toc", rel, "no TOC field"))
    # P1: eastAsia 字体声明
    if "w:eastAsia=" not in styles_s:
        findings.append(Finding("P1", "docx.eastasia", rel, "no eastAsia font declared in styles"))
    # P1: 页码（页脚 part 或 PAGE 字段）
    has_page = any("footer" in n.lower() for n in zf.namelist()) or "PAGE" in doc_xml_s
    if not has_page:
        findings.append(Finding("P1", "docx.pageno", rel, "no footer/page-number part"))
    # P1: 直引号
    text_all = re.sub(r"<[^>]+>", "", doc_xml_s)
    quotes = len(re.findall(r'[一-鿿]"', text_all))
    if quotes > 10:
        findings.append(Finding("P1", "docx.punct", rel, f"{quotes} straight quotes near CJK"))
```

- [ ] **Step 4: 跑测试确认通过 + 全量基线**

Run: `python3 scripts/test_audit_ebooks.py && python3 scripts/audit_ebooks.py 2>&1 | head -40`
Expected: 测试全绿；基线报告呈现预期缺陷（pdf.outlines/pdf.pageno/pdf.header/pdf.overflow/pdf.punct、docx.toc/docx.eastasia/docx.pageno、epub.punct/epub.font.subset/epub.css.*）——把完整基线存档 `developer/export/AUDIT-REPORT-baseline.md`（`python3 scripts/audit_ebooks.py > developer/export/AUDIT-REPORT-baseline.md 2>&1`）

- [ ] **Step 5: 提交（zhurongshuo）**

```bash
git add scripts/audit_ebooks.py scripts/test_audit_ebooks.py && git commit -m "feat(audit): P1 publication checks — punctuation, bookmarks, page numbers, TOC, subset"
```

---

### Task 3: P2 检查项 + epubcheck 深审集成 + 基线报告

**Files:**
- Modify: `/Users/rong.zhu/Code/zhurong/zhurongshuo/scripts/audit_ebooks.py`
- Modify: `/Users/rong.zhu/Code/zhurong/zhurongshuo/.gitignore`
- Test: 追加到 `scripts/test_audit_ebooks.py`

**Interfaces:**
- Consumes: Task 1-2 结构
- Produces: 完整审计器（P0+P1+P2）+ `--epubcheck` 开关

- [ ] **Step 1: 追加 P2 失败测试**

```python
class TestP2(unittest.TestCase):
    def test_epub_heading_skip(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "h.epub")
            xhtml = '<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>t</title></head><body><h1>a</h1><h4>b</h4></body></html>'
            with zipfile.ZipFile(p, "w") as z:
                z.writestr("mimetype", "application/epub+zip", compress_type=zipfile.ZIP_STORED)
                z.writestr("OEBPS/c.xhtml", xhtml)
                z.writestr("OEBPS/c.opf", '<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="u"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>t</dc:title><dc:creator>a</dc:creator><dc:language>zh</dc:language><dc:identifier id="u">x</dc:identifier></metadata></package>')
            findings = ae.audit_epub(p)
            self.assertTrue(any(f.check == "epub.heading.skip" for f in findings), f"got {findings}")

    def test_epub_opf_description_missing_is_p2(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "d.epub"); make_ok_epub(p)
            findings = ae.audit_epub(p)
            self.assertTrue(any(f.check == "epub.opf.description" and f.level == "P2" for f in findings))
```

- [ ] **Step 2: 确认失败 → 实现 P2**

`audit_epub` 追加：
```python
    # P2: 标题跳级（h1 后直接 h4）
    for n in names:
        if n.endswith((".xhtml", ".html")):
            text = zf.read(n).decode("utf-8", "replace")
            levels = [int(m.group(1)) for m in re.finditer(r"<h([1-6])", text)]
            for a, b in zip(levels, levels[1:]):
                if b - a > 1:
                    findings.append(Finding("P2", "epub.heading.skip", rel, f"{n}: h{a}→h{b}"))
                    break
    # P2: OPF description
    if opf_names:
        try:
            opf_text = zf.read(opf_names[0]).decode("utf-8", "replace")
            if "<dc:description" not in opf_text:
                findings.append(Finding("P2", "epub.opf.description", rel, "no dc:description"))
        except Exception:
            pass
```

`--epubcheck` 开关（main 追加 flag；有 jar 且开关开时对 epub 样本运行、把 ERROR 计入 P0、WARN 计入 P2）：
```python
    ap.add_argument("--epubcheck", help="path to epubcheck jar; audits all epubs with it")
```
main 中：
```python
    if args.epubcheck:
        import subprocess
        for dirpath, _d, files in os.walk(args.path):
            for name in files:
                if name.endswith(".epub"):
                    r = subprocess.run(["java", "-jar", args.epubcheck, "--quiet", os.path.join(dirpath, name)],
                                       capture_output=True, text=True)
                    out = r.stdout + r.stderr
                    errs = len(re.findall(r"ERROR", out))
                    warns = len(re.findall(r"WARN", out))
                    relp = os.path.relpath(os.path.join(dirpath, name))
                    if errs:
                        findings.append(Finding("P0", "epubcheck", relp, f"{errs} errors"))
                    if warns:
                        findings.append(Finding("P2", "epubcheck", relp, f"{warns} warnings"))
```

- [ ] **Step 3: 测试通过 + gitignore + epubcheck 下载**

```bash
python3 scripts/test_audit_ebooks.py   # 全绿
mkdir -p developer/audit-tools
grep -q 'developer/audit-tools' .gitignore || echo 'developer/audit-tools/' >> .gitignore
grep -q 'developer/export/' .gitignore || echo 'developer/export/' >> .gitignore
curl -sL https://github.com/w3c/epubcheck/releases/download/v5.1.0/epubcheck-5.1.0.zip -o /tmp/epc.zip && unzip -q /tmp/epc.zip -d developer/audit-tools/
```
（若 5.1.0 URL 失效，用 `https://github.com/w3c/epubcheck/releases/latest` 页面找最新版）

- [ ] **Step 4: 生成完整基线报告**

```bash
python3 scripts/audit_ebooks.py > developer/export/AUDIT-REPORT-baseline.md 2>&1
python3 scripts/audit_ebooks.py --epubcheck developer/audit-tools/epubcheck-5.1.0/epubcheck.jar --path developer/export/epub/books/individual 2>&1 | head -20   # epubcheck 抽单个目录先看兼容性
head -5 developer/export/AUDIT-REPORT-baseline.md
```
Expected: 基线含 P0/P1/P2 计数。基线数字写进 commit message。

- [ ] **Step 5: 提交（zhurongshuo，含 huan.yaml 声明一并）**

```bash
git add scripts/audit_ebooks.py scripts/test_audit_ebooks.py .gitignore huan.yaml && git commit -m "feat(audit): P2 checks + epubcheck integration + plugin declaration (baseline: P0=?, P1=?, P2=?)"
```
（`?` 处填实际基线数字；huan.yaml 的 ebook_exporter 声明是 Task 11 遗留未提交变更，此处一并入库）

---

### Task 4: 抽样深审（18 样本，子代理视觉检查）

**Files:**
- Create: `developer/export/audit/`（渲染 PNG + 深审笔记，gitignored 随 developer/export）

**Interfaces:**
- Consumes: Task 3 的完整审计器（基线）
- Produces: 深审发现清单（并入修复批次输入）

- [ ] **Step 1: 渲染 PDF 样本页面**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
python3 - <<'EOF'
import fitz, os
samples = [
    ("pdf/books/individual/reality-construction.pdf", "rc"),
    ("pdf/books/volumes/volume-1.pdf", "v1"),
    ("pdf/books/complete/books-complete.pdf", "bc"),
]
os.makedirs("developer/export/audit", exist_ok=True)
for path, tag in samples:
    doc = fitz.open(f"developer/export/{path}")
    for label, idx in [("cover", 0), ("toc", 1), ("chapstart", 3), ("body", 10), ("end", doc.page_count - 1)]:
        pix = doc[idx].get_pixmap(dpi=150)
        pix.save(f"developer/export/audit/{tag}-{label}.png")
    doc.close()
print("rendered")
EOF
ls developer/export/audit/
```

- [ ] **Step 2: 视觉深审（读 PNG + 分析）**

逐张读取 15 张 PNG，记录视觉缺陷到 `developer/export/audit/VISUAL-NOTES.md`。已知的检查点（侦察阶段已发现的可直接核实）：文字右缘裁切（17/82 页）、无页眉页码、破折号单字符 `—` 而非 `——`、`10^500` 未上标、标题段前距。每条缺陷格式：`| 文件 | 页 | 问题 | 严重度建议 |`。

- [ ] **Step 3: EPUB/DOCX 深审笔记**

解包 `reality-construction` 的 epub（对照 `/tmp/epubaudit/rc` 已解包内容）：确认 `epub:type` 缺失（如 chapter/cover 语义）、`dir="auto"` 滥用（zh 内容应 `zh-CN`）、CSS 无 orphans/widows。DOCX：确认 theme1.xml 无中文字体、无 settings 中的 evenAndOddHeaders。追加到 VISUAL-NOTES.md。

- [ ] **Step 4: 深审发现并入修复清单**

把 VISUAL-NOTES.md 中每条按已有 P1 检查项归位；没有对应检查项的新缺陷（如 epub:type 语义标注 → P2、`——` 双破折号 → P1 标点批）登记到 AUDIT-REPORT-baseline.md 附录。不 commit 产物（gitignored），只确保清单内容反映到 Task 5-8 的输入。

---

### Task 5: 修复批 P1-a —— 标点归一化 + PDF 换行溢出（P0 级 bug）

**Files:**
- Create: `/Users/rong.zhu/Code/zhurong/huan/.claude/worktrees/ebook-exporter/plugins/ebook-exporter/render/typography.go`
- Test: `plugins/ebook-exporter/render/typography_test.go`
- Modify: `plugins/ebook-exporter/render/ast.go`（解析后应用归一化）
- Modify: `plugins/ebook-exporter/render/pdf.go`（长串折行）

**Interfaces:**
- Consumes: Task 4 的 `Block.Text`（inline markdown 源文本）
- Produces: `TypographCJK(s string) string` —— epub/pdf/docx 三后端统一受益（在 ParseChapter 后的 DocUnit 上应用）

- [ ] **Step 1: 写失败测试**

```go
package render

import "testing"

func TestTypographCJK(t *testing.T) {
	cases := []struct{ in, want string }{
		{`他说"你好"`, `他说“你好”`},                    // 直引号成对 → 弯引号
		{`他说"你好`, `他说“你好`},                       // 单个开引号 → 左弯
		{`你好"他说`, `你好”他说`},                       // 单个闭引号 → 右弯
		{`维度(通常是 10 或 11 维)`, `维度（通常是 10 或 11 维）`}, // CJK 紧邻半角括号 → 全角
		{`abc(def)ghi`, `abc(def)ghi`},                  // 纯拉丁上下文不动
		{`一个—破折号`, `一个——破折号`},                   // CJK 间单破折号 → 双破折号
		{`A—B`, `A—B`},                                  // 拉丁间不动
		{`冒号:中文`, `冒号：中文`},                        // CJK 后半角冒号 → 全角
		{`http://example.com:a`, `http://example.com:a`}, // URL 不动
	}
	for _, c := range cases {
		if got := TypographCJK(c.in); got != c.want {
			t.Errorf("TypographCJK(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: 确认失败 → 实现 typography.go**

实现要点（逐字符状态机，不是正则替换——引号开闭需要状态）：
- `"`：维护 inQuote 栈；CJK 上下文（前后任一字符是 CJK 或全角标点）时转 `“`/`”`；纯拉丁上下文保留
- `(` `)`：前（后）邻字符为 CJK 时转 `（` `）`
- `:`：前邻 CJK 且后邻非 `/`（URL 保护）时转 `：`
- `—`：两侧均为 CJK 且未成双时补为 `——`
- inline markdown 语法保护：`[t](u)` 的 URL 部分、代码 span `` ` `` 内容跳过（状态机遇 `` ` `` 切换 skip 到下一个 `` ` ``；遇 `](` 进入 URL skip 到 `)`）
- 在 `ParseChapter` 末尾对每个 Block 的 Text/Items/Rows 逐个应用（LangZH 时；EN 内容跳过——由调用方传入 lang 或检测 CJK 存在才应用，采用后者：`TypographCJK` 内部检测含 CJK 才处理）

- [ ] **Step 3: PDF 溢出修复**

`render/pdf.go` 的段落文本发射处：对含长拉丁串（`[A-Za-z0-9^_]{20,}` 命中）的 Text，在发射前按 `len/2` 处插入零宽断行机会——gpdf 无软断行 API，采用**字符级预折行**：把超过可用宽度（约 480pt / 字宽）的长串在任意字符边界切块、块间加空格。实现为 `wrapLongLatin(s string, maxRunes int) string`（默认 maxRunes=60），在 `c.Text(inlinePlain(...))` 前应用。加测试：

```go
func TestWrapLongLatin(t *testing.T) {
	long := ""
	for i := 0; i < 120; i++ {
		long += "a"
	}
	got := wrapLongLatin(long, 60)
	if len(got) <= 60 {
		t.Fatalf("expected wrapped, got len %d", len(got))
	}
	if wrapLongLatin("正常中文段落", 60) != "正常中文段落" {
		t.Fatal("short text must pass through")
	}
}
```

- [ ] **Step 4: 测试 + 重建 + 全量重生成 + 归零验证**

```bash
cd plugins/ebook-exporter && go test ./... && go build -buildmode=plugin -o ../../release/plugins/ebook-exporter.so . && cp ../../release/plugins/ebook-exporter.so ~/.huan/plugins/ && cd ../.. && go build -o /tmp/huan-ebook ./cmd/huan
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && /tmp/huan-ebook export ebook --type all --format all --level all --force --jobs 8
python3 scripts/audit_ebooks.py --fail-on P1 | head -5   # 期望：pdf.overflow=0、*.punct=0（其余 P1 仍在）
```

- [ ] **Step 5: 视觉复核 + 提交（huan）**

重渲染 p10 确认不裁切（复用 Task 4 的渲染脚本）。commit：
```bash
git add plugins/ebook-exporter/render/ && git commit -m "fix(ebook-exporter): CJK typography normalization + PDF long-run wrapping"
```

---

### Task 6: 修复批 P1-b —— PDF 出版件（书签/页码/页眉/目录页码）

**Files:**
- Create: `plugins/ebook-exporter/render/pdfpost.go`（PyMuPDF 后处理不可行——插件不能依赖 Python。改为**纯 Go outline 注入**）
- Test: `plugins/ebook-exporter/render/pdfpost_test.go`
- Modify: `plugins/ebook-exporter/render/pdf.go`

**Interfaces:**
- Consumes: `RenderPDF` 现有结构；gpdf `Document.Header/Footer/PageNumber()`（builder.go:126,132, grid.go:446）
- Produces: `injectOutline(pdfBytes []byte, toc []OutlineEntry) ([]byte, error)`（OutlineEntry{Title string; Page int; Level int}）

**技术路线（重要决策）**：gpdf v1.0.11 无 outline API 且不可在 Go 内解析已渲染 PDF 加书签（无 PDF 解析器依赖）。采用**两遍渲染**：
- 第一遍正常渲染，同时记录每章的页码（gpdf 渲染时无法回知页码——所以改为**目录后置**：渲染完 doc 后用 gpdf 自身 `ResolvePageNumbers` 已把页码写进 pages；但章→页映射拿不到）。
- 实际采用方案：**目录页占位 + 后处理**。第一遍渲染时在 TOC 页为每章留一行（只有标题）；渲染完成后用 gpdf 的 `Document.Render` 两次渲染不可行——改为插件内**维护自己的页计数估算不可靠**。
- **最终方案（简单可靠）**：outline 用 gpdf 的低层 `pdf` 包写 `/Outlines` 对象树；章页码通过**渲染两次**获得：第一次渲染到内存、用 gpdf `pdf.Parser`（pdf/parser.go 存在）解析出每页文本、定位章标题所在页，第二次渲染时注入正确 outline + TOC 页码文本。若 `pdf.Parser` API 不便提取文本，则退化为：**目录不带页码**（链接式：gpdf 不支持内链则纯列表）+ outline 页码由章标题文本匹配 `Render` 输出的页面顺序（用 PyMuPDF 只在**审计侧**验证，生成侧接受 outline 无页码 TOC 的降级）。
  - **写计划时的落地决定**：先读 `pdf/parser.go` 的文本提取能力（grep `GetText\|ExtractText`），若 30 分钟内不通，采用降级方案并在 README 记录。_outline 对象写入_是硬指标（P1 归零必须），TOC 页码是尽力而为。

- [ ] **Step 1: 写失败测试（outline 注入）**

```go
package render

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan-plugin-ebook-exporter/content"
)

func TestRenderPDFHasOutline(t *testing.T) {
	fontPath, err := styleFindCJKFontForTest()
	if err != nil {
		t.Skipf("no CJK font: %v", err)
	}
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "o.pdf")
	if err := RenderPDF(book, content.LangZH, out, PDFOptions{FontPath: fontPath}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !bytes.Contains(data, []byte("/Outlines")) {
		t.Fatal("PDF has no /Outlines")
	}
	// 章标题应出现在 outline 标题对象中
	if !bytes.Contains(data, []byte("(")) { // outline titles are literal strings
		t.Fatal("no outline title strings")
	}
}
```

- [ ] **Step 2: 确认失败 → 实现**

`pdf.go` 渲染完成后调用 `pdfpost.go` 的 outline 注入：构造 `/Outlines` + `/Out` 项目树（每章一个，指向页对象 ref）。页对象 ref 通过 gpdf 渲染产物中的 `/Type /Page` 顺序枚举。实现者注意：这是底层 PDF 对象操作——用 gpdf 的 `pdf` 包（`object.go`/`xref.go`）解析再重写，或直接字节级操作（找到 trailer 的 `/Root`、追加对象、更新 xref）。字节级操作 ~150 行可控；优先试 gpdf `pdf` 包 API。
- 同批完成：`doc.Header(func(p *template.PageBuilder){...})` 页眉（书名小字，奇偶可同）、`doc.Footer` + `c.PageNumber()` 页码（格式 "第 N 页" 或纯数字）、TOC 条目后接页码（若两遍方案成立）。
- **注意**：`Header/Footer` 是 `Document` 方法（builder.go:126），须在 AddPage 之前设置——核对调用顺序。

- [ ] **Step 3: 测试 + 重建 + 全量 + 审计归零（本批项）**

```bash
cd plugins/ebook-exporter && go test ./... && go build -buildmode=plugin -o ../../release/plugins/ebook-exporter.so . && cp ../../release/plugins/ebook-exporter.so ~/.huan/plugins/ && cd ../.. && go build -o /tmp/huan-ebook ./cmd/huan
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && /tmp/huan-ebook export ebook --type all --format all --force --jobs 8
python3 scripts/audit_ebooks.py --json | python3 -c "import json,sys; fs=json.load(sys.stdin); print({f['check'] for f in fs if f['level']=='P1' and f['file'].endswith('.pdf')})"
# 期望：pdf.outlines / pdf.pageno / pdf.header 不在集合中
```

- [ ] **Step 4: 提交（huan）**

```bash
git add plugins/ebook-exporter/render/ && git commit -m "feat(ebook-exporter): PDF outlines, running header, page numbers"
```

---

### Task 7: 修复批 P1-c —— DOCX 出版件（TOC 字段/eastAsia/页码）

**Files:**
- Modify: `plugins/ebook-exporter/render/docx.go`
- Test: `plugins/ebook-exporter/render/docx_test.go`（追加）

**Interfaces:**
- Consumes: docxgo `NewTOCField`（docx.go:186）、`SectionBuilder.Footer(FooterDefault) (domain.Footer, error)`（builder.go:133）、`domain.Font{Name, EastAsia}` + `Run.SetFont`（run.go）、`NewPageNumberField()`（docx.go:153）

- [ ] **Step 1: 追加失败测试**

```go
func TestRenderDOCXPublicationParts(t *testing.T) {
	book := mkBook(t, content.LangZH)
	out := filepath.Join(t.TempDir(), "p.docx")
	if err := RenderDOCX(book, content.LangZH, out, DOCXOptions{}); err != nil {
		t.Fatal(err)
	}
	r, _ := zip.OpenReader(out)
	defer r.Close()
	var docXML, stylesXML string
	for _, f := range r.File {
		switch f.Name {
		case "word/document.xml":
			rc, _ := f.Open(); b, _ := io.ReadAll(rc); rc.Close(); docXML = string(b)
		case "word/styles.xml":
			rc, _ := f.Open(); b, _ := io.ReadAll(rc); rc.Close(); stylesXML = string(b)
		}
	}
	if !strings.Contains(docXML, "TOC") {
		t.Fatal("no TOC field in document.xml")
	}
	if !strings.Contains(stylesXML, "w:eastAsia=") {
		t.Fatal("no eastAsia font in styles.xml")
	}
	// 页码：footer part 存在或 PAGE 字段
	hasPage := false
	for _, f := range r.File {
		if strings.Contains(f.Name, "footer") {
			hasPage = true
		}
	}
	if !hasPage && !strings.Contains(docXML, "PAGE") {
		t.Fatal("no footer/page-number part")
	}
}
```

- [ ] **Step 2: 确认失败 → 实现**

- **eastAsia**：两种途径按可行性选一并在 commit message 说明：(a) 每个 run `SetFont(domain.Font{Name: "Noto Sans CJK SC", EastAsia: "Noto Sans CJK SC"})`——改 `docxSetRunText` 一处；(b) styles.xml 后处理注入（docxgo 不暴露 styles.xml 直改，倾向 (a)；审计只查 styles.xml 的 `w:eastAsia=` ——若 (a) 产生的是 run 级 rFonts 而非 styles 级，调整审计检查为「styles.xml 或 document.xml 任一含 w:eastAsia=」——**同步修改 audit_ebooks.py 的 docx.eastasia 检查**并更新其测试）
- **TOC 字段**：标题页后插入：`para, _ := doc.AddParagraph()` → `run, _ := para.AddRun()` → `run.AddField(docx.NewTOCField(map[string]string{"o": "1-3", "h": "1", "z": ""}))`（查 `domain.Run.AddField` 是否存在——grep `AddField` run.go；docx.go:185 注释示例就是 `run.AddField(...)`，成立）
- **页码**：`builder.DefaultSection().Footer(domain.FooterDefault)` 返回 `domain.Footer`（查其接口：`Footer.AddParagraph()`?）→ 段落 run `AddField(docx.NewPageNumberField())`，居中。若 Footer 接口不含 AddParagraph 则查 domain/footer.go 的实际方法并照用。

- [ ] **Step 3: 测试 + 重建 + 全量 + 归零**

```bash
（同 Task 6 Step 3 模板）
python3 scripts/audit_ebooks.py --json | python3 -c "import json,sys; fs=json.load(sys.stdin); print({f['check'] for f in fs if f['level']=='P1' and f['file'].endswith('.docx')})"
# 期望：docx.toc / docx.eastasia / docx.pageno 不在集合中
```

- [ ] **Step 4: Word 打开人工抽查 1 本（en + zh 各一）+ 提交（huan）**

```bash
git add plugins/ebook-exporter/render/ && git commit -m "feat(ebook-exporter): DOCX TOC field, eastAsia fonts, page-number footer"
```

---

### Task 8: 修复批 P1-d —— EPUB 字体子集化 + CSS 出版属性

**Files:**
- Modify: `plugins/ebook-exporter/render/epub.go`（CSS）
- Create: `plugins/ebook-exporter/style/subset.go`
- Test: `plugins/ebook-exporter/style/subset_test.go`
- Modify: `plugins/ebook-exporter/go.mod`（可能新增依赖）

**Interfaces:**
- Consumes: `style.ReadFontData`
- Produces: `style.SubsetFont(data []byte, text string) ([]byte, error)` —— RenderEPUB 嵌入前调用

**技术决策**：Go 侧 CJK 子集化库评估过：`golang.org/x/image/font` 无子集化；成熟方案是 Python fontTools（已装 4.60.2）或命令行 `pyftsubset`。**插件内做子集化**的可行路线：
- (a) Go 调 `exec.Command("pyftsubset", ...)` —— 违反"纯 Go 无外部进程"约束，否决
- (b) 纯 Go 实现 OTF (CFF) 子集化 —— 工作量数周，否决
- (c) **生成期外子集化**：zhurongshuo 侧准备一个预子集的字体文件（覆盖全部内容的字符集，用 pyftsubset 一次性生成，存 `developer/audit-tools/NotoSansCJKsc-Subset.otf`），插件 `fonts_dir` 指向它 —— 零代码改动，字体 14MB→按实际字符集约 2-4MB
- **采用 (c)**。命令：先收集全量字符集再子集化（见 Step 1）。插件侧唯一改动：无（FindCJKFont 已支持 fonts_dir 覆盖；zhurongshuo huan.yaml 配 `fonts_dir: "developer/audit-tools"`，且该目录放子集字体命名保持 `NotoSansCJKsc-Regular.otf` 以命中匹配规则）。**注意排序规则 PreferTTF 与多候选**：确保 fonts_dir 里只有子集字体一个候选。

- [ ] **Step 1: 生成预子集字体（zhurongshuo 侧一次性）**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
python3 - <<'EOF'
import glob
chars = set()
for f in glob.glob("content/books/**/*.md", recursive=True) + glob.glob("content/practices/**/*.md", recursive=True):
    if f.endswith(".en.md"):
        chars.update(chr(c) for c in range(0x20, 0x7F))
    else:
        chars.update(open(f, encoding="utf-8").read())
textfile = "developer/audit-tools/subset-chars.txt"
import os; os.makedirs("developer/audit-tools", exist_ok=True)
open(textfile, "w", encoding="utf-8").write("".join(sorted(chars)))
print("unique chars:", len(chars))
EOF
pyftsubset ~/Library/Fonts/NotoSansCJKsc-Regular.otf \
  --text-file=developer/audit-tools/subset-chars.txt \
  --output-file=developer/audit-tools/NotoSansCJKsc-Regular.otf \
  --layout-features='*' --glyph-names --recalc-bounds
ls -la developer/audit-tools/NotoSansCJKsc-Regular.otf
```

- [ ] **Step 2: huan.yaml 切字体目录 + 重生成 epub + 审计**

```bash
# zhurongshuo huan.yaml: ebook_exporter 下加 fonts_dir: "developer/audit-tools"
/tmp/huan-ebook export ebook --type all --format epub --level all --force --jobs 8
python3 scripts/audit_ebooks.py --json | python3 -c "import json,sys; fs=json.load(sys.stdin); print([f for f in fs if f['check']=='epub.font.subset'][:3]); print('epub size:'); import subprocess; print(subprocess.run(['du','-sh','developer/export/epub'],capture_output=True,text=True).stdout)"
# 期望：epub.font.subset 缺陷清零；epub 总体积显著下降
```
（PDF 也用同一字体源受益——PDF 嵌入本来就是 gpdf 自带子集；docx 不嵌字体不受影响）

- [ ] **Step 3: CSS 出版属性（render/epub.go）**

`epubCSS` 追加：
```css
body { line-break: strict; overflow-wrap: break-word; }
p, li { orphans: 2; widows: 2; }
```
（`line-break: strict` 实现避头尾；EBU 阅读器支持普遍）。EPUB 测试断言 CSS 含 `line-break` 与 `orphans`（在现有 epub 测试里加断言）。加审计联动确认 `epub.css.linebreak`、`epub.css.orphans` 清零。

- [ ] **Step 4: 全部测试 + 重建 + 提交**

```bash
cd plugins/ebook-exporter && go test ./... && （重建 .so + huan 流程）
git add plugins/ebook-exporter/render/epub.go && git commit -m "feat(ebook-exporter): EPUB CSS publication properties (line-break, orphans/widows)"
# zhurongshuo: git add huan.yaml && git commit -m "chore(ebook): point fonts_dir at pre-subset CJK font"
```

---

### Task 9: 终验 —— 全量审计归零 + epubcheck + 人工抽查

**Files:**
- 无新代码（运行验证 + 更新 README 限制清单）

- [ ] **Step 1: 全量审计**

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
python3 scripts/audit_ebooks.py --fail-on P1; echo "exit=$?"
# 期望 exit=0（P0/P1 全清；P2 允许残留但计数记录）
python3 scripts/audit_ebooks.py > developer/export/AUDIT-REPORT-final.md 2>&1
head -3 developer/export/AUDIT-REPORT-final.md
```

- [ ] **Step 2: epubcheck 全量（或至少 18 样本）**

```bash
python3 scripts/audit_ebooks.py --epubcheck developer/audit-tools/epubcheck-5.1.0/epubcheck.jar --path developer/export/epub 2>&1 | head -10
# 期望：0 ERROR（WARN 记入 P2 可残留）
```
（全量 epubcheck 若耗时过长，改 `--path developer/export/epub/books/individual` + volumes/complete 分跑）

- [ ] **Step 3: 人工视觉抽查**

重渲染 3 本 PDF 关键页（复用 Task 4 脚本），确认：右缘无裁切、页眉页码在位、弯引号生效。DOCX 用 Pages/Word 打开 zh+en 各 1 本确认 TOC 字段在 Word 中可刷新。EPUB 抽 1 本确认体积与内嵌字体。

- [ ] **Step 4: 更新 huan README 限制清单 + ADR 补记**

`plugins/ebook-exporter/README.md`：V1 限制清单更新（移除已修复项；新增"TOC 页码若降级未实现则记录"；字体子集化机制说明 fonts_dir 预子集路线）。commit（huan）。

- [ ] **Step 5: 终验报告 + 提交**

```bash
git -C /Users/rong.zhu/Code/zhurong/zhurongshuo add scripts/ && git commit -m "audit: final report — P0/P1 clear, epubcheck 0 errors"
```

---

## Self-Review 记录

- **Spec 覆盖**：审计脚本（P0/P1/P2 + epubcheck）→ Task 1-3；抽样深审 → Task 4；修复批 P1-a/b/c/d → Task 5/6/7/8；P2 → 随各批顺带（orphans/widows 在 Task 8，OPF description 在 README 记为遗留——spec P2 批未单列任务，理由：P2 项全部轻量且分散在各 render 文件，单独成任务粒度过碎；Task 9 终验时 P2 允许残留并在报告计数）；边界（不回写源、工具不入库、huan worktree 续作）→ Global Constraints；验收（--fail-on P1 归零 + epubcheck 0 error + 人工抽查）→ Task 9。
- **占位符扫描**：Task 6 的 outline 技术路线给了明确决策树（gpdf pdf 包优先、字节级兜底、TOC 页码降级）而非 TBD——实现者有 30 分钟探路上限和降级出口；Task 8 字体子集化三方案评估后定死 (c)，无悬空。无 "implement later" 类占位。
- **类型一致性**：`TypographCJK(s string) string`（Task 5 定义、ParseChapter 应用）；`wrapLongLatin(s string, maxRunes int) string`（Task 5 内）；`OutlineEntry{Title, Page, Level}`（Task 6 内）；`style.SubElementFont` 未定义——正确，Task 8 走配置路线无新接口；审计侧 Finding{level,check,file,message} 三任务一致。

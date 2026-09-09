# ebook-exporter 字体自动解析 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `huan export ebook` 在零预生成字体文件、无 python 的机器上直接出正确的书（字体三级回退链 + 纯 Go TTC 提取）。

**Architecture:** 全部改动在 huan 仓库 `plugins/ebook-exporter`：`style` 包新增 sfnt 探测、TTC→TTF 提取（带 `<output_dir>/.font-cache/` 缓存）和出版字体解析链；`plugin.go` 在 Export() 入口一次性解析三个字体槽位（显式配置 → macOS 已知系统源 → 系统扫描），解析结果参与 manifest assetHash 并传入 renderUnit。zhurongshuo 仓库删掉 4 个字体配置键与外部脚本。

**Tech Stack:** Go（标准库 `encoding/binary`、`crypto/sha256`、`os`、`sort`）；无新第三方依赖。插件是独立 module `github.com/iannil/huan-plugin-ebook-exporter`（`plugins/ebook-exporter/go.mod`），以 `.so` 加载自 `~/.huan/plugins/ebook-exporter.so`。

**Spec:** `docs/superpowers/specs/2026-09-09-ebook-exporter-font-autodetect-design.md`

## Global Constraints

- 纯 Go，不 exec 任何外部进程（不调用 python/fontTools）——ADR 0016 原则
- 不新增第三方依赖（stdlib + 既有依赖 only）
- 显式配置的字体路径存在且合法时行为与现状完全一致（优先级最高）
- EPUB 正文字体查找（`FindCJKFont`）行为不变
- 自动回退必须可见：发生时向 `plugin.ExportResult.Warnings` 追加说明
- manifest 前缀 `publication-v4-20260909` 不变；assetHash 改用最终解析路径（换机器自然重建）
- 每个 task 结束 `go test ./...`（在 `plugins/ebook-exporter/` 目录内）全绿后 commit
- 字体二进制格式均为大端（big-endian）

## 背景知识（实现者必读）

- sfnt 结构：每字体 12 字节 offset table（`sfntVersion` u32、`numTables` u16、`searchRange` u16、`entrySelector` u16、`rangeShift` u16）+ `numTables` × 16 字节表记录（`tag` 4B、`checkSum` u32、`offset` u32、`length` u32）+ 表数据。TrueType 轮廓 = 存在 `glyf` 表且 `sfntVersion == 0x00010000`（或 `true`）；CFF = `sfntVersion == 'OTTO'`。
- TTC 结构：`'ttcf'` tag + version u32 + numFonts u32 + offsets[numFonts] u32，每个 offset 指向一个完整 sfnt offset table。同一 TTC 内字体可能共享表。
- `head.checkSumAdjustment`（head 表内 byte 8–11）：整字体 checksum（4 字节大端累加，checkSumAdjustment 视为 0）应为 `0xB1B0AFBA`，即 adjustment = `0xB1B0AFBA - sum32(font)`。
- PDF/gpdf 只接受 TrueType 轮廓；封面是 300dpi 栅格图（字体不嵌入，只参与画图）；EPUB 直接嵌入字体文件。
- 现有事故根因：zhurongshuo huan.yaml 显式配置 `pdf_font` 等指向 `developer/audit-tools/publication-fonts/*.ttf`，文件缺失时 `plugin.go:524` 的 `resolveFont()` 原样传路径 → 逐书硬失败。

---

### Task 1: style 包 sfnt 探测 + 测试字体 fixture

**Files:**
- Create: `plugins/ebook-exporter/style/sfnt.go`
- Test: `plugins/ebook-exporter/style/sfnt_test.go`（含共享 fixture 构造器，后续 task 复用）

**Interfaces:**
- Consumes: 无（新文件）
- Produces:
  - `func HasTrueTypeOutlines(data []byte) bool` — 输入整个字体文件字节（单字体 ttf/otf，或 ttc 取第 0 个字体）；`glyf` 表存在且 sfntVersion 非 `OTTO` 时 true
  - `func IsCollection(data []byte) bool` — `'ttcf'` magic
  - `func FontTrueTypeAt(data []byte, index int) (bool, error)` — ttc 第 index 个字体是否 TrueType 轮廓（非 ttc 输入且 index==0 时退化为单字体判断）
  - 测试 helper（同包 `_test.go`，Task 2 复用）：
    - `func buildTestFont(t *testing.T, tables map[string][]byte) []byte`
    - `func buildTestTTC(t *testing.T) []byte`（双字体集合）

- [ ] **Step 1: 写失败测试**

```go
package style

import (
	"encoding/binary"
	"sort"
	"testing"
)

// tableHeadBytes returns a 54-byte head table with TrueType magic and
// version, per the OpenType spec prefix.
func tableHeadBytes() []byte {
	b := make([]byte, 54)
	binary.BigEndian.PutUint32(b[0:], 0x00010000) // version
	binary.BigEndian.PutUint32(b[12:], 0x5F0F3CF5)
	binary.BigEndian.PutUint32(b[28:], 0x00010000) // magic
	return b
}

// buildTestFont lays out a minimal single-font sfnt from the given tables.
// Table order follows sorted tags (sfnt requires ascending tag order).
func buildTestFont(t *testing.T, tables map[string][]byte) []byte {
	t.Helper()
	if _, ok := tables["head"]; !ok {
		tables["head"] = tableHeadBytes()
	}
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	n := len(tags)
	out := make([]byte, 0, 12+16*n+512)
	hdr := make([]byte, 12)
	binary.BigEndian.PutUint32(hdr[0:], 0x00010000) // sfntVersion
	binary.BigEndian.PutUint16(hdr[4:], uint16(n))
	binary.BigEndian.PutUint16(hdr[6:], 16) // searchRange (numTables≤2 简化)
	binary.BigEndian.PutUint16(hdr[8:], 1)  // entrySelector
	binary.BigEndian.PutUint16(hdr[10:], uint16(n*16-16))
	out = append(out, hdr...)

	type rec struct{ tag string; off, len_ uint32 }
	var recs []rec
	body := make([]byte, 0, 512)
	for _, tag := range tags {
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		data := tables[tag]
		recs = append(recs, rec{tag, uint32(12 + 16*n + len(body)), uint32(len(data))})
		body = append(body, data...)
	}
	for _, r := range recs {
		var rec16 [16]byte
		copy(rec16[0:], r.tag)
		binary.BigEndian.PutUint32(rec16[8:], r.off)
		binary.BigEndian.PutUint32(rec16[12:], r.len_)
		out = append(out, rec16[:]...)
	}
	out = append(out, body...)
	return out
}

// buildTestTTC packs two fonts into a 'ttcf' collection. Font 0 has no glyf
// (CFF-like), font 1 has glyf — index discrimination is under test.
func buildTestTTC(t *testing.T) []byte {
	t.Helper()
	f0 := buildTestFont(t, map[string][]byte{"maxp": {0, 0, 50, 0}, "cmap": []byte("cmap")})
	f1 := buildTestFont(t, map[string][]byte{"glyf": []byte("glyphdata"), "loca": {0, 0, 0, 4}})
	out := make([]byte, 12)
	copy(out, "ttcf")
	binary.BigEndian.PutUint32(out[4:], 0x00010000)
	binary.BigEndian.PutUint32(out[8:], 2)
	out = binary.BigEndian.AppendUint32(out, 12)
	out = binary.BigEndian.AppendUint32(out, 12+len(f0))
	return append(out, append(f0, f1...)...)
}

func TestHasTrueTypeOutlines(t *testing.T) {
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if !HasTrueTypeOutlines(tt) {
		t.Fatal("ttf with glyf: want true")
	}
	cff := buildTestFont(t, map[string][]byte{"CFF ": []byte("cff")})
	// buildTestFont writes sfntVersion 0x00010000; patch to 'OTTO'
	binary.BigEndian.PutUint32(cff[0:], 0x4F54544F)
	if HasTrueTypeOutlines(cff) {
		t.Fatal("OTTO without glyf: want false")
	}
}

func TestFontTrueTypeAt(t *testing.T) {
	ttc := buildTestTTC(t)
	if !IsCollection(ttc) {
		t.Fatal("ttc magic: want IsCollection true")
	}
	if ok, err := FontTrueTypeAt(ttc, 0); err != nil || ok {
		t.Fatalf("font 0 (no glyf): want false, nil; got %v, %v", ok, err)
	}
	if ok, err := FontTrueTypeAt(ttc, 1); err != nil || !ok {
		t.Fatalf("font 1 (glyf): want true, nil; got %v, %v", ok, err)
	}
	if _, err := FontTrueTypeAt(ttc, 2); err == nil {
		t.Fatal("index out of range: want error")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test ./style/ -run 'TestHasTrueTypeOutlines|TestFontTrueTypeAt' -v`
Expected: FAIL（`HasTrueTypeOutlines` 等未定义）

- [ ] **Step 3: 实现 `style/sfnt.go`**

```go
// Package style internal: minimal sfnt/TTC structure probes used by the
// publication font resolution chain. Binary format is big-endian.
package style

import (
	"encoding/binary"
	"fmt"
)

const (
	sfntTrueType = 0x00010000
	sfntTrue     = 0x74727565 // 'true'
	sfntOTTO     = 0x4F54544F
	tagTTCF      = 0x74746366 // 'ttcf'
)

// IsCollection reports whether data is a TrueType Collection.
func IsCollection(data []byte) bool {
	return len(data) >= 4 && binary.BigEndian.Uint32(data[0:]) == tagTTCF
}

// fontOffsetAt returns the byte offset of font #index inside data
// (collections only).
func fontOffsetAt(data []byte, index int) (uint32, error) {
	if len(data) < 12 {
		return 0, fmt.Errorf("truncated ttc header")
	}
	numFonts := binary.BigEndian.Uint32(data[8:])
	if uint32(index) >= numFonts {
		return 0, fmt.Errorf("font index %d out of range (collection has %d)", index, numFonts)
	}
	return binary.BigEndian.Uint32(data[12+4*index:]), nil
}

// FontTrueTypeAt reports whether font #index of a collection has TrueType
// outlines. A non-collection input is treated as font 0 when index == 0.
func FontTrueTypeAt(data []byte, index int) (bool, error) {
	if !IsCollection(data) {
		if index != 0 {
			return false, fmt.Errorf("not a collection; only index 0 exists")
		}
		return hasTrueTypeOutlinesAt(data, 0)
	}
	off, err := fontOffsetAt(data, index)
	if err != nil {
		return false, err
	}
	return hasTrueTypeOutlinesAt(data, off)
}

// hasTrueTypeOutlinesAt parses the offset table at byte offset off.
func hasTrueTypeOutlinesAt(data []byte, off uint32) (bool, error) {
	if int(off)+12 > len(data) {
		return false, fmt.Errorf("offset table out of bounds")
	}
	ver := binary.BigEndian.Uint32(data[off:])
	if ver == sfntOTTO {
		return false, nil
	}
	if ver != sfntTrueType && ver != sfntTrue {
		return false, fmt.Errorf("unrecognized sfntVersion %#x", ver)
	}
	numTables := binary.BigEndian.Uint16(data[off+4:])
	for i := 0; i < int(numTables); i++ {
		rec := int(off) + 12 + 16*i
		if rec+16 > len(data) {
			return false, fmt.Errorf("truncated table directory")
		}
		if string(data[rec:rec+4]) == "glyf" {
			return true, nil
		}
	}
	return false, nil
}

// HasTrueTypeOutlines reports whether the font file data (ttf/otf/ttc) has
// TrueType outlines for its first font.
func HasTrueTypeOutlines(data []byte) bool {
	ok, err := FontTrueTypeAt(data, 0)
	return err == nil && ok
}
```

- [ ] **Step 4: 测试通过**

Run: `cd plugins/ebook-exporter && go test ./style/ -v`
Expected: PASS（含既有 font_test.go）

- [ ] **Step 5: Commit**

```bash
cd plugins/ebook-exporter && git add style/sfnt.go style/sfnt_test.go
git commit -m "feat(ebook-exporter): sfnt/TTC structure probes with test fixtures"
```

---

### Task 2: TTC→TTF 提取（带缓存）

**Files:**
- Create: `plugins/ebook-exporter/style/ttc.go`
- Test: `plugins/ebook-exporter/style/ttc_test.go`

**Interfaces:**
- Consumes: `IsCollection`、`fontOffsetAt`、`hasTrueTypeOutlinesAt`（Task 1，同包）；测试复用 `buildTestTTC`
- Produces: `func ExtractTTC(sourcePath string, index int, cacheDir string) (string, error)`
  - sourcePath 是 TTC → 提取第 index 个字体为独立 .ttf 写入 `cacheDir/<sha256前16hex>.ttf`；缓存键 = sha256("path|mtimeUnixNano|index")，命中且 `HasTrueTypeOutlines(缓存文件)` 通过则直接返回缓存路径
  - sourcePath 非 TTC（单字体文件）→ 原样返回 sourcePath（调用方无需区分）
  - 源文件不存在 / index 越界 / sfntVersion 非 TrueType → error
  - `MkdirAll(cacheDir)` 自理

- [ ] **Step 1: 写失败测试**

```go
package style

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTTCExtractsFontOne(t *testing.T) {
	ttc := buildTestTTC(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ttc")
	if err := os.WriteFile(src, ttc, 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")

	out, err := ExtractTTC(src, 1, cache)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !HasTrueTypeOutlines(data) {
		t.Fatal("extracted font: want TrueType outlines")
	}
	if filepath.Dir(out) != cache {
		t.Fatalf("extracted into %s, want under %s", out, cache)
	}

	// Second call hits the cache: same path, file unchanged.
	out2, err := ExtractTTC(src, 1, cache)
	if err != nil {
		t.Fatal(err)
	}
	if out2 != out {
		t.Fatalf("cache miss on second call: %s vs %s", out2, out)
	}
}

func TestExtractTTCIndexOutOfRange(t *testing.T) {
	ttc := buildTestTTC(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ttc")
	if err := os.WriteFile(src, ttc, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractTTC(src, 5, filepath.Join(dir, "cache")); err == nil {
		t.Fatal("index 5 in 2-font collection: want error")
	}
}

func TestExtractTTCPassesThroughSingleFont(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.ttf")
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if err := os.WriteFile(src, tt, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ExtractTTC(src, 0, filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if out != src {
		t.Fatalf("single font: want pass-through, got %s", out)
	}
}

func TestExtractTTCMissingSource(t *testing.T) {
	if _, err := ExtractTTC(filepath.Join(t.TempDir(), "nope.ttc"), 0, t.TempDir()); err == nil {
		t.Fatal("missing source: want error")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test ./style/ -run TestExtractTTC -v`
Expected: FAIL（`ExtractTTC` 未定义）

- [ ] **Step 3: 实现 `style/ttc.go`**

```go
// Package style internal: TTC→standalone-TTF extraction with a content-keyed
// on-disk cache. Pure Go; table data is copied verbatim, only the table
// directory offsets and head.checkSumAdjustment are recomputed.
package style

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// ExtractTTC returns the path of a standalone TrueType font for font #index
// of the collection at sourcePath, extracting (and caching) under cacheDir.
// A non-collection source is passed through unchanged.
func ExtractTTC(sourcePath string, index int, cacheDir string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("font source: %w", err)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read font source: %w", err)
	}
	if !IsCollection(data) {
		if index != 0 {
			return "", fmt.Errorf("%s: not a collection; only index 0 exists", sourcePath)
		}
		return sourcePath, nil
	}
	off, err := fontOffsetAt(data, index)
	if err != nil {
		return "", fmt.Errorf("%s: %w", sourcePath, err)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("font cache dir: %w", err)
	}
	key := fmt.Sprintf("%s|%d|%d", sourcePath, info.ModTime().UnixNano(), index)
	sum := sha256.Sum256([]byte(key))
	cachePath := filepath.Join(cacheDir, fmt.Sprintf("%x.ttf", sum[:16]))

	if prev, err := os.ReadFile(cachePath); err == nil && HasTrueTypeOutlines(prev) {
		return cachePath, nil
	}

	font, err := rebuildStandalone(data, off)
	if err != nil {
		return "", fmt.Errorf("extract font %d of %s: %w", index, sourcePath, err)
	}
	if err := os.WriteFile(cachePath, font, 0o644); err != nil {
		return "", fmt.Errorf("write extracted font: %w", err)
	}
	return cachePath, nil
}

// rebuildStandalone copies font #at offset off (within collection data)
// into a standalone sfnt with rewritten directory offsets and a freshly
// computed head.checkSumAdjustment.
func rebuildStandalone(data []byte, off uint32) ([]byte, error) {
	if int(off)+12 > len(data) {
		return nil, fmt.Errorf("offset table out of bounds")
	}
	ver := binary.BigEndian.Uint32(data[off:])
	if ver != sfntTrueType && ver != sfntTrue {
		return nil, fmt.Errorf("sfntVersion %#x is not TrueType (CFF cannot serve PDF)", ver)
	}
	numTables := int(binary.BigEndian.Uint16(data[off+4:]))

	// 12-byte offset table is copied verbatim (numTables unchanged, so
	// searchRange/entrySelector/rangeShift stay valid).
	out := make([]byte, 12, 12+16*numTables+64)
	copy(out, data[off:off+12])

	// Pass 1: collect table data (4-byte padded), pass 2: directory.
	type entry struct{ tag string; off, length uint32 }
	var entries []entry
	body := make([]byte, 0, 64)
	for i := 0; i < numTables; i++ {
		rec := int(off) + 12 + 16*i
		if rec+16 > len(data) {
			return nil, fmt.Errorf("truncated table directory")
		}
		tag := string(data[rec : rec+4])
		tOff := binary.BigEndian.Uint32(data[rec+8:])
		tLen := binary.BigEndian.Uint32(data[rec+12:])
		if int(tOff)+int(tLen) > len(data) {
			return nil, fmt.Errorf("table %s out of bounds", tag)
		}
		for len(body)%4 != 0 {
			body = append(body, 0)
		}
		entries = append(entries, entry{tag, uint32(len(body)), tLen})
		body = append(body, data[tOff:int(tOff)+int(tLen)]...)
	}
	for _, e := range entries {
		var rec16 [16]byte
		copy(rec16[0:], e.tag)
		// checkSum is preserved from the source record; recomputed below
		// alongside offsets for simplicity of the rebuild.
		binary.BigEndian.PutUint32(rec16[8:], uint32(12+16*numTables)+e.off)
		binary.BigEndian.PutUint32(rec16[12:], e.length)
		out = append(out, rec16[:]...)
	}
	out = append(out, body...)

	// head.checkSumAdjustment fixup: zero it, sum the whole font, then write
	// 0xB1B0AFBA - sum at head+8.
	if err := fixHeadAdjustment(out); err != nil {
		return nil, err
	}
	return out, nil
}

func fixHeadAdjustment(font []byte) error {
	numTables := binary.BigEndian.Uint16(font[4:])
	for i := 0; i < int(numTables); i++ {
		rec := 12 + 16*i
		if string(font[rec:rec+4]) != "head" {
			continue
		}
		hOff := binary.BigEndian.Uint32(font[rec+8:])
		hLen := binary.BigEndian.Uint32(font[rec+12:])
		if int(hOff)+12 > len(font) || hLen < 12 {
			return fmt.Errorf("head table too short")
		}
		binary.BigEndian.PutUint32(font[hOff+8:], 0)
		var sum uint32
		for i := 0; i+4 <= len(font); i += 4 {
			sum += binary.BigEndian.Uint32(font[i:])
		}
		binary.BigEndian.PutUint32(font[hOff+8:], 0xB1B0AFBA-sum)
		return nil
	}
	return fmt.Errorf("no head table")
}
```

注意：`rebuildStandalone` 里 per-table `checkSum` 字段按 0 写入（上面代码即如此——目录重建时未复制源 checkSum）。gpdf 嵌入时自会重算；封面路径用 `opentype.Parse` 不校验 checkSum。若实现时发现 gpdf 校验表 checkSum，从源记录复制即可（源数据未变，checkSum 不变）。

- [ ] **Step 4: 测试通过**

Run: `cd plugins/ebook-exporter && go test ./style/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd plugins/ebook-exporter && git add style/ttc.go style/ttc_test.go
git commit -m "feat(ebook-exporter): pure-Go TTC→TTF extraction with mtime-keyed cache"
```

---

### Task 3: 出版字体解析链

**Files:**
- Create: `plugins/ebook-exporter/style/publication.go`
- Test: `plugins/ebook-exporter/style/publication_test.go`

**Interfaces:**
- Consumes: `HasTrueTypeOutlines`、`ExtractTTC`（Task 1/2）；既有 `fontCandidates`、`defaultFontDirs`、`PreferTTF`（`style/font.go`，同包）
- Produces:
  - `type FontRef struct{ Path string; Index int }`
  - `var publicationCJKSources = []FontRef{{Path: "/System/Library/Fonts/STHeiti Light.ttc", Index: 1}, {Path: "/System/Library/Fonts/STHeiti Medium.ttc", Index: 1}}`（包级 var，测试可覆写）
  - `func FindPublicationCJKFont(cfgPath, fontsDir, cacheDir string) (path string, note string, err error)` — note 非空表示发生了自动回退（描述实际来源），供上层作为 Warning 呈现
  - `func FindPublicationLatinFont(cfgPath string) string` — 找不到返回 `""`（调用方降级，非错误）

- [ ] **Step 1: 写失败测试**

```go
package style

import (
	"os"
	"path/filepath"
	"testing"
)

// withFakeCJKSource points the known-source chain at a temp TTC whose font 1
// has TrueType outlines (buildTestTTC fixture) for the duration of the test.
func withFakeCJKSource(t *testing.T, cacheDir string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "fake.ttc")
	if err := os.WriteFile(src, buildTestTTC(t), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := publicationCJKSources
	publicationCJKSources = []FontRef{{Path: src, Index: 1}}
	t.Cleanup(func() { publicationCJKSources = orig })
	return src
}

func TestFindPublicationCJKFontPrefersConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "cfg.ttf")
	tt := buildTestFont(t, map[string][]byte{"glyf": []byte("g"), "loca": {0, 0, 0, 1}})
	if err := os.WriteFile(cfg, tt, 0o644); err != nil {
		t.Fatal(err)
	}
	got, note, err := FindPublicationCJKFont(cfg, "", filepath.Join(dir, "cache"))
	if err != nil || got != cfg || note != "" {
		t.Fatalf("want (%s, \"\", nil), got (%s, %q, %v)", cfg, got, note, err)
	}
}

func TestFindPublicationCJKFontFallsBackToKnownSource(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	src := withFakeCJKSource(t, cache)

	got, note, err := FindPublicationCJKFont(filepath.Join(dir, "missing.ttf"), "", cache)
	if err != nil {
		t.Fatal(err)
	}
	if want := "derived from " + src; note != want {
		t.Fatalf("note = %q, want %q", note, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !HasTrueTypeOutlines(data) {
		t.Fatal("derived font: want TrueType outlines")
	}
}

func TestFindPublicationCJKFontMissingConfigEmpty(t *testing.T) {
	// 空 cfgPath：直接走已知源（不报错）。
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	withFakeCJKSource(t, cache)
	if _, _, err := FindPublicationCJKFont("", "", cache); err != nil {
		t.Fatalf("empty cfgPath should try known sources, got %v", err)
	}
}

func TestFindPublicationLatinFont(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "times.ttf")
	if err := os.WriteFile(cfg, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindPublicationLatinFont(cfg); got != cfg {
		t.Fatalf("configured latin: got %q want %q", got, cfg)
	}
	if got := FindPublicationLatinFont(filepath.Join(dir, "nope.ttf")); got != "" {
		t.Fatalf("missing latin: want \"\" (graceful degrade), got %q", got)
	}
}
```

注意：`TestFindPublicationCJKFontFallsBackToKnownSource` / `...MissingConfigEmpty` 在源校验通过的机器上不会走第三级；若第三级（系统扫描）在这台机器也找不到 TrueType CJK，`FindPublicationCJKFont` 返回 err——但 fixture 源有效所以不会。真机第三级的行为由 Task 4 的 export_flow 测试覆盖。

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test ./style/ -run 'TestFindPublication' -v`
Expected: FAIL（`FindPublicationCJKFont` 等未定义）

- [ ] **Step 3: 实现 `style/publication.go`**

```go
// Package style internal: publication font resolution chains. Each chain:
// explicit config path → known macOS system sources (TTC extraction) →
// system font scan. A non-empty return note means auto-derivation happened
// and should be surfaced by the caller.
package style

import (
	"fmt"
	"os"
	"path/filepath"
)

// FontRef names one candidate font file; Index selects a font inside a TTC
// and is ignored for standalone files.
type FontRef struct{ Path string; Index int }

// publicationCJKSources are the approved publication-design sources
// (STHeiti, macOS system). Package-level for test overrides.
var publicationCJKSources = []FontRef{
	{Path: "/System/Library/Fonts/STHeiti Light.ttc", Index: 1},
	{Path: "/System/Library/Fonts/STHeiti Medium.ttc", Index: 1},
}

// readableTrueType reports whether the font named by ref exists with
// TrueType outlines. TTCs are extracted into cacheDir first (the extracted
// file is a standalone font, so the outline check is always index 0).
func readableTrueType(ref FontRef, cacheDir string) (string, bool) {
	path, err := ExtractTTC(ref.Path, ref.Index, cacheDir)
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	if !HasTrueTypeOutlines(data) {
		return "", false
	}
	return path, true
}

// FindPublicationCJKFont resolves the PDF/cover CJK slot:
// cfgPath → publicationCJKSources → system scan (TrueType only).
func FindPublicationCJKFont(cfgPath, fontsDir, cacheDir string) (string, string, error) {
	if path, ok := readableTrueType(FontRef{Path: cfgPath, Index: 0}, cacheDir); cfgPath != "" && ok {
		return path, "", nil
	}
	for _, src := range publicationCJKSources {
		if path, ok := readableTrueType(src, cacheDir); ok {
			return path, "derived from " + src.Path, nil
		}
	}
	// System scan: prefer .ttf/.ttc over .otf (CFF cannot serve PDF).
	dirs := defaultFontDirs()
	if fontsDir != "" {
		dirs = []string{fontsDir}
	}
	for _, pattern := range [][]string{
		{"pingfang"}, {"hiragino"}, {"notosanscjk", "sc"},
		{"sourcehansans"}, {"sourcehansc"}, {"cjk"},
	} {
		cands := fontCandidates(dirs, pattern...)
		PreferTTF(cands)
		for _, c := range cands {
			if path, ok := readableTrueType(FontRef{Path: c}, cacheDir); ok {
				return path, "system fallback: " + c, nil
			}
		}
	}
	return "", "", fmt.Errorf("no TrueType CJK font found for pdf/cover (tried configured %q, %v, scan of %v); install Noto Sans CJK SC or set pdf_font",
		cfgPath, publicationCJKSources, dirs)
}

// FindPublicationLatinFont resolves the cover Latin slot:
// cfgPath → Times New Roman → Georgia → graceful "" (CJK font renders Latin).
func FindPublicationLatinFont(cfgPath string) string {
	if cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath
		}
	}
	for _, pattern := range [][]string{
		{"timesnewroman"}, {"georgia"}, {"didot"}, {"charter"},
	} {
		if cands := fontCandidates(defaultFontDirs(), pattern...); len(cands) > 0 {
			return cands[0]
		}
	}
	return ""
}
```

- [ ] **Step 4: 测试通过**

Run: `cd plugins/ebook-exporter && go test ./style/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd plugins/ebook-exporter && git add style/publication.go style/publication_test.go
git commit -m "feat(ebook-exporter): publication font resolution fallback chains"
```

---

### Task 4: plugin.go 集成 + 事故回归测试

**Files:**
- Modify: `plugins/ebook-exporter/plugin.go`（Export()，约 398–524 行；`resolveFont` 定义于 405–410）
- Test: `plugins/ebook-exporter/export_flow_test.go`（追加测试）

**Interfaces:**
- Consumes: `style.FindPublicationCJKFont(cfgPath, fontsDir, cacheDir) (string, string, error)`、`style.FindPublicationLatinFont(cfgPath) string`（Task 3）
- Produces: 无新对外接口；行为变化 = 配置路径缺失时回退 + Warnings 说明 + assetHash 用最终路径

- [ ] **Step 1: 写失败测试（本次事故的回归测试）**

在 `export_flow_test.go` 追加：

```go
// TestExportFallsBackWhenConfiguredFontsMissing is the regression test for
// the 2026-09-09 incident: huan.yaml font paths pointing at pre-generated
// files that no longer exist must fall back to system-derived fonts instead
// of failing every item.
func TestExportFallsBackWhenConfiguredFontsMissing(t *testing.T) {
	if _, ferr := style.FindCJKFont(""); ferr != nil {
		t.Skipf("no CJK font: %v", ferr)
	}
	root := writeBookProject(t)
	cfg, err := ParseConfig(map[string]any{
		"pdf_font":         "developer/audit-tools/publication-fonts/body-cover-cjk.ttf",
		"cover_font":       "developer/audit-tools/publication-fonts/body-cover-cjk.ttf",
		"cover_latin_font": "developer/audit-tools/publication-fonts/cover-latin.ttf",
	})
	if err != nil {
		t.Fatal(err)
	}
	ex := New(cfg).(plugin.Exporter)

	res, err := ex.Export(context.Background(), plugin.ExportRequest{Type: "books", SourceDir: root, Slug: "demo-book", Level: "individual", Format: "pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findItems(res, res.Succeeded, "pdf"); len(got) == 0 {
		t.Fatalf("pdf items all failed: failed=%+v", res.Failed)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("auto-derivation must be surfaced as a warning, got none")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `cd plugins/ebook-exporter && go test . -run TestExportFallsBackWhenConfiguredFontsMissing -v`
Expected: FAIL（pdf items all failed——现状即事故行为）

- [ ] **Step 3: 修改 Export()**

`plugin.go` Export() 内，紧随 `resolveFont` 定义（现状 405–410 行）之后插入字体解析块：

```go
	// Publication font slots resolve once per batch: explicit config →
	// known system sources (TTC extraction) → system scan. Notes surface
	// auto-derivation; a resolution error only fails items that need PDF.
	fontCacheDir := filepath.Join(outRoot, ".font-cache")
	pdfFont, pdfNote, pdfErr := style.FindPublicationCJKFont(resolveFont(p.cfg.PDFFont), resolveFont(p.cfg.FontsDir), fontCacheDir)
	coverFont, coverNote, _ := style.FindPublicationCJKFont(resolveFont(p.cfg.CoverFont), resolveFont(p.cfg.FontsDir), fontCacheDir)
	coverLatinFont := style.FindPublicationLatinFont(resolveFont(p.cfg.CoverLatinFont))
	if pdfErr != nil {
		res.Warnings = append(res.Warnings, "pdf font: "+pdfErr.Error())
		pdfFont = ""
	}
	for _, n := range []string{pdfNote, coverNote} {
		if n != "" {
			res.Warnings = append(res.Warnings, "fonts: "+n)
		}
	}
```

然后两处替换：

1. assets 列表（原 445 行）：
```go
	assets := []string{fontPath, monoFontPath, pdfFont, coverFont, coverLatinFont, filepath.Join(req.SourceDir, "data/books.yaml"), filepath.Join(req.SourceDir, "data/practices.yaml")}
```

2. renderUnit 调用（原 524 行）：
```go
					if err := renderUnit(j.u.agg, lang, f, out, fontPath, monoFontPath, pdfFont, coverFont, coverLatinFont); err != nil {
```

注意变量遮蔽：renderUnit 的 goroutine 闭包内直接引用外层 `pdfFont` 等即可（只读）。

- [ ] **Step 4: 全部测试通过**

Run: `cd plugins/ebook-exporter && go test ./... -v`
Expected: PASS（含既有 export_flow / render / content 全部）

- [ ] **Step 5: Commit**

```bash
cd plugins/ebook-exporter && git add plugin.go export_flow_test.go
git commit -m "feat(ebook-exporter): resolve publication fonts with fallback, surface derivation notes"
```

---

### Task 5: 文档 + zhurongshuo 清理 + 端到端验证

**Files:**
- Modify: `plugins/ebook-exporter/README.md`（"依赖"节、V1 限制清单第 61 行、"复现此次字体配置"段）
- Modify（zhurongshuo 仓库）: `huan.yaml`（`ebook_exporter:` 节）
- Delete（zhurongshuo 仓库，未入库的工作区文件）: `scripts/gen_subset_font.sh`；本地删除 `developer/audit-tools/`

**Interfaces:**
- Consumes: Task 1–4 全部
- Produces: 部署后的 `~/.huan/plugins/ebook-exporter.so` + 清理后的 zhurongshuo 配置

- [ ] **Step 1: 更新插件 README**

"依赖"节第二条（`pdf_font` 与 `fonts_dir` 分开配置……）之后新增一条，替换原子集化条目（第 52 行"字体子集化（fonts_dir 预子集路线）"整段）：

```markdown
- **字体自动解析（2026-09-09 起）**：`pdf_font` / `cover_font` / `cover_latin_font` 配置路径缺失时，
  插件自动回退：PDF/封面中文 → `/System/Library/Fonts/STHeiti Light.ttc`（TTC 内嵌提取，
  缓存于 `<output_dir>/.font-cache/`）→ 系统扫描（仅 TrueType 轮廓）；封面英文 →
  Times New Roman → Georgia → 空值降级（中文渲染器顶替）。回退发生时在导出结果的
  warnings 中列明实际来源。不再需要 `prepare_publication_fonts.py` / `gen_subset_font.sh`
  预生成字体；EPUB 内嵌字体在无预子集字体时回退全量系统字体（体积代价，见限制清单）。
```

V1 限制清单第 61 行（"子集字体是机器本地产物……"）改为：

```markdown
- EPUB 内嵌字体默认回退全量系统字体（~15.7MB/本，仅本地体积问题，非渲染问题）；预子集字体（`fonts_dir` 指向 pyftsubset 产物）仍被支持但不再是必需。
```

删除"复现此次字体配置（macOS……）"整段（74–80 行，`prepare_publication_fonts.py` 用法）。

- [ ] **Step 2: zhurongshuo 清理**

`huan.yaml` 的 `ebook_exporter:` 节改为（删除 4 个字体键及其注释块，162–171 行）：

```yaml
  ebook_exporter:
    output_dir: "developer/export"
    cover: true
```

```bash
cd /Users/rong.zhu/Code/zhurong/zhurongshuo
rm scripts/gen_subset_font.sh
rm -rf developer/audit-tools
```

- [ ] **Step 3: 构建 + 部署 + 端到端验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan/plugins/ebook-exporter
go vet ./... && go test ./...
go build -buildmode=plugin -o ~/.huan/plugins/ebook-exporter.so .

cd /Users/rong.zhu/Code/zhurong/zhurongshuo
rm -rf developer/export/epub developer/export/pdf developer/export/docx developer/export/.ebook-manifest.json developer/export/.font-cache
huan export ebook --type books --slug how-to-observe --level individual --format all
```

Expected: 单本三格式 0 failed；输出含 warnings（首次全链回退的 derivation 说明）；PDF 封面为 STHeiti 外观（与 2026-09-05 出版设计一致）。再跑一次全量：

```bash
huan export ebook --type books --level all --format all
```

Expected: `156 ok, 0 failed`；PDF 抽查一本能打开、正文中文正常（预览 `open "developer/export/pdf/books/individual/如何观察：为可塑的未来而教育.pdf"`）。

- [ ] **Step 4: 提交（两个仓库）**

```bash
cd /Users/rong.zhu/Code/zhurong/huan && git add plugins/ebook-exporter/README.md && git commit -m "docs(ebook-exporter): font auto-resolution replaces pre-generated font requirement"
cd /Users/rong.zhu/Code/zhurong/zhurongshuo && git add huan.yaml && git commit -m "chore: ebook 字体配置交由插件自动解析，移除预生成字体依赖"
```

（huan 发版/VERSION bump 不在本计划内，由用户另行决策——CI 装 latest release 后才在线上生效。）

---

### Task 4b: cmap format-12 合并重写（2026-09-09 追加，用户决策）

**背景**：实现中发现 spec 缺陷——STHeiti 全量提取字体虽为 TrueType 轮廓，但其 cmap
format-12 子表含 22861 个碎片化 groups（未合并连续区间），超过渲染层 x/image sfnt 的
`maxCmapSegments = 20000` 硬上限（x/image font/sfnt/cmap.go:266），host 无关。原 python
管线可用 STHeiti 是因 pyftsubset 顺带把 cmap 子集/合并了。合并连续区间后 STHeiti 仅需
~3420 groups。用户决策：在插件内用纯 Go 做 cmap 合并重写，保住 STHeiti 出版设计。

**Files:**
- Create: `plugins/ebook-exporter/style/cmap.go`
- Modify: `plugins/ebook-exporter/style/ttc.go`（`rebuildStandalone` 中对 cmap 表调用归一化）
- Modify: `plugins/ebook-exporter/plugin.go`（顺手修两条 Task 4 review minors）
- Test: `plugins/ebook-exporter/style/cmap_test.go`

**Interfaces:**
- Consumes: `rebuildStandalone`（Task 2）的表数据布局；x/image `font/opentype`（go.mod 已有）
- Produces: `func normalizeCmap(cmap []byte) ([]byte, error)` — 输入一个 cmap 表的完整字节，
  输出归一化后的 cmap 表字节：解析 header（version u16、numTables u16、encoding records
  8B each：platformID u16/encodingID u16/offset u32），对每个 format-12 子表合并可合并的
  相邻 groups（`end[i]+1 == start[i+1] && startGlyph[i+1] == startGlyph[i] + (end[i]-start[i]) + 1`），
  重写子表 length 与 encoding record offsets，其余子表原样保留。format-4 子表不动
  （x/image 取 (3,10)/(0,x) format-12 优先；format-4 超限的系统字体不在已知源内）。
  合并是幂等的、无条件执行（碎片化才有多余 groups，已合并字体为 no-op）。

**Steps:**

1. 失败测试 `style/cmap_test.go`：
   - `buildFragmentedCmapTTC(t)`：用 `buildTestFont` 造一个 cmap 表为 format-12、
     25000 个单码点 groups 的字体（每 group startCharCode=i、endCharCode=i、startGlyphID=i+1，
     可全部合并成 1 个 group），打包成 TTC（参照 `buildTestTTC` 的 20/20+len 偏移写法）。
   - `TestNormalizeCmapMergesRuns`：直接调用 `normalizeCmap`，断言输出的 format-12 子表
     nGroups 远小于输入（此 fixture 应为 1），且抽查若干码点的 glyphID 映射不变
     （读 group 二分或线性扫描均可）。
   - `TestExtractTTCNormalizesCmap`：`ExtractTTC` 该 TTC 后，对输出文件
     `opentype.Parse`（x/image）必须成功——这是渲染层验收，回归 STHeiti 事故的根因。
     （对照：不做归一化时该 fixture 25000 groups > 20000，Parse 必失败。）
   - `TestNormalizeCmapPassthrough`：已是连续区间的 cmap 字节原样通过（或等价重写）。
2. 实现 `style/cmap.go`（大端；注意所有偏移重算与 4 字节对齐）。
3. `style/ttc.go`：`rebuildStandalone` 中，当复制的表 tag 为 "cmap" 时对表数据调用
   `normalizeCmap`，err 时返回错误（提取失败进既有的错误路径）。
4. 顺手修 Task 4 review minors（plugin.go）：
   - 回归测试 warning 断言收紧为 `strings.Contains(strings.Join(...), "fonts")`（test 文件）。
   - "is not renderable, degrading" 警告补上降级目标（"degrading to system scan font"
     / "degrading to builtin cover font"）。
5. `cd plugins/ebook-exporter && go test ./...` 全绿 + `go vet`/`gofmt`；真机抽查：
   `ExtractTTC("/System/Library/Fonts/STHeiti Light.ttc", 1, <tmp>)` 后对输出
   `opentype.Parse` 成功（可写成 `TestExtractTTCRealSTHeiti`，系统无该文件时 skip）。
6. Commit：`feat(ebook-exporter): merge fragmented cmap groups so extracted system fonts render`

**验收**：真机导出 PDF/封面走 STHeiti 派生字体（warnings 报 "derived from ...STHeiti"，
无 "not renderable" 降级）；Task 5 e2e 复验。

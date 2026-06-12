# MEMORY — huan 项目长期记忆

> 维护规则：当检测到有意义信息（用户偏好 / 关键决策 / 经验教训 / 项目上下文变化）时智能合并；过期信息主动更新或删除。
> 最近更新：2026-06-12

## 用户偏好

- 交流与文档使用**中文**，生成的代码使用英文
- 文档放在 `docs/` 下，使用 Markdown
- 数据库 / 消息队列 / 缓存等基础设施尽量用 Docker 部署，配独立网络避免冲突
- 全链路可观测性：JSON 结构化日志（`timestamp` / `trace_id` / `span_id` / `event_type` / `payload`），装饰器 / 切面与业务逻辑解耦
- 发布产物固定在 `/release`，必须包含全量 + 增量发布所需全部文件
- 面向 LLM 的可改写性：一致分层、单一职责、显式类型、声明式配置、统一命名（`parseXxx` / `assertNever` / `safeJsonParse` 等）、小步提交
- 批量程序性改动前先备份到 `/backup`，错误数异常上升立即回滚

## 项目上下文

- **huan** = Go 静态站点生成器，阶段一目标：替代 Hugo 构建 zhurongshuo.com，输出与 Hugo 在「肉眼 / SEO / AI 三维度」无差异（甚至更好），详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) 与 [ADR 0001](../docs/adr/0001-redefine-equivalence.md)
- 关联内容项目：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`），当前仍由 Hugo 构建
- 当前分支：`master`；**stage 1 已于 2026-06-12 完成**（三维度等价标准 ADR 0001 落地，4 项必修/应修差异全解决，三维度验证管线 gate 通过）；**stage 2 phase 1（meta plainify）+ phase 2（中文排序 port + tags RSS 顺序）已完成**；phase 3（RSS items 内容差 17 文件）待启动
- `huan build` 与 Hugo byte-diff 一致率：905/2028 共有文件完全相同（44.6%，噪声 ±75，详见经验教训），剩余 5 类差异已按三维度尺子重新归类（详见 [ADR 0001](../docs/adr/0001-redefine-equivalence.md)）
- `huan serve`（HTTP + fsnotify + LiveReload）已于 2026-06-12 完成，17 commits 落地
- 仓库整洁度：无 TODO/FIXME 标记，无 backup/tmp/CI/Makefile；`pkg/` 与 `internal/{pipeline,plugin,search}/` 已删除（stage 2 时再建）

## 关键决策

- 模板引擎阶段一用 `html/template`，阶段二可插件替换
- Markdown 用 goldmark（与 Hugo 同源库）
- 配置格式 `huan.yaml`（YAML），非 drop-in 替换 Hugo
- 验证方式：`./scripts/diff-build.sh` 多模式对比（byte 雷达 + normalized / seo / ai 三维度门禁），三维度任一失败则阻断合并
- 插件架构阶段一**只预留文档**（接口未落地，连占位空目录都已删除），stage 2 增量扩展
- 加密密文仍由外部 Node.js `scripts/encrypt-content.js` 生成，huan 只负责读取与嵌入
- serve 模式用临时目录 `os.MkdirTemp("", "huan-serve-*")`，绝不污染 `docs/` 生产输出
- rebuild 用原子 swap（sibling staging dir + rename），保证 rebuild 期间无 404
- `BuildSite` 非并发安全，rebuild 通过 `atomic.Bool` 串行化 + pending 合并
- **三维度等价标准（2026-06-12，ADR 0001）**：stage 1 验收从「逐字节 100% 一致」改为「肉眼 / SEO / AI 三维度与 Hugo 输出无差异（甚至更好）」。byte-diff 保留作回归雷达，三维度对比作为门禁。允许修正型 + 扩展型「更好」（不破坏基线即可）。

## 经验教训

- **huan build 非确定性**（2026-06-12 发现）：连续两次跑相同代码的 `./huan build`，输出有约 75 个文件不同（占 3.7%）。来源未确认——可能涉及 map iteration、文件扫描顺序、或时间戳。**含义**：单次 diff-build.sh 的 same/diff 计数有 ±75 噪声。判断"改动让 diff 增/减 N 个"时，N 在 75 以内**不可信**——必须用同一份 huan-baseline 跟新 huan-output 直接对比，而不是依赖绝对数字。
- **Go template + Scratch 引用语义 bug**（2026-06-12 撞墙，未解）：模板 `{{ $x := sort ($scratch.Get "key") }}{{ range $x }}` 中，`range $x` 实际遍历的是 `$scratch.data["key"]` 的**原始 slice**，不是 sort 返回值。证据：把 sort 改成 in-place mutate input slice 也不能改变 HTML 输出。试图给 `internal/template/funcs.go` 的 `sortFunc` 加 PageSlice/[]interface{} mutation 都失败。**含义**：补 Hugo 兼容排序的"在 sortFunc 里动手脚"路径走不通。下次尝试应该从 **Scratch.Add/Set 的 slice 共享** 或 **Go template 变量赋值在 pipe + variadic + 多返回值场景下的实际行为** 入手，并配合最小复现单元测试。
- **grill 实证必须 fresh build**（2026-06-12 教训）：早期基于 `/tmp/huan-output`（几天前的旧 build）做实证，误判 huan 有"effective-constraints 缺 15 章"的 bug。重新 build 后该 bug 消失——是旧输出造成的假象。**含义**：任何关于 huan 输出的实证，必须**当场重新跑** `./huan build`，不能信任 `/tmp` 里已有的输出。
- **CURRENT_STATE.md 的「5 类差异」描述部分过期**（2026-06-12 更新）：第 2 类「Hugo date 相同时顺序不稳定」是错的——实证发现 Hugo 有稳定 tiebreaker（推断为 `Date desc → lower(Title) asc → RelPath asc`）。stage 1 收尾后所有 5 类差异已按三维度尺子重新归类，详见 [`docs/standards/equivalence.md`](../docs/standards/equivalence.md) §4 与 §5。
- **Hugo CJK WordCount 的真实算法**（2026-06-12，Phase 3 确认）：不是 `unicode.Is(unicode.Han, r)` 之类的 Unicode 属性分类，而是 `strings.Fields(plain)` + 每个 field 若 `len==runeCount` 算 1 词（纯 ASCII），否则算 `runeCount`（含 CJK）。源码在 `hugolib/page__content.go`（Hugo master）。**含义**：CJK 段落里没有空格分隔的标点也按 rune 计入（如 `"巧合"` 算 4 词，不是 2 词）。huan 的实现见 `internal/build/summary.go::CountWordsInPlain`。
- **`div` 模板函数只支持 int 的 bug**（2026-06-12 Phase 3 暴露，Phase 3.5 已修复）：原 `div`/`add`/`sub`/`mul` 是 `func(int, int) int`，无法处理 zhurongshuo 模板 `{{ div $totalWords 10000.0 }}` 的 float64 字面量。**含义**：Phase 3.5 已通过 `toFloat64` coercion helper 修复（`internal/template/funcs.go`）。Stage 1 范畴内类似隐藏 bug 仍需在 stage 2 kickoff 时检查（例如其他模板函数签名假设）。
- **Hugo summary 用块边界，不是定长截断**（2026-06-12，Phase 5.5 确认）：`summaryLength=120` 是**下限**。Hugo 找到第 N 词位置后，**forward-scan 到包含该词的 block 元素的 close tag**（`</p>` / `</h1>`~`</h6>` / `</li>` / `</blockquote>` 等）才截断。所以长文章 summary 通常**远多于 N 词**——是包含第 N 词的整个第一/二段。GitHub issue #11863 是入口。huan 的实现是 `internal/build/summary.go::TruncateHTMLToBlockBoundary`（包装 `TruncateHTMLByWords` + `commonPrefixLen` trick + 26 种 block close tag forward-scan）。
- **`commonPrefixLen` trick**（2026-06-12，Phase 5.5 总结）：要在不破坏已有函数接口的前提下，拿到「带合成 close tag 的截断结果」在原输入中的真实切点 byte offset——比较原输入与截断结果的最长公共前缀即可。第一个分歧 byte 就是切点。比让原函数暴露内部状态干净。
- **stage 1 收尾的「3 项遗留」全部是误判**（2026-06-12 grill-me 复核）：原报告说 meta description 换行、RSS items 数量差、lastBuildDate 格式差——grill-me 全量复核后发现：(a) meta description 方向反了（huan 多行、Hugo plainify 折叠，根因是 `plainify` 函数没调 `collapseWhitespace`），(b) RSS items 数量差是 grep 命令误用（minified 单行 RSS 用 `grep -c "<item>"` 数行不数 occurrence），(c) lastBuildDate 两边 byte-identical。**含义**：发布"已完成"前必须用全量分析（不依赖 sample）+ 多角度 grep 验证；"3 项遗留"实际是 5 类真实差异（meta plainify / RSS URL 编码 / books part 顺序 / body 渲染细节 / minify artifacts），详见 `docs/progress/CURRENT_STATE.md` Stage 2 候选清单。
- **plainify 修复"两次撞墙"教训**（2026-06-12 stage 2 phase 1）：第一轮按"折叠 `\n` 为空格"方向修（collapseWhitespace + TrimSpace），实证发现差异增加 300 个文件——方向反了。Hugo `tpl/template.go:StripHTML` 实际用 placeholder 保留 `</p>` / `<br>` 边界为 `\n`，只对源 `\n` 与连续 whitespace 去重。**含义**：(a) Port 上游算法前必须读真实源码，不能凭"算法应该是 X"的直觉；(b) byte-level 实证（`od -c` 或 Python repr）比 grep 更可靠——前者能看到 `\n` / 空格 / 前导/尾随，后者会因 minified 单行 / regex 转义等假象误导；(c) 第一轮修复 reset 后用 Hugo 源码 Port 完整 StripHTML 算法，diff-build.sh 4 模式全部下降（seo 983 → 699，ai 323 → 36）。
- **stage 1 收尾「3 项遗留」+ stage 2 phase 2「RSS URL 编码」连续误判**（2026-06-12 两次全量复核教训）：stage 1 收尾报告的 3 项遗留经第一次全量复核发现全误判（meta 方向反 + RSS items 数量差不存在 + lastBuildDate 不存在）；第二次全量复核（stage 2 phase 2 启动前）发现 stage 2 候选 #2「RSS URL 编码 464 文件」也是误判（实际 0 文件有单纯 URL 编码差，真实分类是 187 个 RSS items 顺序差 + 17 个 items 内容差）。**含义**：(a) "影响 N 文件" 必须基于全量分析（不是 sample），否则会因 sample 选择偏差严重高估/低估；(b) byte-level 实证（`od -c` / Python repr）比 grep 在 `\n` / 空格 / 编码差异上更可靠；(c) 同一根因（中文排序）可能在不同表面（list 顺序 / RSS items 顺序 / books part 顺序）出现，归类时需要看根因不看表象。
- **taxonomy.Build 接收已排序 pages 的契约**（2026-06-13 stage 2 phase 2 教训）：`internal/taxonomy/taxonomy.go::Build` 不重排 term 内 pages（按 input order append），所以**调用方**必须传已排序的 pages。`internal/build/build.go` 用 `taxonomy.BuildAll(site.RegularPages)`（在 `content.BuildTree` 内部经 `sortPagesDefault` 排序），不要传 `pages`（目录扫描序，未排序）。否则 tags RSS items 顺序会按扫描序乱排。该契约由 `TestBuild_PreservesInputPageOrder` 守护。**含义**：未来若新增调用 `taxonomy.Build` 的地方，必须传 sorted pages。同理其他"按页排序输出"的子系统（list / section / RSS），都应消费 `site.RegularPages` 而不是原始 `pages`。
- **Hugo 用 collate 库做 locale-aware 排序**（2026-06-13 stage 2 phase 2 发现）：Hugo `resources/page/pages_sort.go:DefaultPageSort` 在 Date 相同时用 `langs.GetCollator1(currentSite.Language())` 构建 `golang.org/x/text/collate.Collator`，按 site language 的 CLDR 表排序（zh-cn = 拼音序）。huan 原用 `strings.ToLower(Title)` 字节级 UTF-8 比较，对中文章节标题完全错序（一 < 七 < 三 < 二 < 五 vs Hugo 二 < 六 < 七 < 三 < 四 < 五 < 一）。**含义**：(a) Port 上游排序算法必须查 collator 使用，不能假设字节级比较；(b) Go 标准库 `strings.Compare` 是 byte-level，要 locale-aware 排序必须用 `golang.org/x/text/collate`；(c) DefaultPageSort 完整链含 5+ 层 tiebreaker（Ordinal / Weight0 / Weight / Date / Collator LinkTitle / Path），不能只 Port 单层；(d) **排序修复要覆盖所有调用路径**——phase 2 初版只修了 sortPagesDefault，遗漏了 taxonomy.BuildAll 的 pages 参数（build.go:173），导致 154 个 tags RSS 文件未对齐，必须实证全量验证。
- **stage 2 phase 3 拆 3 子项 + 新发现候选**（2026-06-13）：原 stage 2 候选 #4「RSS items 内容差（17 文件）」经实施发现是 3 类独立问题：(a) hidden 字段未过滤（cascade inheritance 缺失），(b) posts section 缺 _index.md 时 auto-create 时机晚 + section.RegularPages 只含直接子，(c) BuildTaxonomyContext 错用 siteCtx.RegularPages 应为 term stubs。修复后发现**新的差异**：(d) 单个 term RSS（tags/{cjk}/）的 link/guid 应 percent-encode CJK（这是 stage 1 收尾报告里"RSS URL 编码 464 文件"的真正根因——之前误判不存在）；(e) 空 tag RSS 文件未生成（huan 缺 22 个 .xml）。**含义**：(a) "影响 N 文件"的估算必须基于全量分类（按差异类型，不按文件数），同根因的差异可能在不同表面出现；(b) 一次 phase 修复可能暴露下一层差异，需要递归调查；(c) "误判"和"实际不存在"之间需要严谨的 byte-level 验证才能区分。


## 文档与导航

- 入口索引：[`docs/INDEX.md`](../docs/INDEX.md)
- 总图：[`docs/technical-plan.md`](../docs/technical-plan.md)
- 当前进展：[`docs/progress/CURRENT_STATE.md`](../docs/progress/CURRENT_STATE.md)
- 已完成报告：[`docs/reports/completed/`](../docs/reports/completed/)
- 文档规范：[`docs/standards/documentation.md`](../docs/standards/documentation.md)
- 项目根指南：[`CLAUDE.md`](../CLAUDE.md)

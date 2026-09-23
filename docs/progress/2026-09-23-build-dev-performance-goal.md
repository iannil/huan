# 持续性能优化目标

时间：2026-09-23。用户明确要求“继续优化，直到性能极致优化 /goal”。源码优化与验收现已完成，2026-09-24 纳入 v0.11.20 发布提交；最终结论见[验收报告](../reports/2026-09-23-build-dev-performance-verification.md)。下文保留按阶段的历史记录，其中“正在/待验证”描述记录当时状态。

## 完成判据

完整生产输出、主题/插件行为和错误语义保持，缓存/增量与干净构建对照通过。对剩余主要 CPU、I/O、分配热点逐项形成可复现实测，实施有净收益的方案；无净收益或正确性代价过高的方案记录证据后放弃。不能宣称硬件理论极限。

## 第一批基线

phase1 实际站点 build 中位数 2.264s；dev 保存到 HTTP 新内容 2.315s（400ms debounce），重建1.912s。源码/插件快照位于 `/var/folders/qt/v_t3_fms0ts0kfdz1bv65qyr0000gn/T/huan-perf-phase2-ixnsgg8k/`。

## 第二阶段

- [x] 搜索摘要融合：保持 `plainify → replaceRE \\s+ → truncate` 字节语义，常见 HTML 仅扫描必要前缀，复杂输入回退；fuzz 与整站输出验收。
- [x] dev 原始内容解析缓存：每轮仍读取和哈希真实字节，命中跳过 YAML；返回独立未构树 Page，预算128MiB，双语/同mtime/删除/改名/污染测试。
- [x] 真实完整插件 build CPU/alloc profile，确定页面模板与写盘后续优化。
- [x] 跨进程 Markdown 缓存：持久化纯渲染值，键包含原始/展开正文、配置及可执行文件身份；损坏回退，原子写，有界清理；冷/热开销实测后决定默认启用策略。
- [x] 对照第一批基线测量并回归，不把微基准倍数当作整站提速。

## 后续候选

模板/partial 执行热点、页面级依赖失效与插件依赖契约、搜索条目缓存、按内容跳过文件往返、跨语言流水线并发（必须隔离i18n/插件状态/目录清理）、dev静态资源快速更新。根据 profile 证据排序，不盲目堆并发。

## 第二阶段中间验证（仍在推进）

- 全量 `go test ./...` 与 build/content/dev/output/template/cmd 的 race 通过。
- 第一批快照对照第二阶段候选：5轮热build中位数2.265s→1.665s（约26.5%）；dev两组各3次有效编辑保存到HTTP中位数2.268s→1.648s（约27.3%，默认400ms debounce）。
- 首次候选3.99s含异常首次插件准备1.80s，不能归因磁盘cache；正在用预热插件+独立空cacheDir测冷cache。
- 7245产物的差异仅落在此前发现的重复slug页面及相关sitemap；本轮基线自身未覆盖全部竞争结果，仍需核实两个中文差异，不直接称全等价。
- 新目录Mkdir快速路径微基准约18–19%收益，待端到端验证；下一热点为SEO HTML分析分配。
- 修复cache排除目录的symlink路径别名，避免自触发重建。

## 后续实验与取舍

- 独立预热插件后的cache三模式中位：关闭1.937s、空cache2.075s、热cache1.739s。冷cache成本约0.14s，继续保留可关闭的默认持久cache。
- 追加4轮旧baseline：7,236稳定文件全部相同；9个既有不稳定路径的候选最终hash全部在旧baseline中精确出现。9个路径为8个重复slug产物及1个sitemap条目顺序变化。证据已存artifacts。
- partial Builder减少复制：Renderer微基准2419→2154ns，2633→2033B，49→47alloc。
- SEO复用src、惰性正文抽取：完整文件微基准119→111µs，216→168KB；446,482次差分fuzz通过。
- 拒绝SEO仅解析head即no-op：完整body深度超过512时旧html.Parse会失败并走不同注入路径，只看head不能保证输出等价；保留全DOM语义。
- 正在测试有界模板worker保留，确认GC后的重复Clone成本及RSS代价。

### 模板worker保留实验：不保留

将sync.Pool换成容量GOMAXPROCS的非阻塞idle channel，GC后保留clone，回归和race均通过。隔离整站4轮有效样本中位1.62473→1.62344s（仅0.08%，视为无确定提速）；峰值RSS中位722.3→702.4MB，范围重叠。已恢复原sync.Pool，保留有明确收益的partial Builder。实验数据见artifacts/2026-09-23-worker-retention-experiment.json。

### 最终候选冻结前

磁盘命中提升到内存时复用已经独立解码的不可变字符串，普通渲染值入缓存仍逐字段Clone，防止小substring保留大buffer。新增差分分配回归旧实现30次对30次失败，优化后少3次复制；build相关race通过。最终profile使用临时源码副本与匹配插件，不在生产入口加入采样逻辑。

### 第二次profile后的复制消除

warm profile显示总累计分配4.11GiB；重要剩余临时复制在compare（195MiB）、Raw/Expanded哈希（98.5MiB）、disk.ReadAll增长（216MiB）、Minifier.String（357MiB）。继续逐项验证：

- compare精确string/template.HTML直接取值，其他保留fmt语义；196,372次fuzz、template race通过。
- Raw==Expanded复用digest，约58KiB文本46→23µs，128→64KiB临时分配；仍先展开shortcode。
- disk Stat+有界一次预分配，128KiB读取43.6→20.9µs、302→140KiB；多读最多1字节，变化回退cache miss。
- Writer使用标准库File.WriteString，保持os.WriteFile flags/权限/错误顺序，64KiB写入减少65,536B临时分配，I/O耗时近似持平。
- Minifier输出Builder减少一次完整copy，但micro慢约2–3%，正在独立整站+RSS A/B决定是否保留；strings.Reader导致上游ReadAll增长已拒绝。

### Minifier输出Builder实验：不保留

隔离整站4轮有效样本：原M.String中位1.61690s，Builder1.61477s（仅0.13%）；峰值RSS中位577.5→600.9MB，反而升高。累计临时分配下降没有转化为可确认的耗时或峰值内存收益；Builder容量保留可能影响存活内存，但未单独证明因果。已恢复原minify.go并移除本次实验测试，不改变依赖；保留compare/hash/disk读取/Writer等已验证复制消除。原始数据见artifacts/2026-09-23-minifier-output-experiment.json。

### 主题存在判断

剩余compare默认分支150MiB中找到明确调用点：books/practices/products三处ne Page nil会格式化整个Context。已证明数据流只有*Context或nil，仅将三处主题表达式改为not (not Page)，不更改core其他值的格式化/比较语义。nil、typednil、空/完整Context以及GetPage未命中返回空Context的现有行为对照通过，待最终整站byte gate。

## 最终验收

2026-09-24T00:00+08:00：最终build 2.274→1.648s，dev保存到HTTP 2.272→1.633s；全部验收通过，明确记录缓存内存代价与未保留实验。源码工作已收敛，未执行Git提交/发布；报告放在docs/reports供验收，提交后再按文档规范归档。

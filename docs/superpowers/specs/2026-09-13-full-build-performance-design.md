# zhurongshuo 全量构建提速设计

日期：2026-09-13。范围：huan 的共享全量构建链路，覆盖 build、dev 首次启动及 dev 的全量重建。

## 问题与基准

使用已安装的 huan 0.11.12（e8ae32d），对 zhurongshuo 执行 `huan build --source /Users/rong.zhu/Code/zhurong/zhurongshuo --destination /private/tmp/huan-build-speed-baseline-20260913`。

单次测量：CLI wall time 13.42 秒，user 16.54 秒，sys 2.40 秒；多语言流水线报告 13.388 秒。中文 1,408 页、3,775 文件，英文 1,355 页、2,812 文件。内容目录约 43 MB、2,712 个被加载页面。本地媒体目录为空，配置没有图片处理插件。这是初始观测，尚不能据此分配各阶段耗时或承诺提速比例。

已确认的结构性成本：翻译清单预扫描完整解析内容一次，每个语言再解析全部内容一次；翻译校验和 data 加载按语言重复；Writer 用全局锁包住压缩、canonify 和文件 I/O；每次模板 Render 都 Clone 完整模板工厂。页面 HTML 已按 GOMAXPROCS 并行，Markdown 仍串行。

## 方案选择

选择先计时、消除重复加载和缩小 Writer 锁范围。它服务共享全量路径，不改变语言调度、模板语义或插件接口。

模板执行复用作为后续候选：潜在收益可观，但目前 Clone 隔离 site 与 partial 闭包，不能直接缓存已执行的模板。Markdown 并行也需先验证 shortcode 和 renderer 并发安全。本批次仅测量这两项，不修改它们。

语言间并行、持久化构建缓存、partialCached 语义及主题导航缓存另行设计。语言并行当前受全局 i18n bundle 与根输出目录清理约束；持久缓存需要额外失效规则。

## 计时与观测

build 和 dev 增加可选 `--timings`，默认输出保持现状。记录完整 CLI 构建总耗时，以及配置/插件准备、共享输入准备、每语言内容准备、Markdown、树与 taxonomy、模板与输出清理、contexts、页面渲染与写入、feeds/specials、静态资源、各输出后处理 hook、主题 BeforeRender/AfterRender、图片后处理耗时。dev 每次重建打印独立报告，重建总耗时包含目录交换。

内部使用每次构建独立的计时收集器，不依赖包级全局计时状态。并行页面阶段报告 wall time，不能把各任务耗时相加当作流水线耗时；嵌套 hook 时间标注为子项，不能重复求和。报告通过当前 Logf/命令输出通道发送，不改变插件接口和 Result.Duration 的既有含义。报错时也输出已完成及失败阶段耗时。

## 一次加载，按语言隔离

BuildMultiSite 创建仅在单次调用内存在的原始输入快照：原始 pages、data、翻译可用清单、翻译校验结果。清单从已加载页面生成；校验只执行一次，严格模式及警告语义保留。不存在跨 build 或跨 dev 重建复用旧输入。

共享快照通过 build 包内部入口传入各语言 pipeline，不增加可由插件调用的新接口。各语言先取得独立页面副本，再沿用现有 PageFilter、OnContentLoaded、Markdown、BuildTree 等步骤。复制 Tags、Keywords 及 Cascade 内可变成员；原始快照不包含构建后的 Parent/Pages/Sections 关系，不能复制其他语言的树状态。data 的 YAML map/slice 递归复制，避免模板或 hook 写入影响其他语言。

中性 gallery 页面也必须复制，不能因在两个语言中使用就共享 Page 指针。单语言 BuildSite 保持原有加载路径。PipelineCache 的内容填充能力保留，缓存只接收当前语言 pipeline 的独立页面对象；不用现有 ContentCache 替代输入快照，它目前是面向 JIT 的写入缓存。

保留默认语言先构建、每语言输出清理、staticExclude/staticRoot、hreflang、draft/future/expired/build 指令和各语言 hook 调用次序。配置加载、页面读取或解析失败仍按原有阶段错误语义返回，不为了提速吞掉错误。

## Writer 并发边界

不同输出路径可并行执行压缩、canonify、MkdirAll 和写文件。统计锁仅用于成功写入后的计数及 Stats 快照。所有 Write、WriteBytes、WriteBytesPath、copyFile 使用一致的同路径互斥规则，避免同一个文件出现交错写入。

同路径锁采用固定数量的路径哈希分片，避免随文件数增长永久保留锁表；哈希冲突只降低并发度，不影响正确性。保留当前阶段顺序，尤其 project static 覆盖 theme static，以及后处理在页面写入完成之后执行。并发阶段本来没有确定的同路径胜出顺序，不引入新的覆盖优先级承诺。

minifier 和 canonify 配置在构建开始后保持稳定。实施前读取项目已锁定依赖的并发契约和实现，确认 minify 注册后的实例与各注册处理器可并发使用。若不能确认，则对压缩保留独立锁，文件 I/O 和 canonify 仍退出统计锁；不直接移除所有保护。Setter 仍要求在开始写入前调用。

## 测试与验收

增加有意义的测试：语言间页面切片、Cascade、data 与 hook 修改隔离；中性页面双语渲染；严格翻译校验及读取失败；不同路径并行写入、同路径输出完整性、成功/失败写入统计准确性。对相关包执行常规测试和 race 检查，并运行项目要求的既有检查。

真实性能验收使用同一机器、同一内容快照、同一插件与版本配置，每个版本执行三次全量构建取中位数。输出写到新的临时目录；记录端到端耗时、阶段耗时和峰值内存。以相同输入分别执行父版本和修改后版本，比较文件集合、文件数与内容。对动态日期字段仅做具体、明确的归一化，其余差异必须解释并修复；不能只以页面数相等作为产物一致性的证据。

dev 验证首次双语输出和一次内容修改后的全量重建，确认 LiveReload 注入、失败保留旧输出、目录交换行为不变。站点源文件不用于持久修改；需要触发修改的场景使用临时站点副本。

本批次成功标准：两处确定的结构性成本被移除，相关测试及一致性验收通过，全量构建中位耗时降低，内存变化有记录。若实测没有降低，分析原因并调整设计，不宣称提速成功。模板或 Markdown 仍占主要耗时时，依据计时结果提出下一批次设计。

## 涉及位置

- cmd/huan/main.go、dev.go：计时选项及端到端报告。
- internal/build/build.go、pipeline*.go、multisite.go：计时分段、原始输入快照及单语言内部入口。
- internal/content/page.go 与 config 的 Cascade 定义：复制边界参考，复制实现优先放在 build 包内。
- internal/output/writer.go：统计锁与路径分片锁。
- 对应包测试及构建使用文档：并发、隔离、计时选项和验收说明。

设计自检：无待定实现范围；模板/Markdown 的观测与后续实施边界明确；不改变插件接口、语言调度及输出清理规则。用户已同意整体方向，本文待用户审阅后进入 writing-plans。

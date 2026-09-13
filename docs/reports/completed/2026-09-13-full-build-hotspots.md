# 第二轮全量构建热点诊断 — 2026-09-13

## 范围与方法

基于 master 19dc73f，在临时源码副本插入七项 feeds/specials 子阶段计时及 runtime/pprof CPU 采样。诊断代码仅保留在临时副本，不修改产品代码或已安装 CLI。使用上一轮固定站点 /private/tmp/huan-perf-txye40p6/site、空 HUAN_HOME，重新构建匹配的主程序和全部八个插件。

原始材料：/var/folders/qt/v_t3_fms0ts0kfdz1bv65qyr0000gn/T/huan-profile-grf9helk，包含 source、probe-huan、plugins、valid-build.log、valid-cpu.pprof、valid-output。首次插件路径错误的 build.log/cpu.pprof 无效，全部排除。

一次有效采样：完整构建阶段 7.366 秒，CLI 总计 13.060 秒，其中新编译插件首次 registry/theme preparation 5.693 秒。主题加载成功，51 个模板，中文 1408 页、英文 1355 页。此次为诊断样本，不是新性能基准，不评估提速或输出等价。

## 子阶段 wall time

| 项目 | 中文秒 | 英文秒 |
|---|---:|---:|
| 标签页 | 0.793 | 0.039 |
| 空分类页 | 0.002 | 0.002 |
| 分页首页 | 0.116 | 0.100 |
| 404 | 0.001 | 0.002 |
| sitemap | 0.011 | 0.012 |
| 搜索索引 | 0.311 | 0.883 |
| AI 输出 | 0.078 | 0.094 |

## CPU profile

采样总 CPU 15.46 秒，持续 13.20 秒。累计函数时间相互包含且包含并行工作，不可相加或直接作为 wall time 改善估计。

- Writer.Write 累计 6.51 秒；Canonify 累计 3.18 秒，其中 canonifySegment 2.80 秒。
- MkdirAll 累计 2.43 秒；Stat 2.24 秒，嵌套在前者内。目录重复检查值得单独验证。
- 搜索索引累计 1.02 秒；replaceREFunc 0.60 秒、plainify 0.36 秒，存在嵌套。
- html/template.Clone 累计 0.35 秒（2.26% 总 CPU），不优先重构。

源码交叉检查：Canonify 对 code/pre 区域分段，逐段执行三种 URL 正则，其中两种回调再次匹配捕获组；区域正则每次调用编译。搜索模板 index.searchindex.json 对每篇全文 Content 执行 plainify、replaceRE 空白归一化，然后 truncate 800；replaceREFunc 每次调用编译正则。Writer 各入口写入前调用 MkdirAll。

## 建议

优先设计保持原输出语义的 Canonify 优化及搜索文本处理优化；随后验证 Writer 生命周期内的成功目录创建复用。Markdown 有界并行作为独立下一步，模板复用及双语言并行暂不优先。

任何正式改动均需保留 URL 引号/裸值、协议相对地址、code/pre、JSON-LD 行为及搜索空白、Unicode 截断、标签空数组和条目顺序。通过针对性行为测试、实际站点文件哈希比较、相同环境至少三次交替测量及峰值内存评估后才能报告收益。现有重复 slug 和 sitemap 顺序问题沿用上一轮精确记录，不新增宽泛差异豁免。

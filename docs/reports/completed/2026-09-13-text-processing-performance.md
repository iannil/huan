# 第二轮全量构建性能验收 — 2026-09-13

基线为上一轮完成的 master `19dc73f`，候选运行时代码 `ec9d8af`。三轮交替实测全量构建耗时中位数 **7.29 秒 → 6.68 秒，减少 8.4%**。本轮基线与上一轮不同，不混用两轮数字计算累计提速。

## 实现

- `7a20987`：复用固定 code/pre 正则，从捕获位置直接重写 URL，避免每个匹配再次执行正则。保留原匹配顺序、协议相对地址、code/pre 边界、首页 generator、JSON-LD 及裸根路径特殊 `$` 展开行为。
- `ec9d8af`：复用 stripTags 正则；仅为搜索使用的空白正则和单空格替换增加字节扫描快速路径。空白集合严格等同 Go 正则 ASCII 集合，排除 VT/NBSP/CJK 空白；其他模式、替换字符串及无效正则走原路径。
- 搜索模板、plainify 段落和空白语义、HTML 返回类型、800 字截断、条目顺序、标签字段保持不变。无依赖升级、通用正则缓存、语言并行或目录缓存。

## 环境和结果

Go 1.26.2 darwin/arm64，常规编译参数。分别重建主程序及八个匹配插件，空 HUAN_HOME 排除安装插件覆盖。固定站点 `/private/tmp/huan-perf-txye40p6/site` 的 2,749 个源文件 SHA256 与上一轮 manifest 完全相同。同一 cwd、独立输出目录，基线及候选各预热一次插件加载（排除），之后六次按 baseline/candidate 交替串行执行，用 `/usr/bin/time -l` 测量。

| 版本 | 轮次 | real 秒 | 峰值 RSS 字节 |
|---|---:|---:|---:|
| baseline | 1 | 7.29 | 394706944 |
| candidate | 1 | 6.65 | 412270592 |
| baseline | 2 | 7.21 | 394117120 |
| candidate | 2 | 6.68 | 373866496 |
| baseline | 3 | 7.29 | 383844352 |
| candidate | 3 | 6.71 | 379404288 |

峰值 RSS 中位数 394,117,120 → 379,404,288 字节（375.86 → 361.83 MiB，约减少 3.7%）。三轮且单次 RSS 波动较大，不据此保证普遍内存改善。

## 阶段中位数（秒）

父阶段包含子阶段，不可重复相加。

| 阶段 | 基线 | 候选 |
|---|---:|---:|
| zh-cn/markdown | 0.827 | 0.821 |
| en/markdown | 1.142 | 1.139 |
| zh-cn/pages render + write | 0.745 | 0.736 |
| en/pages render + write | 0.768 | 0.781 |
| zh-cn/feeds + specials | 1.290 | 1.230 |
| en/feeds + specials | 1.095 | 0.572 |
| zh-cn/static + finalize/hook seo_injector | 0.615 | 0.605 |
| en/static + finalize/hook seo_injector | 0.538 | 0.541 |

## 兼容性和检查

- 六次输出为相同 6,591 个路径，完整文件 SHA256 比较通过。baseline-1 和 candidate-2 全部文件逐字节哈希一致。
- 其他样本差异仅为上一轮已证实的四个双语言重复 slug 目录内 index.html/index.md（八个路径）；每个完整哈希均属于上一轮精确记录的合法覆盖赢家。两个 sitemap 的完整 XML 子节点集合、根属性及文本验证通过。本轮没有新增差异豁免，不归一化 HTML、日期或搜索内容。
- 候选 `go test ./...` 通过（session 81042），`go vet ./...` 退出 0。output/template 各自普通和 race 测试通过。初始工作树缺少 git-ignored admin/dist 导致 setup 失败，复制原仓库未修改嵌入资源后完整基线测试通过（session 92492）。
- 两项改动均独立通过规格和质量审查，没有待修问题。
- 微基准：Canonify 约 1.50 → 1.43 ms/op、1512 → 635 allocs；搜索空白快速路径约 125 µs/op，对照预编译正则约 996 µs/op。微基准不等同全站收益。

## 证据与范围

原始材料 `/private/tmp/huan-text-performance-records` 包含基线 git archive 源码（补齐未修改 admin/dist）、基线/候选主程序及匹配插件、六次构建日志、完整 manifests.json、summary.json、stage-medians.json、benchmark.py、verify.py、summarize.py、各任务 brief/report/review/diff。所有站点源文件仍与上一轮固定快照相同。

前置诊断见[热点报告](2026-09-13-full-build-hotspots.md)。本轮不更新已安装 CLI 或全局插件、不安装发布、不修改真实站点。改动位于 build/dev 共用的输出和模板函数；本轮没有单独测量 dev 编辑重建，不报告 dev 提速百分比。

# build / dev 持续性能优化验收

验收时间：2026-09-24T00:00+08:00。延续第一批性能优化。源码与验收完成，工作区未提交、未发布；原有脚注和主题 CSS 修改保留。目标收敛到现有完整输出与模板/插件兼容边界内已验证的优化，不宣称硬件理论极限。

## 整站结果

Apple M5 Max、Go 1.27.1；真实双语站点2,959个Markdown、7,245个输出文件。基线为第一批完成后的源码/插件快照，候选为本次最终实现和匹配插件。

| 指标 | 第一批基线 | 最终候选 | 耗时减少 |
|---|---:|---:|---:|
| 热缓存完整build，CLI全程 | 2.274s | 1.648s | 27.5% |
| dev重建执行 | 1.870s | 1.231s | 34.2% |
| 保存到HTTP新正文，含400ms防抖 | 2.272s | 1.633s | 28.1% |

build预热后交替5轮；dev按基线/候选/基线/候选4个会话，每会话预热后测3次修改并检查HTTP标记。计入完整模板、聚合、静态文件与全部插件，未降低防抖时间。测试串行进行，不与其他重型基准竞争CPU。HTTP结果不含浏览器绘制；没有清理操作系统文件缓存。

首次加载新编译插件受macOS校验影响，最终build候选预热轮3.563s，不纳入热构建结论。另用已预热插件、每轮独立空cacheDir测3种模式，排除首轮后中位数：关闭缓存1.855s、建立空缓存1.987s、热缓存1.687s。首次写缓存有约0.13s成本；这些是独立样本组，不能混用成同组提速率。

## 内存代价

独立使用macOS time测量，预热后各3轮：

| 模式 | CLI中位 | 峰值RSS中位 |
|---|---:|---:|
| 第一批基线 | 2.323s | 477.7MiB |
| 最终默认缓存 | 1.646s | 563.7MiB |
| 最终 --noCache | 1.837s | 451.3MiB |

默认缓存以额外内存换重复构建延迟，实测build峰值中位增加约85.9MiB。此表不是dev常驻RSS；缓存预算也不是进程RSS上限。

## 保留的实现

- 搜索摘要融合为searchExcerpt，保持原plainify、空白压缩、truncate语义，常见长正文扫描所需前缀后停止；复杂HTML、占位符和非法UTF-8走原组合回退。
- Canonify属性URL改为线性扫描，保留三遍执行顺序、code/pre排除、异常属性、含美元符号的baseURL及JSON-LD无效数值语义。
- dev解析缓存每次仍读取实际原始字节并hash，跳过未变化frontmatter解析；每次返回独立Page及版本，不保留构树/插件修改。预算128MiB。
- Markdown持久缓存使用原文、展开正文、完整Markup配置及可执行文件身份；shortcode每轮仍展开。128MiB内存层、512MiB磁盘记录预算；损坏/缺失回退重算，原子发布，独占writer lease，写前预留包含临时文件的容量。Linux/macOS支持写lease，其他平台保留内存及只读能力。
- 磁盘记录按已验证文件大小一次预分配，最多多读1字节；增长/截断回退miss。解码字符串直接提升到内存，普通渲染值仍Clone以限制substring底层数据保留。Raw等于Expanded时复用摘要。
- 新输出叶目录优先Mkdir，失败回退MkdirAll；Write字符串路径使用File.WriteString，保持权限、截断及写/关闭错误语义，消除整页byte复制。
- partial使用strings.Builder；比较函数仅为精确string/template.HTML类型绕过fmt，其他保留原语义。主题三处Page存在判断改用双not，避免格式化整页。
- SEO每文件仅转换一次输入string，仅在缺少描述标签时提取正文；继续使用原完整DOM解析器。
- 自定义缓存目录支持源目录内使用，并正确排除路径别名；serve别名继承dev flags。基准脚本逐轮保留manifest，已知不稳定路径使用精确历史证据。

没有更换第三方依赖版本。所有页面模板、聚合与插件仍执行，不是页面级增量跳过。

## 正确性

- go test ./...通过；build/content/dev/output/template/cmd相关race通过；SEO插件独立race通过。
- 差分fuzz：摘要978,055次、Canonify329,765次、SEO446,482次、比较196,372次通过。局部微基准不作为整站倍数。
- 7,236个稳定产物每轮SHA256一致，无新增或缺失文件。其余9个既有不稳定路径：8个重复slug产物和en/sitemap.xml顺序差异；最终所有候选hash均在本次或已保留的旧基线样本中出现。没有宣称7,245个文件全部稳定逐字节相同。
- 缓存测试覆盖内容/配置/shortcode变化、损坏、错误key、预算、并发writer/read-only、临时文件清理、同mtime修改、改名、删除和Page污染；双语缓存构建对照干净构建一致。
- 实际serve小站将cacheDir置于source内，一次修改只触发一次重建，HTTP返回新标记；正常退出。

## 实验取舍与收敛边界

| 未保留的方案 | 证据 |
|---|---|
| 有界模板worker替换sync.Pool | 整站1.62473→1.62344s，仅0.08%，内存样本范围重叠 |
| Minifier输出Builder | 整站1.61690→1.61477s，仅0.13%；RSS577.5→600.9MB，未确认净收益 |
| Minifier直接strings.Reader | 上游触发ReadAll增长，分配反而增多 |
| SEO只解析head并提前返回 | 正文超过512层时完整parser失败，旧输出行为与head-only不同 |

多轮profile后的剩余主要成本是完整输出的文件系统操作、html/template执行/转义、压缩和完整HTML解析。最后一轮复制优化后的阶段采样（主题3处存在判断修改之前）累计分配约3,816MiB，系统调用占CPU采样约67%；并发累计CPU不是墙钟，累计分配不是峰值内存。

进一步大幅降低延迟需要页面/模板依赖失效、插件依赖契约或输出复用设计。任意模板/插件可访问全站数据并改写文件，直接跳过页面、复用最终文件或换解析器不能保证当前完整输出与错误语义。本轮不保留无实测净收益的复杂度，也不把这些架构路线宣称为已经实现。

## 使用与复现

默认build/dev启用可复用缓存。--noCache关闭复用；--cacheDir指定缓存根，其下使用markdown子目录，默认$HUAN_HOME/cache/markdown。可执行文件内容改变会使用新namespace；旧记录受统一预算淘汰。缓存故障回退内存/重算，不阻断构建。

使用scripts/bench-build.py和scripts/bench-dev.py传入source、baseline/candidate及各自plugins路径；完整参数示例见第一批报告。build可用--known-instability读取已验证历史JSON，仅允许精确路径，新增/缺失文件仍失败。

机器可读结果：[最终数据](artifacts/2026-09-23-build-dev-final.json)。独立实验：[worker](artifacts/2026-09-23-worker-retention-experiment.json)、[Minifier](artifacts/2026-09-23-minifier-output-experiment.json)、[旧基线差异证明](artifacts/2026-09-23-build-output-collision-verification.json)。

原始最终build工件：huan-build-bench-k3nrmp97；dev：huan-dev-bench-wp9rkvqm；内存：huan-final-memory-bench-8vrfsfy0；缓存模式：huan-cache-modes-t1ig1sfj，均位于本机临时目录。临时程序不作为发布产物，持久数据保存在上述artifacts。

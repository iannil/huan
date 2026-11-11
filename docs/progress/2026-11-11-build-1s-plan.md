# huan build zhurongshuo → 1s 进展（Phase A）

时间：2026-11-11 22:40+08:00
目标口径：全量干净 build（`--noCache`），中位 <1.0s（≥5 轮），输出等价。
状态：**Phase A + B1 完成，未达标**；当前中位 **3.21s**（原始 4.84s，累计 -34%），受外部负载影响。

## 1. 基线与本轮结果

环境：Apple Silicon（18 逻辑核），Go 1.27.1，站点 `../zhurongshuo`（双语 zh-cn + en）。

| 阶段 | 中位耗时 | 产物文件 | 备注 |
|---|---:|---:|---|
| 优化前（2026-11-11 22:05） | 4.84s | 10261 | 5 轮 4.04/4.84/5.01/4.28/5.32 |
| A1 后（22:18） | 3.27s | 8763 | 5 轮 1.81/2.17/3.68/3.27/3.46，波动大 |
| A2/A3 后（22:40） | 3.79s | 8763 | 5 轮 3.51/3.79/3.60/4.08/3.91，load1≈10.5 |
| B1 GOGC 后（22:45） | 3.21s | 8763 | 5 轮 3.54/3.13/3.28/3.21/3.05，load1≈19 |
| **低负载同条件 A/B（22:57）** | **before 1.86s → after 1.72s** | 10261 → 8763 | 同二进制交替 3 轮，load1≈3.4–6.1 |
| 低负载 5 轮（22:58） | **1.73–1.75s** | 8763 | load1≈5.7–7.2，含 B2 正则缓存 |

**重要测量前提（必读）**：22:05–22:45 的测量期间机器 load1 约 **9.7–19**（外部任务占用），因此那批 3.2–4.8s 的数字**不可直接比较**。22:47 之后 load1 降到 3.4–7.2，同条件 A/B 显示真实收益为 **1.86s → 1.72s（-7.5%）**，不是早期表面上的 -60%。

**同条件 A/B（git HEAD 构建的旧二进制 vs 当前二进制，交替 3 轮）**

| 轮次 | 改动前 | 改动后 |
|---|---:|---:|
| 1 | 1.86s | 1.72s |
| 2 | 1.83s | 1.71s |
| 3 | 1.90s | 1.75s |
| 中位 | **1.86s（10261 文件）** | **1.72s（8763 文件）** |

低负载下 `--noCache` 全量构建当前中位 **1.73–1.75s**，落盘下界 **0.67s**。

## 2. 已实施改动

### A1 en 静态去重（收益明确）

- 现象：`en.staticExclude: ["en/"]` + `staticRoot: "en"` 仍把 1502 个共享静态（12.6MB）再拷进 `docs/en/`，与根目录内容逐字节相同；模板引用的是根绝对路径 `/css` `/js`，`docs/en/css` 无人访问。
- 改动：`internal/config/languages.go` 新增 `skipProjectStatic`；`internal/build/pipeline_write.go` 跳过 project static 拷贝但保留 `staticRoot` 重映射；`zhurongshuo/huan.yaml` 的 `en` 设 `skipProjectStatic: true`。
- 效果：`en/static copy` **≈350ms → 1–2ms**；产物 **10261 → 8763**；`docs/en/llms.txt` 仍由 staticRoot 生成。

### A2 i18n 去全局依赖（并行前置）

- `funcs.go` 新增 `FuncMapWithI18n(baseURL, bundle)`，`i18n`/`T` 闭包捕获 bundle；`FuncMap` 保留旧签名走全局回退。
- `context.go` 的 `Renderer.newWorker` 现在把 `Renderer.funcMap` 重新绑定到克隆模板上——此前 `funcMap` 存了但未使用，运行期一直读包级 `SetI18nBundle`。
- `loader.go` 增加 `loadMu`，串行化 `LoadAll`/`SetActiveTemplate`（它们共享 `tmplRef`）。
- 结果：渲染不再依赖全局 bundle；`SetI18nBundle` 仅作兼容回退保留。

### A3 双语并行（保留为可选，默认关闭）

- `BuildMultiSite` 拆出 `languageSpec`/`makeLanguageSpec`，新增 `ParallelLanguages` 开关（env `HUAN_PARALLEL_LANGS=1`）。
- 并行时各语言写独立 staging 目录，结束用 **rename** 合并：默认语言 staging → 发布根，其它语言 → `<root>/<baseURL前缀>`；`merge staged outputs` 实测 **0.46ms**。
- 每语言 worker 预算 = `GOMAXPROCS / 语言数`。
- **A/B 结论（同二进制交替 5 轮）**：串行中位 **3.62s**，并行中位 **3.39s**，仅 -6%，且并行时单语言 `pages render+write` 由 250–500ms 涨到 1.2–1.3s。原因：页面渲染已用满全部核心，再并行只是抢占。
- 因此**默认串行**；并行保留用于语言数多、单语言页数少的站点。

### 静态拷贝并行 + APFS clonefile

- `internal/output/writer.go`：`CopyStatic` 改为有界 worker 池（首错即停），`copyFile` 优先 `clonefile`（COW），失败回退字节拷贝。
- `copyclone_darwin.go` / `copyclone_other.go` 平台隔离；`golang.org/x/sys` 提升为直接依赖。
- 实测 `zh-cn/static copy` 仍约 170–185ms，**未见明显收益**（瓶颈在 mkdir/stat 而非字节拷贝），保留改动但不算提速项。

### B1 GOGC 调优（收益明确）

- 现象：CPU profile 中 `runtime.madvise`（GC 归还内存）累计 **1.82s**，纯浪费。
- 矩阵实测（同机 3 轮）：基线 3.67/3.76/3.64，`GOGC=200` **3.27/3.36/3.31**，`GOGC=400` 3.00/3.28/3.41，`GOMAXPROCS=8/12` 无收益。
- 代价：峰值 RSS 约 **600MB → 900MB**（GOGC=400 约 1.2GB）。
- 实现：`cmd/huan/main.go` 在 `GOGC` 未显式设置时 `debug.SetGCPercent(200)`；用户环境变量始终优先。
- 效果：中位 **3.79s → 3.21s**（5 轮 3.54/3.13/3.28/3.21/3.05，load1≈19）。

### B2 模板正则缓存（无输出变化）

- `replaceREFunc` / `findREFunc` 原先每次调用都 `regexp.MustCompile`，search index 模板每页调用一次。
- 改为 `cachedRegexp`（`sync.Map` + 上限 512 条），非法模式仍在首次调用时 panic，保持既有失败语义。
- 实测墙钟无可见差异（1.75s vs 1.73s，噪声内），但消除了重复编译的 CPU 浪费。输出 8763 文件 0 差异。

### B3 可选的 CPU profile 入口

- `cmd/huan/profile.go`：`HUAN_CPUPROFILE=/path/x.pprof huan build ...` 采集 CPU profile；未设置时零成本。

### 新 profile 热点（GOGC=200，3.2s 墙钟，CPU 合计 ≈12.7s）

| 项 | CPU 累计 | 说明 |
|---|---:|---|
| syscall（写盘） | 5.51s | `Writer.Write` 5.07s：open 2.11s、Mkdir 1.63s |
| markdown | 1.21s | goldmark Convert 1.06s |
| taxonomy 页 | 1.16s | 436 tag ×（HTML+RSS）×2 语言 |
| 模板渲染 | 1.03s | text/template |
| seo_injector | 1.00s | 其中 `os.ReadFile` 0.95s——重读刚写完的 HTML |

写盘 syscall 已接近产物形态决定的下界；`seo_injector` 的“重读全站 HTML”是唯一明显可优化的架构性浪费。


## 3. 正确性证据

- `go test ./...` 全量通过。
- `go test -race ./internal/build ./internal/output ./internal/template ./cmd/huan` 通过。
- 新增测试：`TestMultiSiteSkipProjectStatic`、`TestFuncMapWithI18nBindsBundle`。
- **串行 vs 并行输出逐字节对比**：8763 文件，仅串行 0、仅并行 0、内容差异 **0**。
- clonefile 输出与源文件 `cmp` 一致（css/js 抽样）。

## 4. 当前耗时构成（低负载单轮 1.79s，串行双语合计）

| 桶 | ms | 占比 |
|---|---:|---:|
| pages render + write | 563 | 31% |
| feeds + specials | 443 | 25% |
| static + finalize（含 seo_injector 201ms） | 319 | 18% |
| markdown + tree | 267 | 15% |
| shared input preparation | ~167 | 9% |
| templates / contexts / setup | ~28 | 2% |

feeds 细分：paginated home 140ms、taxonomy 121ms、ai outputs 97ms、search index 63ms、sitemap 20ms。

## 5. 1s 是否物理可达（决定性证据）

用 `scripts/bench-io-floor.py` 按真实产物规模（8763 文件 / 234.3 MB）只做写入、不做任何渲染：

| 轮次 | 耗时 |
|---|---:|
| 1 | 0.727s |
| 2 | 0.870s |
| 3 | 0.870s |
| **中位** | **0.870s（0.099 ms/文件）** |

即：**仅把 8763 个产物落盘就要 0.87s**。而完整干净构建 3.6s，其中约 0.9s 是落盘，约 2.7s 是 CPU/渲染。

要达到总时长 <1.0s，CPU+渲染必须压到 ≈0.13s——相对当前 2.7s 需 **20 倍** 提升。结论：**在当前产物形态和本机硬件上，1s 不可达**；本机实测最好单轮 1.81s 也正好落在「0.87s 落盘 + 约 0.9s 渲染」的量级。

产物构成（8763）：

| 类别 | 文件数 | 体积 |
|---|---:|---:|
| HTML 页面 | 3668 | 116.5 MB |
| markdown 镜像 | 2630 | 39.8 MB |
| RSS/sitemap xml | 952 | 10.8 MB |
| 静态 js/svg/css/font | ~1484 | 12.5 MB |
| AI json / search | 9 | 48.5 MB |

要逼近 1s，只能改产物形态或语义：

- **减产物**：关闭 markdown 镜像（-2630 文件/-40MB）、按 tag 的 RSS（-952 文件）→ 文件数约 5000，落盘下界约 0.5s。
- **跳过未变写入**：重复构建只写变化文件——但这就不是「干净全量」口径。
- **放宽目标**：以本机可达的 ≈1.8–2.2s 作为验收线。

## 6. 结论与下一步

**结论（最终口径）**：

- 低负载同条件实测：**1.86s → 1.72s（-7.5%）**，产物 10261 → 8763（-15%）。早期「4.84 → 3.2」中大部分是外部负载差异，不作为提速声明。
- 落盘下界 **0.67s**（低负载），当前 1.73s 中约 **1.0s** 是渲染/markdown/feeds/SEO 注入。
- 1s 仍不可达：即使渲染成本减半也只到约 1.2s，需要同时削减产物或跳过未变写入（用户已排除）。
- 确定收益项：A1（en 静态去重，-1498 文件）、B1（GOGC=200）。A2 是并行前提；A3 并行默认关闭；B2 无墙钟收益但消除重复编译。

**下一步（按 profile 证据排序）**：

1. **seo_injector 重读全站（CPU 1.00s，其中 `os.ReadFile` 0.95s）**：目前 `OnOutputWritten` 会把刚写完的 HTML 全部读回、解析、再写回。可选方案：给 build.Hook 增加“内存态 HTML”回调，或让注入器在渲染阶段处理。**属插件契约变更，需确认后再动。**
2. **写盘 syscall（CPU 5.51s）**：已接近产物形态下界（每页一个目录 + HTML/MD/RSS 三类文件），只能靠减产物或跳过未变写入，用户已明确不走这两条。
3. **taxonomy（CPU 1.16s）/ 模板（1.03s）/ markdown（1.21s）**：微观优化空间有限，需先确认输出等价。
4. **低负载复测**：已完成（load1≈3.4–7.2，中位 1.72–1.75s）。后续测量必须同时记录 load1，否则数据不可比。

## 7. 复现

```bash
SRC=../zhurongshuo
BIN=$(go env GOPATH)/bin/huan
OUT=/tmp/huan-bench; mkdir -p "$OUT"
$BIN build -s "$SRC" -d "$OUT/warm" --noCache >/dev/null
for i in 1 2 3 4 5; do
  rm -rf "$OUT/r$i"
  /usr/bin/time -p $BIN build -s "$SRC" -d "$OUT/r$i" --noCache --timings \
    >"$OUT/log$i.txt" 2>"$OUT/time$i.txt"
done
# 并行对照
HUAN_PARALLEL_LANGS=1 $BIN build -s "$SRC" -d "$OUT/p" --noCache
```

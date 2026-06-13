# ADR 0006：v0.2.3 移除 `huan.yaml` 的 `encryptGroups` dead config

- **状态**：Accepted
- **日期**：2026-06-14
- **决策者**：用户（owner）+ Claude
- **替代方案**：见 §3
- **取代**：[ADR 0005](0005-remove-encrypt-and-v02-feature-batch.md) §1.2「`huan.yaml` 的 `params.encryptGroups` 保留为 dead config」
- **被引用**：[ADR 0005](0005-remove-encrypt-and-v02-feature-batch.md)（被本 ADR 取代的子节）

## 1. 背景

[ADR 0005](0005-remove-encrypt-and-v02-feature-batch.md) §1.2 在 v0.2.0 时决策「保留 `huan.yaml` 的 `params.encryptGroups` 为 dead config」，核心理由是「向后兼容」——`params.*` 本来就是开放 map，多余字段无害。当时明确预留口子：「用户可自行决定何时清理」。

v0.2.2 发版后，三个独立因素触发本次反转：

- **(a) 代码诚实性**：dead config 在 yaml 里具有误导性——新人看到 `encryptGroups` 会以为有功能，必须读 ADR 0005 §1.2 才知道是死的
- **(b) 移除伪兼容约束**：huan 是 0.2.x 的新项目，没有 GA，更没有「已部署用户依赖 `encryptGroups` yaml key」的真实兼容性包袱。ADR 0005 §1.2 当时选「保留」是惯性，不是基于真实约束
- **(c) zhurongshuo Cloudflare 迁移准备**：迁移过程需要清理配置，`encryptGroups` 是其中一项

实证（2026-06-14）：

- `grep -rn 'EncryptGroups\|encryptGroups' --include='*.go' .` 返回 0 行——huan 代码零消费
- `grep -rln 'encrypt\|EncryptGroup' internal/ cmd/ --include='*_test.go'` 返回 0 文件——零测试涉及
- zhurongshuo 仓库的 `huan.yaml` **已经先行清理**（领先于 huan 仓库版本，已含 cloudflare 插件配置）

## 2. 决策

彻底移除 `huan.yaml` 的 `params.encryptGroups` 配置块（11 行）+ 同步全套文档反映这个事实。

具体动作：

- **代码**：`huan.yaml` 删 §26-36 `encryptGroups` 11 行
- **ADR**：本 ADR 0006 新建；ADR 0005 §1.2 加 `**Superseded by [ADR 0006]**` 标记
- **README**：
  - `README.md` §50 bullet 3 重写（移除「Bake encryption into the core」广告，改为 AI-friendly 表述）+ §51 bullet 4 数字 905/2028 (44.5%) → 2026/2032 (99.7%) 顺手校正
  - `README.md` §96-99 `### Encryption & redaction (removed in v0.2.0)` subsection 整段删除（Features section 不留墓碑）
  - `README.md` §243 目录树删 `encrypt/` 行 + 修 `shortcode/` 注释
  - `README.zh-CN.md` §90 对应 subsection 整段删除
- **technical-plan.md**：§96 yaml 示例删 `encryptGroups:` 行 + §154 参数对照表删 `[params.encryptGroups]` 行；§4.4/§4.5 历史段保留（已有 warning 标注）
- **INDEX.md**：ADR 表加 0006 行 + 版本史表加 v0.2.3 行
- **MEMORY.md** §36 改为「v0.2.3 已删」表述
- **CURRENT_STATE.md** v0.2.x 系列表加 v0.2.3 行
- **VERSION**：`0.2.2` → `0.2.3`

**版本号策略**：v0.2.3 + tag + CI 自动 release（与 ADR 0005 v0.2.2 同一发布流水线）。虽然无运行时行为变化（huan 早就不消费该字段），但 yaml/ADR/README 的同步是用户可见事实，需要可定位的 release notes。

## 3. 否决备选

- **保留更久（status quo）**——违反 (a) 代码诚实性触发原因；dead config 持续累积认知负担
- **huan.yaml 启动时报错 "encryptGroups no longer supported"**——用户（zhurongshuo）的 yaml 已经干净，但其他 fork huan 的用户可能仍有该字段；启动报错过于激进，违反 huan 作为通用 SSG 的开放配置原则
- **写自动迁移工具（扫描 yaml 并提示用户清理）**——YAGNI，本次清理是单次动作，不值得为单次动作建工具；未来若有类似批量清理需求再考虑

## 4. 影响

- **zhurongshuo 仓库**：完全不动。zhurongshuo 的 `huan.yaml` 已先行清理，源代码（layouts / content / static）也已全部干净；`docs/`（publishDir）下的孤儿构建产物（`docs/js/random-redact.js` + `docs/js/content-decrypt.js` + 5 个引用它们的旧 HTML）是独立的 zhurongshuo 维护议题，与 huan 加密遗产无关
- **未来加密功能重新启用**：若未来真有付费/会员/加密需求，应作为新 capability（如 `internal/payment/` 或 `internal/access/`）从零设计，不应复活本 ADR 移除的 dead config。yaml 入口届时重新定义，不参考 `encryptGroups` 旧 schema
- **CLAUDE.md**：不动。CLAUDE.md 不提 encrypt，无需同步
- **向后兼容窗口**：本次发版**关闭**「保留为 dead config」的兼容窗口。从 v0.2.3 起，`huan.yaml` 不应再出现 `encryptGroups` 字段；旧 yaml 仍能 build（多余字段被 `params.*` 开放 map 忽略），但文档不再承认其存在

## 5. 验证

- `go test ./...`：PASS（无 Go 代码改动）
- `huan build -s /Users/rong.zhu/Code/zhurongshuo`：3032 输出文件，与 stage 3 基线一致（yaml 改动对实际构建无影响——huan/huan.yaml 仅是 huan 仓库的示例配置，不被 `-s $PROJECT_DIR` 模式的实际构建消费）
- `huan version`：输出 `0.2.3`
- ADR 渲染：本 ADR + ADR 0005 §1.2 supersede 标记在 GitHub markdown 渲染正确

> **注**：`./scripts/diff-build.sh` 目前**无法运行**——zhurongshuo 在 `800b67a59 chore: drop encryption infrastructure and Hugo legacy`（2026-06-13 21:52，本 ADR 之前）一并行删除了 `config.toml`，Hugo 找不到 baseline 配置。这是**预先存在的状态**，与 v0.2.3 改动无关。修复 diff-build.sh（或正式退役该工具）应作为独立工作。

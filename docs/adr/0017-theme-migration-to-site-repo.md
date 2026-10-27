# ADR 0017: zhurongshuo 主题迁回站点仓库

- **状态**: 部分被 [ADR 0018](0018-site-theme-boundary.md) 取代（2026-10-27）：模板/资源迁站点的决定保留；主题 Go helper 进入核心及提示全局注入的决定被取代。下文保留当时决策记录。
- **日期**: 2026-10-17
- **决策者**: 用户 + Claude
- **引用**: [ADR 0012](0012-theme-plugin-system.md)（主题插件系统——本决策部分反转其"模板嵌入 .so"形态）

## 背景

ADR 0012 将 zhurongshuo 主题做成 `plugins/zhurongshuo` .so 插件（模板 + assets 用
`go:embed` 编入，Go 模板函数走 `FuncMap()` 能力接口）。运行三个月后暴露迭代税：

1. 任何主题改动（哪怕一行 CSS）都要走「改 huan → 发版 → 站点 bump 容器 pin」全链路
2. 站点仓库被迫用整文件 override（`layouts/` 遮蔽）绕开发版周期，`products/list.html`
   等文件出现双向分叉，注释里的 "Keep in sync" 成为漂移警告
3. huan 本身就是为 zhurongshuo.com 写的单站点 SSG（`cmd/huan/main.go` 自述），
   间接层没有第二个消费者

## 决策

**主题资产回站点仓库，Go 函数进 huan 内核：**

| 层 | 去向 |
|---|---|
| 41 个模板 | 站点 `layouts/`（优先级链最高，3 个站点 override 保留站点版） |
| 静态资源 3.3M | 站点 `static/theme/zhurongshuo/`（URL 不变：`/theme/zhurongshuo/...`） |
| Go 函数 ×8 | huan `internal/template/funcs_theme.go`，`FuncMap()` 自动合并 `ThemeFuncMap()` |
| `publicationRevisionNotice` 配置 | 站点 `huan.yaml` 从 `plugins.zhurongshuo` 移到 `params:`（文案不变），由 `SetPublicationRevisionNotice()` 在 4 个构建入口注入 |
| `plugins/zhurongshuo/` 目录 | 删除（v0.12.1 起） |

## 不变的部分

- **主题插件机制保留**：`internal/theme`、Admin API `/admin/api/theme`、
  `ThemePlugin` 能力接口不动——通用机制与首个消费者解耦，未来其他主题仍可用
- e2e theme 用例按"无可用 theme 插件"口径断言，无需改动
- 站点部署校验链语义保留但改源：`check_huan_install.py` 的 theme_assets
  指纹比对从 install manifest 改为站点内 `static/theme/` vs `docs/theme/` 自洽

## 验证

- 无插件构建 vs 插件激活构建：`docs/` 全量字节级零差异（3647 文件）
- 函数测试从插件移植进内核，守护测试 `TestNoSilentNoOpFuncs` 注册齐全，
  huan 全仓 1111 测试通过
- revision notice / guide 图表 SVG / 主题资源 URL 抽查一致

## 发布序列（依赖链）

站点部署工具链硬依赖 zhurongshuo.so 校验和，故顺序固定：

1. huan v0.12.0：函数上移（tag 仍带 .so，过渡版本）
2. huan v0.12.1：删 `plugins/zhurongshuo/`
3. 站点 pin v0.12.1 + 校验脚本改源（与 2 同批落地）

## 影响

- 主题迭代只需站点仓库 commit，LiveReload 直接反馈，DX 改善
- huan 发版回归纯引擎节奏，不再被站点样式需求驱动
- 双重来源风险消除：模板唯一权威在站点 `layouts/`

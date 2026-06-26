# Admin Content 页面全面重设计报告

**日期**: 2026-06-26 15:30
**修改人**: Reasonix

## 需求

`/admin/content` 页面全面重新设计，要求：
1. 文章的中文版和英文版合并显示
2. 支持分别编辑
3. 显示不同的文件路径

## 分析

当前实现已具备中英文合并的基础能力：

- 后端 `listAll()` 通过 `content.LoadDir` 返回所有语言 variant
- 前端 `mergeContentItems()` + `extractGroupKey()` 按 base path 分组

但 UI 存在以下问题：
1. **空的"语言"列** — 占用列宽但实际语言信息在 title cell 内通过小字显示
2. **聚合状态** — 所有 variant 合用一个状态标签（已发布/部分草稿/草稿），无法看到每个语言的发布状态
3. **编辑按钮隐藏** — 需要 hover row 才显示，不够直观
4. **语言标识不醒目** — 语言只是灰色小字 `{language}`，没有视觉层次

## 改动

**文件**: `web/admin/src/pages/ContentList.tsx`（仅前端渲染层）

### 布局变更

| 旧 (7列) | 新 (5列) |
|---|---|
| ☐ \| 标题 \| 类型 \| 语言 \| 状态 \| 日期 \| 编辑 | ☐ \| 标题/文件路径 \| 类型 \| 日期 \| 操作 |

### 每行展示

每个 merged article 的 variant 都独立展示：

```
[☐] [EN] English Title                    ●      [page] [2025-10-29] [编辑 EN →]
     📄 posts/foo.en.md                   
     ─────────────────────────────────     
     [默认] 中文标题                        ○                        [编辑 →]
     📄 posts/foo.md
```

- **语言徽章**: `[EN]` 样式为 monochrome badge（bg-muted, 10px font-mono uppercase）
- **状态圆点**: 实心=已发布，空心=草稿
- **文件路径**: 每个 variant 下方显示实际文件路径（含语言后缀）
- **编辑按钮**: 始终可见（移除了 hover 隐藏）
- **分隔线**: 多个 variant 之间以半透明边框分隔

### 清理的代码

- 删除 `totalArticles`（未使用）
- 删除 `totalVariants`（未使用）
- 删除 `allPublished`、`anyDraft` 聚合状态变量（不再需要聚合状态）

## 验证

- TypeScript 编译零错误
- 服务端 API 正常返回 1698 条内容数据（9 sections）
- Vite HMR 已自动热加载
- HTTP 200 OK

## 后续

- 考虑在后端 `ContentListResponse` 中直接返回 merged 结构，减少前端处理
- `buildTree` 的 count 目前包括所有 variant，可能需要去重

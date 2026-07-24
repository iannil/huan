### Task 3: Admin UI 插件列表页面增强

**Files:**
- Modify: `web/admin/src/pages/Plugins.tsx`

- [ ] **Step 1: 更新 PluginInfo 接口，添加新字段**

```tsx
interface PluginInfo {
  name: string
  version: string
  source: string
  capability: string
  status: string
  loadedAt: string
  error: string
  author: string
  repoURL: string
  license: string
  tags: string[]
}
```

- [ ] **Step 2: 修改表格列**

当前表格使用 grid 布局：`grid-cols-[1fr_80px_100px_80px_1fr_120px]`

改为：`grid-cols-[1fr_80px_80px_100px_80px_1fr_120px]`

在"名称"和"来源"之间插入"版本"列。

表格 header 在 `"名称"` 后增加 `"版本"`。

表格 body 在名称 span 后增加版本 span：

```tsx
<span className="text-muted-foreground truncate">{p.version || '-'}</span>
```

- [ ] **Step 3: 添加官方徽章和 Tags**

在名称列中，如果 `p.tags` 非空且 `p.author` 为 "huan team"，在名称后显示 "官方" badge：

```tsx
<span className="text-foreground font-medium truncate">{p.name}</span>
{p.tags && p.tags.length > 0 && (
  <div className="flex items-center gap-1">
    {p.tags.slice(0, 3).map((tag) => (
      <Badge key={tag} variant="secondary" className="text-[9px] leading-none px-1 py-0">
        {tag}
      </Badge>
    ))}
  </div>
)}
```

- [ ] **Step 4: 构建验证**

```bash
cd web/admin && npm run build
```
Expected: Build succeeds

- [ ] **Step 5: 提交**

```bash
git add web/admin/src/pages/Plugins.tsx
git commit -m "feat(admin): enhance plugin list with version, tags, official badge

Co-Authored-By: Claude <noreply@anthropic.com>"
```

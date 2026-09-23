# huan 与 zhurongshuo 脚注支持

日期：2026-09-23

## 修改

- huan 正文已启用 Goldmark Footnote；补齐模板 `markdownify` 的同等支持。
- zhurongshuo 主题新增数字角标、文末注释区、跳转目标高亮、键盘焦点和导航避让样式。使用原生锚点，无需 JavaScript。
- 正文及模板渲染测试检查引用与返回链接，同一脚注多次引用时每个位置均有对应返回链接。

## 写法

```markdown
这里是正文[^source]，这里再次引用[^source]。

[^source]: 这里是注释，支持 **强调** 和 [来源链接](https://example.com)。
```

引用编号按出现顺序生成，注释放在文末。点击数字跳转到注释，点击注释后的返回箭头回到正文引用位置。脚注定义与正文之间保留空行；多段注释的后续段落缩进四个空格。

## 验证与生效范围

- `go test ./internal/template ./internal/markdown`。
- 在 `plugins/zhurongshuo` 下运行 `go test ./...`。
- 本次修改主题源文件；站点使用更新后的 huan 与重新构建的 zhurongshuo 主题插件后生效。未部署线上站点，也未覆盖站点已有生成文件。
- 未进行浏览器视觉验收。

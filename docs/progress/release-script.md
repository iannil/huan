# Release Script

- **日期**：2026-07-30
- **状态**：完成

## 完成内容

创建 `scripts/release.sh` 一键发布脚本，实现了：

1. 版本检测 — 读取 `internal/version/VERSION` 并校验 semver 格式
2. 平台检测 — 自动检测当前 GOOS/GOARCH
3. 编译 huan 二进制 — `CGO_ENABLED=0 -trimpath -ldflags="-s -w"`
4. 编译所有插件 — 逐个 `go build -buildmode=plugin`，失败跳过
5. 生成 checksums.txt — `shasum -a 256 -c` 兼容格式
6. 生成 manifest.json — 含 version、go_version、git_sha、git_dirty、build_time、文件列表
7. 输出摘要

## 设计决策

- Shell 脚本（非 Go 子命令），避免鸡生蛋问题
- 输出路径：`release/v{version}/{os}-{arch}/`，保持版本和平台隔离
- 不删除旧文件（`release/huan` 和 `release/plugins/` 保留不动）
- 插件编译失败跳过，最终汇总失败列表

## 验证

- 实际运行通过，产物完整
- checksums 校验通过（9 个文件全部 OK）
- manifest JSON 格式正确

## 文件结构

```
release/
  v0.7.0/
    darwin-arm64/
      huan              ← huan 二进制
      plugins/
        cloudflare.so
        diagram-renderer.so
        html-injector.so
        image-pipeline.so
        qwen3-translate.so
        seo-injector.so
        sitemap-enhancer.so
        zhurongshuo.so
    huan_0.7.0-checksums.txt
    huan_0.7.0-manifest.json
```
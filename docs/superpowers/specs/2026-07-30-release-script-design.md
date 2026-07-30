# 发布脚本设计文档

- **日期**：2026-07-30
- **状态**：Approved
- **关联项目**：[huan release 命令 (ADR 0004)](../../adr/0004-release-command.md)

## 背景

当前 huan 的发布需要手动执行多个步骤：
1. `go build` 编译 huan 二进制
2. 手动放到 `release/` 根目录
3. `scripts/build-plugins.sh` 编译插件 .so 到 `release/plugins/`
4. 无版本号、无平台区分、无校验和、无 provenance

需要**一键发布脚本**，将 huan 二进制 + 插件 .so 按平台分类输出到 `release/{version}/` 下。

## 设计决策

### 1. 方案选择：Shell 脚本（非 Go 子命令）

| 维度 | 方案 A（Shell 脚本） | 方案 B（扩展 huan release） |
|------|---------------------|---------------------------|
| **复杂程度** | 低 | 高（鸡生蛋问题） |
| **交叉编译支持** | 无需（仅当前平台） | 需要但 Go plugin 不支持 |
| **对现有代码的侵入** | 无 | 需要修改 Go 编译逻辑 |
| **ADR 0004 兼容性** | 兼容（当时否决的是多平台脚本） | 与 release 命令目标冲突 |

**结论**：选 Shell 脚本。场景变了——只打包当前平台，没有交叉编译需求，脚本足够简单可靠。

### 2. 目录结构

```
release/
  v{version}/
    {os}-{arch}/
      huan
      plugins/
        cloudflare.so
        diagram-renderer.so
        html-injector.so
        image-pipeline.so
        qwen3-translate.so
        seo-injector.so
        sitemap-enhancer.so
        zhurongshuo.so
    checksums.txt
    manifest.json
```

- `{version}`：从 `internal/version/VERSION` 读取，例如 `0.7.0`
- `{os}-{arch}`：自动检测，例如 `darwin-arm64`
- `checksums.txt`：`shasum -a 256 -c` 兼容格式，包含 huan 和所有 .so
- `manifest.json`：provenance 信息（version、go_version、git_sha、build_time、文件列表）

### 3. 脚本流程

```
scripts/release.sh
```

1. **版本检测** — 读 `internal/version/VERSION`，校验 semver
2. **平台检测** — `go env GOOS` / `go env GOARCH`
3. **创建输出目录** — `mkdir -p release/v{version}/{os}-{arch}/plugins/`
4. **编译 huan** — `go build -trimpath -ldflags="-s -w" -o ...`
5. **编译插件** — 遍历 `plugins/*/`，逐个 `go build -buildmode=plugin`
6. **生成 checksums** — 计算所有文件 sha256，写入 `checksums.txt`
7. **生成 manifest** — JSON 格式 provenance
8. **输出摘要** — 打印每个文件及其 SHA256

### 4. 错误处理

- **fail-fast**：VERSION 缺失/非 semver → 退出
- **fail-fast**：huan 编译失败 → 退出（没有二进制其他都没意义）
- **部分失败**：单个插件编译失败 → 跳过，继续编译其他，最后汇总
- **原子写入**：先写临时文件，最后 `mv` 到目标路径

### 5. 与现有组件的关系

| 现有组件 | 处理方式 |
|----------|----------|
| `release/huan`（根目录旧二进制） | 保持不动，不清理 |
| `release/plugins/`（根目录旧插件） | 保持不动，不清理 |
| `huan release` 命令 | 保持不动 |
| `scripts/build-plugins.sh` | 保持不动（新脚本复用其 .so 文件名推导逻辑） |

### 6. 用法

```bash
# 一键发布（默认行为）
scripts/release.sh

# 指定输出目录（用于测试）
scripts/release.sh --out-dir /tmp/test-release

# 跳过编译，只重新生成 checksums 和 manifest
scripts/release.sh --skip-build

# 只编译不生成 checksums/manifest
scripts/release.sh --skip-checksums
```

## 输出示例

### checksums.txt
```
a1b2c3d4...  darwin-arm64/huan
e5f6g7h8...  darwin-arm64/plugins/cloudflare.so
...
```

### manifest.json
```json
{
  "version": "0.7.0",
  "go_version": "go1.24.0",
  "git_sha": "abc1234",
  "git_dirty": false,
  "build_time": "2026-07-30T12:00:00+08:00",
  "host_platform": "darwin/arm64",
  "files": [
    {
      "path": "darwin-arm64/huan",
      "sha256": "a1b2c3d4...",
      "size": 37823906
    },
    ...
  ]
}
```

## 不变量

- **不删除旧文件**：只写新路径，不碰 `release/huan` 和 `release/plugins/`
- **不修改 VERSION 文件**：版本管理是开发者的工作
- **不碰 git**：不 tag、不 push、不 commit
- **不依赖预构建的 huan 二进制**：每次从源码编译
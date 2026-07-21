### Task 2: 创建 cloudflare 独立插件仓库

**Files:**
- Create: `../huan-plugin-cloudflare/` — 独立目录
- Create: `../huan-plugin-cloudflare/go.mod`
- Create: `../huan-plugin-cloudflare/plugin.go` — 主插件文件（InitPlugin 导出）
- Create: `../huan-plugin-cloudflare/options.go` — 配置解析
- Copy: 从 `internal/deploy/cloudflare/` 复制所需文件
- Delete: 从 huan 主仓库删除 `internal/deploy/cloudflare/` 目录

**关键设计：** 插件仓库通过 `go.mod` 的 `replace` 指令引用 huan 主仓库的内部包：

```
// huan-plugin-cloudflare/go.mod
module github.com/iannil/huan-plugin-cloudflare

go 1.26.2

require github.com/iannil/huan v0.6.0

replace github.com/iannil/huan => ../huan
```

- [ ] **Step 1: 创建插件仓库目录和 go.mod**

```bash
mkdir -p /Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare
```

`/Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare/go.mod`：

```
module github.com/iannil/huan-plugin-cloudflare

go 1.26.2

require github.com/iannil/huan v0.0.0

replace github.com/iannil/huan => /Users/rong.zhu/Code/zhurong/huan
```

- [ ] **Step 2: 复制 cloudflare 插件代码**

从 `internal/deploy/cloudflare/` 复制所有 `.go` 文件到 `huan-plugin-cloudflare/`。

注意：`client.go`、`concurrency.go`、`git.go`、`hash.go`、`manifest.go`、`options.go`、`pages.go`、`plugin.go`、`r2.go`、`retry.go`、`worker.go` 都需要复制。

- [ ] **Step 3: 创建 InitPlugin 入口**

`/Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare/plugin_main.go`：

```go
package main

import (
    "github.com/iannil/huan/internal/deploy/cloudflare"
    "github.com/iannil/huan/internal/plugin"
)

// InitPlugin 是 .so 插件加载器查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := cloudflare.ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    return cloudflare.New(parsedCfg), nil
}
```

- [ ] **Step 4: 编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan-plugin-cloudflare
go build -buildmode=plugin -o cloudflare.so .
```

预期：BUILD SUCCESS

- [ ] **Step 5: 删除 huan 主仓库的 cloudflare 代码**

```bash
rm -rf /Users/rong.zhu/Code/zhurong/huan/internal/deploy/cloudflare/
```

- [ ] **Step 6: 删除 deploy 包中的 cloudflare 引用**

检查 `internal/deploy/` 中是否还有 cloudflare 引用（如 `types.go` 中的 import），清理。

- [ ] **Step 7: 编译验证 huan 主仓库**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...
```

预期：BUILD SUCCESS（deploy 包本身不依赖 cloudflare 子包）

- [ ] **Step 8: 运行测试**

```bash
go test ./internal/deploy/... -v
```

预期：ALL PASS

- [ ] **Step 9: 提交**

```bash
# 在 huan 主仓库
git add -A
git commit -m "refactor(deploy): extract cloudflare plugin to external repository"
```

---


### Task 3: 创建 qwen3_translate 独立插件仓库

**Files:**
- Create: `../huan-plugin-qwen3/` — 独立目录
- Create: `../huan-plugin-qwen3/go.mod`
- Create: `../huan-plugin-qwen3/plugin_main.go` — InitPlugin 导出
- Copy: 从 `internal/translate/qwen3/` 复制所需文件
- Delete: 从 huan 主仓库删除 `internal/translate/qwen3/` 目录

- [ ] **Step 1: 创建插件仓库目录和 go.mod**

```bash
mkdir -p /Users/rong.zhu/Code/zhurong/huan-plugin-qwen3
```

`/Users/rong.zhu/Code/zhurong/huan-plugin-qwen3/go.mod`：

```
module github.com/iannil/huan-plugin-qwen3

go 1.26.2

require github.com/iannil/huan v0.0.0

replace github.com/iannil/huan => /Users/rong.zhu/Code/zhurong/huan
```

- [ ] **Step 2: 复制 qwen3 插件代码**

从 `internal/translate/qwen3/` 复制所有 `.go` 文件到 `huan-plugin-qwen3/`。

- [ ] **Step 3: 创建 InitPlugin 入口**

`/Users/rong.zhu/Code/zhurong/huan-plugin-qwen3/plugin_main.go`：

```go
package main

import (
    "github.com/iannil/huan/internal/plugin"
    "github.com/iannil/huan/internal/translate/qwen3"
)

// InitPlugin 是 .so 插件加载器查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
    parsedCfg, err := qwen3.ParseConfig(cfg)
    if err != nil {
        return nil, err
    }
    // qwen3.New 需要 projectRoot 参数，从 cfg 中获取
    // 或者通过其他方式传递
    projectRoot := ""
    if v, ok := cfg["_project_root"].(string); ok {
        projectRoot = v
    }
    return qwen3.New(parsedCfg, projectRoot)
}
```

注意：qwen3.New 需要 `projectRoot` 参数，需要让 Loader 支持传递额外配置。

- [ ] **Step 4: 编译验证**

```bash
cd /Users/rong.zhu/Code/zhurong/huan-plugin-qwen3
go build -buildmode=plugin -o qwen3.so .
```

预期：BUILD SUCCESS

- [ ] **Step 5: 删除 huan 主仓库的 qwen3 代码**

```bash
rm -rf /Users/rong.zhu/Code/zhurong/huan/internal/translate/qwen3/
```

- [ ] **Step 6: 编译验证 huan 主仓库**

```bash
cd /Users/rong.zhu/Code/zhurong/huan
go build ./...
```

预期：BUILD SUCCESS

- [ ] **Step 7: 运行测试**

```bash
go test ./internal/translate/... -v
```

预期：ALL PASS（translate 包本身不依赖 qwen3 子包）

- [ ] **Step 8: 提交**

```bash
git add -A
git commit -m "refactor(translate): extract qwen3 plugin to external repository"
```

---


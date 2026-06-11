# CLAUDE.md

此文件为 Claude Code (claude.ai/code) 在此代码库中工作时提供指导。

## 项目概述

huan 是一个用 Go 编写的静态站点生成器，用于替代 Hugo 构建 zhurongshuo.com 网站。

## 关联项目

- **zhurongshuo**（祝融说）—— 当前使用 Hugo 的内容站点，代码路径：`../zhurongshuo`（即 `/Users/rong.zhu/Code/zhurongshuo`）
- 阶段一目标：huan 生成的站点输出必须与 Hugo 的输出 100% 一致

## 常用命令

```bash
# 构建
go build -o huan ./cmd/huan

# 运行
./huan build              # 构建站点
./huan serve              # 开发服务器

# 测试
go test ./...
```

## 架构决策

- 语言：Go
- 模板引擎：阶段一用 `html/template`，阶段二可插件替换
- Markdown 渲染：goldmark（与 Hugo 同源）
- 配置格式：`huan.yaml`
- 项目定位：独立项目，非 Hugo drop-in 替换
- 验证方式：diff 测试管线，与 Hugo 输出逐字节对比
- 插件架构：阶段一预留骨架，阶段二增量扩展

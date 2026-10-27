# 主题与生成器分离验收

日期：2026-10-27。源码实施及验证完成；独立最终审查无新增缺陷。未提交、推送或发版。

## 实现

- 删除核心 `ThemeFuncMap`、`funcs_theme.go`、相关主题测试及四处修订提示构建注入。不存在本站提示全局状态。
- 通用函数位于 `funcs_text.go` / `funcs_data.go`：textWidth、textWrap、textTruncate、parseYAML、errorf。不包含本站文案、guide schema 或图表类型。
- 站点导读提取、图表白名单、字段校验、修订提示全部归站点 layouts/data。旧 readingTime 无站点调用，直接删除。
- 电子书 publication 身份和背景资源显式来自站点，默认中性无本站品牌；三格式共享同一输入。配置与PNG字节进入缓存指纹；根相对EPUB链接按配置URL的origin解析。
- CSV默认 posts-，站点deploy.sh显式传旧前缀；清理仅匹配指定前缀+八位日期的普通文件。
- 原站点字体准备脚本迁回站点；本站封面红栏/圆点在站点背景资源中。
- ADR0018取代ADR0017中主题helper进入核心的决定；两仓当前指导文档已同步。通用主题插件机制、后台、Markdown数学解析保留。

## 验证

- huan `go test ./...` 通过。首次失败因本轮电子书备份`.go`被Go识别为混合包，备份排除后全套通过。
- 电子书独立模块 `go test ./...` 通过；根仓测试不替代该模块测试。
- 站点 `HUAN_BINARY=/tmp/huan-theme-separation.qxdhi7/huan-task1 python3 scripts/test_theme_boundary.py`：5个测试组通过，覆盖guide提取/错误、notice配置与语言/URL过滤/HTML转义。
- 两语言完整构建：中文1388页、英文1335页，无WARN；各1311个source_hash检查通过。
- 基线与候选9517个输出文件：增加0、删除0、字节变化0。564个SVG、1834个提示、6262个JSON-LD块保持一致。
- 浏览器中文/英文信任导读各5个SVG，提示语言正确，1280px无横向页面溢出；formalization109处公式、15个块公式、错误0、数学资源失败0。
- 电子书实现方对中英文封面与迁移前基线逐像素校验一致；生成双语PDF/EPUB/DOCX六份样张并核对出版身份。样张在 `/tmp/huan-ebook-separation-samples/{pdf,epub,docx}/books/individual/`（示范书 / Demo Book）。属于配置真实站点身份的合成双语样本，不是全量书稿出版验收。
- 最终电子书测试含URL子目录回归：92项、4包通过；配置和同路径背景图变更均触发实际重新导出，无变化时跳过。
- `git diff --check` 两仓通过。
- 现有 `python3 -I scripts/check_guide_v2.py --help` 因环境未安装PyYAML失败，未为此安装依赖；新增主题集成测试和完整站点构建已验证本轮变化。

## 产物与限制

- 候选二进制：`/tmp/huan-theme-separation.qxdhi7/huan-task1`。
- 基线：同目录 baseline；候选：candidate；浏览器临时构建：browser。
- 本次没有替换已安装huan、重启用户服务或升级插件；本站新模板及CSV参数需要本次候选/后续包含改动的版本，电子书publication配置需要配套新版插件。不要用旧二进制构建新模板。
- 其他会话的 `internal/version/VERSION` 改动保留，本次未修改版本。

# 断言模式库（patterns）

可复制的断言命令模式。与 [RUNBOOK.md](RUNBOOK.md) 的变量契约（`${bin}`/`${site}`/`${port}`/`${base}`/`${token}`/`${run_id}`/`${artifacts}`）配套使用；每条模式假定环境已按 RUNBOOK §2 准备（含 `HUAN_HOME=$site/.huan-home` 与固定 `${token}`）。已知状态常量引用 [../fixtures/FIXTURES.md](../fixtures/FIXTURES.md)。

## 1. SSE 监听（daemon）

macOS 无 `timeout` 命令，长连接一律用 curl `--max-time` 截断：

```bash
curl -sN --max-time 5 ${base}/api/v1/events | head -8 | tee $artifacts/sse-head.txt
```

事件行格式（`internal/daemon/sse/hub.go`）：

- 有类型事件：两行——`event: <type>` + `data: {...json...}`，随后空行
- 无类型事件：仅 `data: {...}` 行
- **心跳**：`: keepalive` 形式的 SSE 注释行（`:` 开头，每 15s 一次）；注：hub 单元测试用 `:heartbeat` 字样做内部契约，线上注释前缀以实测 `:` 开头为准

断言「订阅即建立」看响应头：

```bash
curl -s -i -N --max-time 3 ${base}/api/v1/events | head -3   # Content-Type: text/event-stream
```

触发事件后断言（后台订阅 + 写文件 + 收流）：

```bash
curl -sN --max-time 8 ${base}/api/v1/events > $artifacts/sse-stream.txt &
SSE_PID=$!
sleep 1
# …此处执行触发动作（如 POST ${base}/admin/api/build，带 Authorization）…
wait $SSE_PID
grep -E '^event: ' $artifacts/sse-stream.txt | sort | uniq -c   # 断言事件类型清单
```

## 2. WS 握手（dev 的 /livereload）

curl 只能做升级握手层（不能完整说 WS 帧）。握手断言 101：

```bash
curl -s -i -N \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  --max-time 3 http://127.0.0.1:$port/livereload | head -5
# 期待首行 HTTP/1.1 101 Switching Protocols + Sec-WebSocket-Accept 头
```

完整帧级断言（hello / reload 广播）用 python3 标准库手搓 WS 客户端（8 行内核）：

```bash
python3 - <<'EOF' | tee $artifacts/lr-hello.txt
import socket, base64, os, struct, json
s = socket.create_connection(("127.0.0.1", int("PORT_PLACEHOLDER")), 3)
key = base64.b64encode(os.urandom(16)).decode()
req = f"GET /livereload HTTP/1.1\r\nHost: 127.0.0.1\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n"
s.sendall(req.encode()); print(s.recv(4096).decode(errors="replace"))  # 握手响应
EOF
```

（`PORT_PLACEHOLDER` 执行时替换为 `${port}`；`/livereload.js` 静态脚本可直接 `curl -s http://127.0.0.1:$port/livereload.js | head -c 60` 断言是 JS。）

浏览器层旅程（连接后 fs.write 改文件 → 观察页面自动刷新）归 browser 套件的 LiveReload 用例（agent-browser 两 tab 并开），此处只做握手层。

## 3. 并发 PUT（最后写赢 + 审计多行）

```bash
for i in 1 2 3 4 5; do
  curl -s -X PUT -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d "{\"frontmatter\":{\"title\":\"并发-$i\"},\"rawContent\":\"并发正文 $i\\n\"}" \
    ${base}/admin/api/content/posts/concurrent-target.md &> $artifacts/put-$i.txt &
done
wait
```

断言终态收敛：

```bash
# 内容为五个版本之一（最后写赢，具体谁赢不作断言）
curl -s -H "Authorization: Bearer $token" ${base}/admin/api/content/posts/concurrent-target.md \
  | python3 -c 'import json,sys; t=json.load(sys.stdin)["frontmatter"]["title"]; assert t in [f"并发-{i}" for i in range(1,6)], t; print("ok:", t)'

# 审计行数 = 5（每次 PUT 一行 content.update）
grep -c "content.update" $site/memory/daily/$(date +%Y-%m-%d).md
```

## 4. mtime 对比（增量构建）

macOS 用 `stat -f %m`（Linux 为 `stat -c %Y`）：

```bash
before_a=$(stat -f %m $site/public/posts/hello-huan/index.html)
before_b=$(stat -f %m $site/public/about/index.html)
# …触发增量构建（改 posts/hello-huan.md 后 build）…
after_a=$(stat -f %m $site/public/posts/hello-huan/index.html)
after_b=$(stat -f %m $site/public/about/index.html)
[ "$after_a" != "$before_a" ] && echo "ok: changed file rebuilt"
[ "$after_b"  = "$before_b" ] && echo "ok: untouched file preserved"
```

注意：全量回退（模板变更）场景两条都会变——正反倒换断言即可。

## 5. 审计断言（AuditRecord 行）

审计写入 `$site/memory/daily/<YYYY-MM-DD>.md`，markdown 小节格式（`internal/admin/audit.go`）：

- 小节头：`### admin audit (HH:MM:SS)`
- `- **action**: \`content.create\``（action 枚举：content.create/update/delete、settings.update、settings.yaml.update、media.upload/delete、plugin.load/unload/reload）
- `- **path**: \`posts/x.md\``
- `- **sha256**: \`<64-hex>\` → \`<64-hex>\``（create 为 `_new_ →`，delete 为 `→ _deleted_`；断言前 8 位即可）

```bash
grep "content.create" $site/memory/daily/$(date +%Y-%m-%d).md | head -2
# 断言对应 path 行与 sha256 前 8 位（从响应或落盘文件现算）：
shasum -a 256 $site/content/posts/crud-001.md | cut -c1-8
grep "$(shasum -a 256 $site/content/posts/crud-001.md | cut -c1-8)" $site/memory/daily/$(date +%Y-%m-%d).md
```

## 6. upload multipart（media 上传）

```bash
curl -s -w '\n%{http_code}' \
  -F "file=@tests/e2e/fixtures/minimal/static/logo.svg" \
  -H "Authorization: Bearer $token" \
  ${base}/admin/api/media
# 期待 201 + {"name":"logo.svg","path":"logo.svg","size":..., "ext":".svg"}
# 注意：path 是相对 static/ 的路径；fs.assert 对 $site/static/logo.svg
```

带 `dir` 子目录上传：

```bash
curl -s -w '\n%{http_code}' \
  -F "file=@tests/e2e/fixtures/minimal/static/logo.svg" -F "dir=assets" \
  -H "Authorization: Bearer $token" \
  ${base}/admin/api/media
# 期待 path: "assets/logo.svg"（fs.assert 对 $site/static/assets/logo.svg）
```

1×1 PNG 现场生成（二进制上传用例前置）：

```bash
printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB\x60\x82' > $site/pixel.png
# 上传后 fs.assert 前八字节 == \x89PNG\r\n\x1a\n：head -c 8 $site/static/pixel.png | xxd | head -1
```

## 7. 插件 .so 现场编译（with-plugins 套件前置）

`.so` 必须与宿主二进制**同源码树**构建（Go plugin 按 buildid + 绝对源码路径校验，跨目录树必报 `different version of package`）：

```bash
# ${bin} 是从本仓库（或本 worktree）go build 出来的，.so 也要在同一目录树下编译：
cd <仓库根或 worktree 根>    # 即构建 ${bin} 时所在的目录
# 注意：seo-injector 与 sitemap-enhancer 是独立 Go module（有自己的 go.mod），
# 必须在各自子目录内编译（顶层 `go build ./plugins/seo-injector` 报
# "main module does not contain package"，实测 2026-08-15）：
( cd plugins/seo-injector && go build -buildmode=plugin -o $site/plugins/seo-injector.so . )
( cd plugins/sitemap-enhancer && go build -buildmode=plugin -o $site/plugins/sitemap-enhancer.so . )
```

要点：
- 输出到 `$site/plugins/`（项目本地插件目录），并保持 `HUAN_HOME=$site/.huan-home` 隔离（RUNBOOK §2.1），否则 `~/.huan` 旧 .so 先被扫到并毒化同名插件。
- 编译顺序无关，但两枚 .so 与 `${bin}` 必须同一次工作区状态产出；`${bin}` 重建后旧 .so 全部作废需重编。
- fixture 目录本身不含 .so（.gitignore 排除），每次 run 现场编译。
- `internal/plugin/testdata/simple_plugin/` 属主 module，在 worktree 根直接编译：`go build -buildmode=plugin -o $site/plugins/simple-plugin.so ./internal/plugin/testdata/simple_plugin`（plugins-admin 套件的 load/reload 用例用）。其 `InitPlugin` 契约是 `func(map[string]any) (interface{}, error)`——返回 `plugin.Plugin` 会因跨模块接口身份校验失败报 "InitPlugin has wrong signature"（实测 2026-08-15 修复在案，勿回退）。

## 8. 报告路径常量

- 报告目录：`docs/reports/e2e/`（首次 `mkdir -p`）；文件名 `<YYYY-MM-DD>-<module>.md`
- 截图/证据：先落 `$artifacts`，报告引用；需要长期保留的拷 `docs/reports/e2e/assets/<run_id>/`
- 格式与判定规则见 RUNBOOK §5/§7

## 9. 已知偏差断言口径（引用速查）

写断言前先对表（详见 FIXTURES.md 各「已知状态偏差」节）：

- draft：单页不落盘、公开 JSON API 排除，但 sitemap/RSS/列表聚合渲染**含** draft——聚合断言要么绕开要么显式断言偏差
- build stdout 的 terms/searchindex WARN 是常态，不得断言零 WARN
- `og:type` 恒 `website`（seo-injector guessKind 形态限制）；sitemap-enhancer 恒 no-op——插件生效证据用注入 meta / daemon plugins list，不用 sitemap diff
- daemon serve 页面无插件注入（daemon 构建不传 PluginRegistry）——注入断言仅对 CLI `huan build` 产物
- `/api/v1/posts` 不存在（404），公开 API 一律 `/api/v1/pages`；en 条目 url 无 `/en/` 前缀且无 language 字段

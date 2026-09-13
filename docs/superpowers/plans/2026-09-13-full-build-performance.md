# Full Build Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 降低 zhurongshuo 双语全量构建及 dev 全量重建时间，并提供阶段耗时证据。

**Architecture:** 每次多语言调用加载一个原始输入快照，按语言复制后进入现有流水线。Writer 使用统计锁和固定路径分片锁，让不同文件并行处理。独立计时收集器覆盖 CLI 和流水线，不共享全局可变状态。

**Tech Stack:** Go 1.26.2、Cobra、Goldmark、tdewolff/minify v2.24.13、YAML v3。

**Spec:** `docs/superpowers/specs/2026-09-13-full-build-performance-design.md`

## Global Constraints

- 本批次仅测量这两项，不修改它们。（模板执行复用与 Markdown 并行。）
- 不存在跨 build 或跨 dev 重建复用旧输入。
- 单语言 BuildSite 保持原有加载路径。
- 保留默认语言先构建、每语言输出清理、staticExclude/staticRoot、hreflang、draft/future/expired/build 指令和各语言 hook 调用次序。
- 同路径锁采用固定数量的路径哈希分片，避免随文件数增长永久保留锁表。
- Setter 仍要求在开始写入前调用。
- dev 每次重建打印独立报告，重建总耗时包含目录交换。
- 不改变插件接口和 Result.Duration 的既有含义。
- 输出写到新的临时目录；站点源文件不用于持久修改。
- 不新增依赖、不改变版本号、不安装或替换用户的 huan；基准与候选主程序及 .so 必须使用相同 Go 工具链和构建参数。

## 文件与边界

新增 `internal/build/timings.go`：计时收集和格式化；`internal/build/inputs.go`：快照和副本；各自配套测试。修改 `build.go`、`pipeline.go`、`pipeline_setup.go`、`pipeline_write.go`、`multisite.go`：插入这些边界，保留现有职责。修改 `cmd/huan/main.go`、`dev.go` 并新增 `timings.go`：命令选项和总耗时输出。修改 `internal/output/writer.go` 并新增 `writer_concurrency_test.go`：锁粒度和真实并发测试。文档更新到 README.md 和 README.zh-CN.md。验收报告保存在 `docs/reports/completed/2026-09-13-full-build-performance.md`。

## Task 1: 可选计时覆盖完整构建

**Files:** Create `internal/build/timings.go`, `internal/build/timings_test.go`, `cmd/huan/timings.go`, `cmd/huan/timings_test.go`; modify `internal/build/build.go`, `pipeline.go`, `pipeline_setup.go`, `pipeline_write.go`, `multisite.go`, `cmd/huan/main.go`, `dev.go`, `README.md`, `README.zh-CN.md`.

**Interfaces:**
- Produce `type TimingEntry struct { Scope, Stage string; Duration time.Duration; Failed bool }`.
- Produce `type Timings struct` with synchronized entries; `NewTimings() *Timings`, `(*Timings).Measure(scope, stage string, fn func() error) error`, `(*Timings).Record(scope, stage string, duration time.Duration, failed bool)`, `(*Timings).Entries() []TimingEntry`, `(*Timings).Report(logf func(string, ...any))`.
- Add `Options.Timings *Timings`; pipeline keeps an internal `timingScope string`, initialized to language code or `site`.
- CLI create one collector per run/rebuild only when `--timings` is true; callers always use an outer defer to report on success or failure.

- [ ] Write failure and nil-collector tests:

```go
func TestTimingsRecordsFailure(t *testing.T) {
    c := NewTimings()
    want := errors.New("stage failed")
    if err := c.Measure("en", "load content", func() error { return want }); err != want {
        t.Fatalf("error = %v", err)
    }
    got := c.Entries()
    if len(got) != 1 || !got[0].Failed || got[0].Scope != "en" {
        t.Fatalf("entries = %+v", got)
    }
}
func TestNilTimingsRunsStage(t *testing.T) {
    var c *Timings
    called := false
    if err := c.Measure("site", "load", func() error { called = true; return nil }); err != nil || !called {
        t.Fatal("nil collector must preserve execution")
    }
}
```

- [ ] Run `go test ./internal/build -run 'TestTimings|TestNilTimings' -count=1`; expect undefined API failure.
- [ ] Implement the collector core; Entries returns a copy under lock. Report uses `[timings] scope/stage duration` and `failed` suffix, does not print a sum of nested measurements.

```go
func (c *Timings) Measure(scope, stage string, fn func() error) error {
    if c == nil { return fn() }
    start := time.Now()
    err := fn()
    c.Record(scope, stage, time.Since(start), err != nil)
    return err
}
func (c *Timings) Record(scope, stage string, d time.Duration, failed bool) {
    if c == nil { return }
    c.mu.Lock()
    defer c.mu.Unlock()
    c.entries = append(c.entries, TimingEntry{scope, stage, d, failed})
}
```

- [ ] Add `Bool("timings", false, "report build stage durations")` to build/dev. Pass Options.Timings through multi-language options. Measure config/registry/theme preparation and image processing in CLI. Record build total in deferred reporting and dev rebuild total after SwapBuildDir; collector must be reset each rebuild and must also report initial startup build.
- [ ] Wrap existing stages and split timing around Markdown/tree/taxonomy, templates/cleanup, static copy/finalization without changing their execution order. For void stages use `Measure(..., func() error { p.renderPages(); return nil })`; explicitly mark failure from per-page error count in the recorded page-stage entry. Record each hook with a qualified child stage such as `static + finalize/hook seo_injector` and preserve the existing warning/fail-fast handling. Theme hooks and AfterBuild callbacks receive their own entries. Keep all measurements inside the existing Result.Duration boundary where applicable.
- [ ] Add a CLI test that `runBuild` with invalid content returns an error and prints timings only when enabled, plus a collector test that concurrent Record calls produce all entries. Add a minimal successful single-language fixture using existing writeFile helpers and assert all major stage names are present without asserting duration thresholds.
- [ ] Run `go test ./internal/build ./cmd/huan`; expected PASS. Document flag examples and wall-time/nested-time interpretation in both READMEs.
- [ ] Commit only this task's files with `feat(build): add optional stage timing reports`.
- [ ] Build a diagnostic binary using `go build -o /private/tmp/huan-perf-diagnostic ./cmd/huan`; rebuild plugins with `scripts/build-plugins.sh /private/tmp/huan-perf-diagnostic-plugins`. Run it against zhurongshuo with `--plugins /private/tmp/huan-perf-diagnostic-plugins --timings --destination /private/tmp/huan-perf-diagnostic-output`. Save the report; it establishes stage attribution before performance changes.

## Task 2: 一次加载的多语言输入快照

**Files:** Create `internal/build/inputs.go`, `inputs_test.go`; modify `build.go`, `pipeline.go`, `multisite.go`, `multisite_test.go`.

**Interfaces:**
- Produce internal `type buildInputs struct { pages []*content.Page; data map[string]interface{}; available map[string]map[string]bool; stale *I18nStaleReport; staleErr error }`.
- Produce `loadBuildInputs(sourceDir string, cfg *config.Config, timings *Timings) (*buildInputs, error)`, `cloneInputPage(*content.Page) *content.Page`, `cloneInputData(interface{}) interface{}`, `availableTranslationsFromPages([]*content.Page, *config.Config) map[string]map[string]bool`.
- Produce internal `buildSiteWithInputs(opts Options, inputs *buildInputs) (*Result, error)`; public BuildSite delegates with nil inputs. `newPipeline` remains compatible; internal pipeline gains inputs field assigned by this entry point.
- Consume Task 1 Options.Timings and Timings.Measure.

- [ ] Add independent clone behavior tests, including nested YAML string-key maps, interface-key maps and slices, Tags/Keywords, Cascade values, and empty tree links:

```go
func TestInputPageIsolation(t *testing.T) {
    src := &content.Page{Tags: []string{"one"}, Keywords: []string{"key"}}
    src.Cascade.Build.Render = "always"
    a, b := cloneInputPage(src), cloneInputPage(src)
    a.Tags[0], a.Keywords[0] = "changed", "changed"
    a.Cascade.Build.Render = "never"
    if b.Tags[0] != "one" || b.Keywords[0] != "key" || b.Cascade.Build.Render != "always" {
        t.Fatal("language inputs share mutable state")
    }
}
func TestInputDataIsolation(t *testing.T) {
    src := map[string]interface{}{"books": []interface{}{map[string]interface{}{"title": "A"}}}
    a := cloneInputData(src).(map[string]interface{})
    a["books"].([]interface{})[0].(map[string]interface{})["title"] = "B"
    if src["books"].([]interface{})[0].(map[string]interface{})["title"] != "A" {
        t.Fatal("data snapshot was mutated")
    }
}
```

- [ ] Run `go test ./internal/build -run 'TestInput' -count=1`; expect undefined clone API failure.
- [ ] Implement copies. Cascade/Sitemap currently contain only value fields; copy by value. Reset all tree links, copy Tags and Keywords with `append([]string(nil), src.Tags...)`. Recursively clone all mutable forms produced by LoadDataFiles:

```go
func cloneInputData(v interface{}) interface{} {
    switch x := v.(type) {
    case map[string]interface{}:
        out := make(map[string]interface{}, len(x))
        for k, value := range x { out[k] = cloneInputData(value) }
        return out
    case map[interface{}]interface{}:
        out := make(map[interface{}]interface{}, len(x))
        for k, value := range x { out[k] = cloneInputData(value) }
        return out
    case []interface{}:
        out := make([]interface{}, len(x))
        for i, value := range x { out[i] = cloneInputData(value) }
        return out
    default:
        return v
    }
}
```

- [ ] Implement loadBuildInputs: run stale check once, store report/error with existing warning semantics; call content.LoadDir and LoadDataFiles once, wrapping their errors as content/data errors; derive available from loaded pages using the same default language normalization as current buildAvailableTranslations. Retain the wrapper buildAvailableTranslations if tests use it, but BuildMultiSite must use the snapshot instead.
- [ ] Route each language through buildSiteWithInputs. In loadContent, check cached stale report with the same strict-mode and warning behavior, copy raw pages before applying PageFilter and calling hooks, copy data before assigning it. Nil-input path preserves existing loading. Deduplicate the report formatting helper so both paths preserve error text. For PipelineCache, Store the current language page copies using their file metadata; do not let copies from another language enter this pipeline's content cache. Keep cache-related caller behavior unchanged and run existing incremental/JIT tests.
- [ ] Extend multi-language integration fixtures with a neutral gallery page and zh/en sidecars. Use AfterBuildSite to capture the two Sites; mutate first-language Tags/Cascade/data and assert second-language values remain original. Verify home/section creation, hreflang, translated params and default-first cleanup using existing tests. Add strict stale-hash failure fixture and malformed Markdown/frontmatter fixture; missing data directory still yields an empty map.
- [ ] Run `go test ./internal/build ./internal/content` and `go test -race ./internal/build`; expected PASS. Run diagnostic timings once to verify one shared content/data/check stage and two language-copy stages.
- [ ] Commit with `perf(build): load multilingual inputs once per full build`.

## Task 3: Writer 不同路径并行输出

**Files:** Modify `internal/output/writer.go`; create `internal/output/writer_concurrency_test.go`; extend `internal/output/minify_test.go`.

**Interfaces:**
- Keep all Writer public signatures unchanged.
- Add internal `(*Writer).pathMutex(relPath string) *sync.Mutex` and fixed `[256]sync.Mutex` array; existing mu becomes statistics-only.
- Add `(*Writer).recordWrite(size int64)`; all successful writes call it exactly once. Stats continues to lock mu.

- [ ] Write contention and integrity tests. Hold `w.mu`, start Write to a different file, wait for the file to exist, then release mu; bounded polling deadline is 2 seconds and release must happen in a defer on failure. Before change the file cannot appear, proving statistics locking covers I/O. Separately use 64 goroutines writing distinct HTML files with minify and canonify enabled; compare each file against a sequential Writer and verify Stats. Repeated concurrent same-path writes must leave exactly one complete payload, not mixed bytes. Failed writes must not increment Stats.
- [ ] Run `go test ./internal/output -run 'TestWriter.*Concurrent|TestWriter.*StatsLock|TestWriter.*Failed' -count=1`; expect contention test failure against original Writer.
- [ ] Inspect the locked dependency source at `$GOMODCACHE/github.com/tdewolff/minify/v2@v2.24.13/minify.go` and registered html/css/js/json/svg/xml Minifier methods. Record the concurrency rationale in the Writer code comment; Minify documentation explicitly declares concurrent safety and registration occurs only in construction. If a registered handler has mutable shared working state, protect minification with a separate mutex instead of assuming safety.
- [ ] Implement path hashing and statistics helper:

```go
func (w *Writer) pathMutex(relPath string) *sync.Mutex {
    path := filepath.Clean(PathToFilePath(relPath, w.publishDir))
    var hash uint32 = 2166136261
    for i := 0; i < len(path); i++ { hash = (hash ^ uint32(path[i])) * 16777619 }
    return &w.pathLocks[hash%uint32(len(w.pathLocks))]
}
func (w *Writer) recordWrite(size int64) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.written++
    w.bytes += size
}
```

- [ ] Replace each Write/WriteBytes/WriteBytesPath/copyFile outer global lock with its path lock. Keep the complete operation under that path lock; lock statistics only after successful I/O. Preserve raw/minified/canonified behavior in each method, static-copy order, and existing copy-file byte accounting. Do not change CleanPublishDir semantics or allow clean/write concurrency.
- [ ] Run `go test ./internal/output ./internal/build` then `go test -race ./internal/output ./internal/build`; expected PASS. Include minify concurrent tests for HTML, CSS, JS, XML, JSON, SVG against sequential expected bytes.
- [ ] Commit with `perf(output): allow parallel writes to different paths`.

## Task 4: 双语产物、dev 和性能验收

**Files:** Create `docs/reports/completed/2026-09-13-full-build-performance.md`; fix only regressions within Tasks 1–3 scope. No permanent test machinery is needed for this one-site benchmark.

**Interfaces:** Consume CLI --timings, --plugins and --destination; use baseline revision `76984df` and candidate HEAD. All benchmark artifacts stay in a fresh mktemp directory.

- [ ] Run `go test ./...` and `go vet ./...`. Inspect the release workflow for additional applicable checks; do not publish a release. If sandbox prevents Go cache/network writes, request normal command escalation.
- [ ] Create a temporary baseline source archive and site copy. Shell execution starts with `perf_root=$(mktemp -d /private/tmp/huan-perf.XXXXXX)`; retain the concrete directory in the report. Use `git archive 76984df` into the temporary baseline source, and copy the site's content/data/i18n/static/huan.yaml without .env/.git or production output. Build baseline/candidate with identical Go compiler, CGO_ENABLED=1, no trimpath/ldflags; each uses plugins built from its own matching source via scripts/build-plugins.sh. Host/plugin checksums must match. Do not use installed .so with a newly compiled incompatible host.
- [ ] Alternate baseline and candidate runs three times each against the same temporary site. Use `/usr/bin/time -l` on macOS to capture wall time and maximum RSS; candidates enable --timings, each run gets a separate output directory. Example after assigning concrete paths:

```bash
/usr/bin/time -l "$perf_root/candidate-huan" build --source "$perf_root/site" --plugins "$perf_root/candidate-plugins" --destination "$perf_root/candidate-1" --timings
```

- [ ] Compare sorted relative file lists and SHA256 contents using a temporary Python verifier. For each directory use `Path(root).rglob('*')`, ignore directories, hash `read_bytes()` with hashlib.sha256, and compare dictionaries. On mismatch print relative paths only, inspect differing files and identify exact dynamic fields. Allow no broad HTML normalization; documented date-only normalization must be explicit and justified. Compare all three runs to identify unstable outputs before judging candidate equivalence.
- [ ] Run candidate dev with temporary site, `--bind 127.0.0.1 --port 15313 --timings --plugins ...`; fetch zh/en representative book/gallery pages with curl, assert language-specific labels and LiveReload injection. Edit one temporary Markdown body, wait for completion, fetch the page and verify changed content and unchanged other-language labels. Write malformed frontmatter in the temporary file and confirm failed rebuild preserves old served output; repair the file and confirm next rebuild succeeds. Stop the owned server using its exec session signal; do not stop other user processes. Report initial startup and successful rebuild timings separately.
- [ ] Write report: exact revisions/compiler, commands/artifact paths, six run durations, medians, peak RSS, stage breakdown, output comparison, tests and dev behavior. Compare to initial installed-binary 13.42-second observation only as context; primary comparison is controlled baseline/candidate. If median does not improve, diagnose using timings and fix within approved scope or report the unresolved bottleneck without claiming success.
- [ ] Commit report with `docs: record full build performance validation`. Summarize measured reduction and any remaining template/Markdown bottleneck to user; further optimizations require their own design.

## Plan self-review

Coverage: optional CLI/pipeline/hook/error timings → Task 1; single-call snapshot, neutral-page/data isolation, cache and strict checks → Task 2; path/statistics/minifier boundaries → Task 3; full suite, race, controlled three-run median/RSS, byte comparison and dev failure/swap behavior → Task 4. Cascade is currently a value-only struct, so no recursive Cascade copier is necessary. No task implements template reuse, Markdown parallelism, persistence or language parallelism. Interfaces are defined at their producing task and retained in consumers.

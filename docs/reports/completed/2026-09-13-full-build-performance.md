# Full build performance validation — 2026-09-13

Controlled baseline median **10.93 s → 7.45 s (31.8% less wall time)**. Median peak RSS **353,894,400 → 402,128,896 bytes (+13.6%; 337.50 → 383.50 MiB)**. All six accepted outputs contain the same 6,591 paths. All bytes outside the nine explicitly documented pre-existing collision/order paths match; supplemental single-worker outputs match every byte, including those paths. Dev edit, failed rebuild preservation, and repair passed.

## Revisions and isolation

- Baseline: `76984df`; candidate: `8c4a917403f00aba8ecec6932ff303edbac573f6` (includes empty-slice correction `8c4a917`).
- Go: `go1.26.2 darwin/arm64`; `CGO_ENABLED=1`; normal builds, no trimpath or ldflags. Eight plugins built from each matching source using `scripts/build-plugins.sh`. Host SHA256 baseline `0590842bfd110f50fe87621bc90d60d8db5ae97c70ba83ab7d5096b678445882`, candidate `78a1fc47880660bcddacd1b5b9574a428254567d63ad623ae2bb36dc24a2c840`; full eight-plugin digests retained in `binary-checksums.txt`. Successful plugin loading without warnings demonstrates matching Go package checksums; host/plugin file hashes are recorded separately, not expected to be identical.
- Artifact root: `/private/tmp/huan-perf-txye40p6`. Reused the prepared `baseline-source`, `baseline-huan`, `baseline-plugins`, and fixed `site` from the handoff. Preparation used a Python-created `huan-perf-*` temporary root instead of the plan’s literal shell `mktemp` command; this step was not repeated and no new live-site copy was taken. Existing generated `internal/admin/dist` was copied during preparation into both source trees, unchanged.
- `site` contains only content/data/i18n/static/huan.yaml, excluding .env/.git/production output. Execute from `site` because theme readFile reads cwd-relative content. Every build has `HUAN_HOME` pointing to `empty-home`; installed global plugins otherwise override local plugins. Dev edits use only separate `dev-site`.
- No installed binary/plugins, live site, versions or dependencies changed. No release/install/publish. All heavy operations ran sequentially.

## Commands and measured cohort

```bash
cd /private/tmp/huan-full-build-performance
CGO_ENABLED=1 go build -o /private/tmp/huan-perf-txye40p6/candidate-huan ./cmd/huan
CGO_ENABLED=1 scripts/build-plugins.sh /private/tmp/huan-perf-txye40p6/candidate-plugins
go test ./...
go vet ./...
cd /private/tmp/huan-perf-txye40p6/site
export HUAN_HOME=/private/tmp/huan-perf-txye40p6/empty-home
for n in 1 2 3; do
  /usr/bin/time -l /private/tmp/huan-perf-txye40p6/baseline-huan build --source /private/tmp/huan-perf-txye40p6/site --plugins /private/tmp/huan-perf-txye40p6/baseline-plugins --destination /private/tmp/huan-perf-txye40p6/accepted-baseline-$n > /private/tmp/huan-perf-txye40p6/accepted-baseline-$n.log 2>&1
  /usr/bin/time -l /private/tmp/huan-perf-txye40p6/candidate-huan build --source /private/tmp/huan-perf-txye40p6/site --plugins /private/tmp/huan-perf-txye40p6/candidate-plugins --destination /private/tmp/huan-perf-txye40p6/accepted-candidate-$n --timings > /private/tmp/huan-perf-txye40p6/accepted-candidate-$n.log 2>&1
done
```

`time -l` explicitly escalated to access macOS kern.clockrate; Go compilation/tests/vet explicitly escalated for Go cache access and localhost test listeners. Initial un-escalated candidate compile failed only on cache permissions, then succeeded with escalation. Final tests and vet exited 0; logs: `final-test.log`, `final-vet.log` (empty, clean). Release workflow inspected: admin UI build and cross-platform artifacts are release preparation; UI source unchanged and identical prebuilt embed used here, so npm/release/container/publishing commands were not run. Relevant race checks already passed in Tasks 2–3, including final corrected Task 2 `go test -race ./internal/build` (1.692 s); see sibling task reports. No repeated race check solely for reporting.

Six accepted runs only, in this exact alternating order; each exited 0:

| Order | Version | Real s | User s | Sys s | Maximum RSS bytes |
|---:|---|---:|---:|---:|---:|
| 1 | baseline 1 | 11.38 | 14.41 | 1.60 | 356548608 |
| 2 | candidate 1 | 8.03 | 14.67 | 1.43 | 389103616 |
| 3 | baseline 2 | 10.87 | 14.06 | 1.26 | 349241344 |
| 4 | candidate 2 | 7.26 | 14.74 | 1.30 | 404963328 |
| 5 | baseline 3 | 10.93 | 14.11 | 1.27 | 353894400 |
| 6 | candidate 3 | 7.45 | 15.13 | 1.46 | 402128896 |

Initial installed-binary 13.42 s observation is context only, not the controlled comparator. Excluded: original incompatible-global-plugin `baseline-1`; `baseline-isolated-1` warmup; timing-only diagnostic; the entire initial `measured-*` cohort at candidate `2e00d7e`, because it exposed gallery tags []→null. That regression changed 79 entries in each search file exclusively in tags, was fixed by preserving allocated empty Tags/Keywords, reviewed, rebuilt, and revalidated. All six accepted search files now hash identically. First abandoned dev session46692 was stopped during startup when the correction pause arrived, and is not a successful dev test.

## Candidate stages

Median wall durations over the three accepted candidates. Parent stages include nested children; **do not sum this table**. Parallel page timing is stage wall time, not worker CPU sums. Registry/theme preparation was 0.214/0.013/0.013 s across runs, so the first-run loading cost remains in the accepted wall measurement.

| Stage | Median seconds |
|---|---:|
| cli/config preparation | 0.000568 |
| cli/registry + theme preparation | 0.013433 |
| multi/load config | 0.000813 |
| multi/shared input preparation/stale check | 0.089943 |
| multi/shared input preparation/content | 0.093324 |
| multi/shared input preparation/data | 0.001777 |
| multi/shared input preparation | 0.185540 |
| zh-cn/theme before render | 0.000000 |
| zh-cn/load config | 0.000057 |
| zh-cn/load content/language copy | 0.000512 |
| zh-cn/load content | 0.000550 |
| zh-cn/markdown | 0.825021 |
| zh-cn/tree + taxonomy | 0.002272 |
| zh-cn/render markdown + tree | 0.827371 |
| zh-cn/setup templates + writer/cleanup | 0.000018 |
| zh-cn/templates | 0.002253 |
| zh-cn/setup templates + writer | 0.002429 |
| zh-cn/contexts | 0.002813 |
| zh-cn/pages render + write | 0.738611 |
| zh-cn/feeds + specials | 1.303562 |
| zh-cn/static copy | 0.003874 |
| zh-cn/static + finalize/hook diagram_renderer | 0.000000 |
| zh-cn/static + finalize/hook html_injector | 0.000002 |
| zh-cn/static + finalize/hook seo_injector | 0.608217 |
| zh-cn/static + finalize/hook sitemap_enhancer | 0.005927 |
| zh-cn/static + finalize | 0.617890 |
| zh-cn/theme after render | 0.000000 |
| en/theme before render | 0.000000 |
| en/load config | 0.000040 |
| en/load content/language copy | 0.000441 |
| en/load content | 0.000630 |
| en/markdown | 1.133252 |
| en/tree + taxonomy | 0.002739 |
| en/render markdown + tree | 1.136067 |
| en/setup templates + writer/cleanup | 0.000017 |
| en/templates | 0.002206 |
| en/setup templates + writer | 0.002409 |
| en/contexts | 0.002545 |
| en/pages render + write | 0.777507 |
| en/feeds + specials | 1.095455 |
| en/static copy | 0.003653 |
| en/static + finalize/hook diagram_renderer | 0.000000 |
| en/static + finalize/hook html_injector | 0.000002 |
| en/static + finalize/hook seo_injector | 0.544815 |
| en/static + finalize/hook sitemap_enhancer | 0.006057 |
| en/static + finalize | 0.554226 |
| en/theme after render | 0.000000 |
| cli/image processing | 0.000005 |
| cli/build total | 7.443039 |

Feeds/specials (~2.40 s combined), serial Markdown (~1.96 s combined), pages render/write (~1.52 s combined), and SEO (~1.15 s combined, nested in finalize) remain meaningful costs. Template setup is small; clone/execution work sits inside pages/feeds and was not independently measured. Do not infer a precise clone-only cost from these stage numbers. Further template reuse or Markdown parallelism needs its own design; neither changed here. RSS increase is a measured tradeoff of this workload, not evidence of a persistent cache/leak.

## Output equivalence and baseline nondeterminism

Verifier `verify-final.py` walks sorted `Path.rglob("*")`, ignores directories, hashes each file’s complete `read_bytes()` with SHA256, and compares dictionaries. `accepted-manifests.json` records every path/hash; `accepted-comparison.log` enumerates run differences and all varying hashes. No dates or arbitrary HTML were normalized. All paths are exact. The union of varying paths is exactly:

- `posts/2023/04/1401/index.html`, `posts/2023/04/1401/index.md`
- `en/posts/2023/04/1401/index.html`, `en/posts/2023/04/1401/index.md`
- `posts/2025/02/2202/index.html`, `posts/2025/02/2202/index.md`
- `en/posts/2025/02/2202/index.html`, `en/posts/2025/02/2202/index.md`
- `en/sitemap.xml`

The source pairs 1401/1402 and 2202/2203 each share a slug, in both languages. Existing concurrent complete-page overwrites have no winner guarantee. Mirrors hash exactly to one of those source Markdown files; each source winner has a single complete HTML hash across observations. `collision-winners.json` and `collision-validation.log` enumerate source identity, exact Markdown/HTML hashes, and accepted runs; complete valid winner table below (each listed directory has index.md/index.html):

| Output directory | Winning source | index.md SHA256 | index.html SHA256 |
|---|---|---|---|
| `posts/2023/04/1401` | `content/posts/2023/04/1401.md` | `9dfe48060bdff4f2412822fa6d2d1c889f95d82846fd949417870f80af2029bd` | `0d3cadba71661511984ce56446e2634c1832a91c87b09eb69d6157f83470b60b` |
| `posts/2023/04/1401` | `content/posts/2023/04/1402.md` | `91159df121341dadf971b07cd26490df09f292ac2ab84dad869baf173b1abf93` | `156bec8f75e30e9c4a297903557f24ef167ef8ca16ee78a452a44f1f08e3c575` |
| `en/posts/2023/04/1401` | `content/posts/2023/04/1401.en.md` | `1e5cd69f2fe60c568e07b6822a5d260449228aa5ad1f32370979de29bef7b776` | `d2857e326fdd8a88a59519bdda804cf8cd46b087027dd27b82c6242dea484b31` |
| `en/posts/2023/04/1401` | `content/posts/2023/04/1402.en.md` | `6578cb420c6494b3a373897f61a8941b144622b5362081ef82b6f62b42be1f93` | `2cc539370b63be5bd28d5c112d0af7061af2e10386ee83d4c3642bf363276814` |
| `posts/2025/02/2202` | `content/posts/2025/02/2202.md` | `5821ac609c2edc1009006cfc88b44549b313fdfcf28af36885b19138c4f289d7` | `dfb5cc5cc6e245966c897dc4175d1d7ddd3b6b2f500050028c962e940c59bef2` |
| `posts/2025/02/2202` | `content/posts/2025/02/2203.md` | `39b992d7af85820c38f9d137a3eab97d384bb3ea52888dfc214633ffc8b4fb29` | `03a152a835f206483619d958439d9175f42e1775dbdafe09741ba7a7c460b6f7` |
| `en/posts/2025/02/2202` | `content/posts/2025/02/2202.en.md` | `a7b9fe26f3319d9859a82b3905f421bbf160fc62bdf761c8204ecd016ded53aa` | `5cb7e0c4404bc43d654f50b232e34ff9e044940fd113fcef74e723bb3a05a449` |
| `en/posts/2025/02/2202` | `content/posts/2025/02/2203.en.md` | `2b183e196fe78ae3e565448f002567add76250f4029e0078969eb87a5be2b334` | `ce149ec8e329ea798601caa32ce0a0b45c53d1fe1c4c07663dd00894ddcf80fc` |

Sitemap difference is exactly the final two complete XML URL children, `/en/posts/` and `/en/products/`, exchanging order. All 1,355 full XML children (including all attributes/text/alternate links) have identical multisets in all six outputs; no values are dropped or normalized. Baseline accepted-1/3 SHA256 `45245b1fc2a03aadf7ad5b7d84727b1a2bd2f324b248af4e337f8f1275a2bd84`; baseline accepted-2 and all candidates `e0df8d9398053931a6fc10fa9edf8491c6f3ab906a144288e7b34eff46f2d15c`. Thus this variability is directly observed within baseline. Unchanged source chain: content/tree.go map topLevelDirs (69), range (78), append autogenerated sections (110), preserve pages order into site.Pages (233–234); template/context.go PopulateSitePages iterates site.Pages (581) and appends siteCtx.Pages (586); build/context.go BuildSitemapContext copies that slice (33). No deterministic ordering promise added. This narrowly scoped baseline limitation was accepted by the controller; no blanket exclusion.

Supplemental sequential same-input runs:

```bash
for v in baseline candidate; do
  HUAN_HOME=/private/tmp/huan-perf-txye40p6/empty-home GOMAXPROCS=1 /private/tmp/huan-perf-txye40p6/$v-huan build --source /private/tmp/huan-perf-txye40p6/site --plugins /private/tmp/huan-perf-txye40p6/$v-plugins --destination /private/tmp/huan-perf-txye40p6/canonical-$v > /private/tmp/huan-perf-txye40p6/canonical-$v.log 2>&1
done
```

`canonical-comparison.log`: 6,591 paths each, no path differences, **no byte differences**. `canonical-manifests.json` preserves every SHA256. These two runs are supplemental equivalence checks, excluded from six measured medians. One worker removes collision races; map ordering happened to match in this pair. No source slug/content repair or baseline semantics change was used.

## Dev integration

```bash
cd /private/tmp/huan-perf-txye40p6/dev-site
HUAN_HOME=/private/tmp/huan-perf-txye40p6/empty-home HUAN_ADMIN_TOKEN=temporary-performance-validation-token /private/tmp/huan-perf-txye40p6/candidate-huan dev --source /private/tmp/huan-perf-txye40p6/dev-site --bind 127.0.0.1 --port 15313 --plugins /private/tmp/huan-perf-txye40p6/candidate-plugins --timings > /private/tmp/huan-perf-txye40p6/final-dev.log 2>&1
python3 /private/tmp/huan-perf-txye40p6/dev-verify.py initial
python3 /private/tmp/huan-perf-txye40p6/dev-verify.py edited
python3 /private/tmp/huan-perf-txye40p6/dev-verify.py failed
python3 /private/tmp/huan-perf-txye40p6/dev-verify.py repaired
```

Local listener and curl checks explicitly escalated. A temporary nonsecret admin token prevented autogenerated secret printing. Only owned final exec session65117 was signaled with Ctrl-C, exit0. Four URLs checked using `curl -fsS`: `/books/volume-1/reality-construction/appendix/`, English equivalent prefixed `/en/`, `/gallery/simple-van-gogh-picasso-2/`, and English equivalent. All returned success with LiveReload injection; zh navigation 首页/归档 and en Home/Archive labels, book titles and gallery title asserted at every stage.

- Startup `cli/build total`: **7.720769625 s**.
- Appended `HUAN-PERF-BODY-MARKER` only to temporary zh appendix body. Rebuild **8.685083792 s**, including directory swap **0.461390042 s**. Marker present in served zh book, absent en; English/book/gallery labels remain correct.
- Replaced temporary zh frontmatter with `title: [unterminated`. Failed shared content parsing reported exact YAML error, `cli/rebuild total` **0.107520375 s failed**, no swap. All four served responses byte-identical to edited-stage responses.
- Repaired by restoring edited valid file. Next rebuild **7.872279958 s**, swap **0.447751042 s**. Marker and language labels verified again. No extra rebuild repetitions.

Evidence: `final-dev.log`; `dev-validation.log` with all assertions PASS; `dev-{initial,edited,failed,repaired}-{zhbook,enbook,zhgallery,engallery}.html`; original/edited temporary appendix backups. Fixed benchmark source manifest `fixed-site-manifest.json` checked unchanged after dev.

## Bounded baseline dev speed observation

Small validation extension: the user requested both build and dev improvement, so one baseline dev startup/edit run was added after candidate behavior passed. Independent `baseline-dev-site` copied from the fixed temporary `site`, same probe body marker, cwd, HUAN_HOME and matching baseline plugins; localhost port15314. Command `python3 -u /private/tmp/huan-perf-txye40p6/baseline-dev.py` owns the baseline subprocess/session58824; explicitly escalated for listener, then stopped by owned Ctrl-C, exit0. No malformed/repair repeat and no extra suite repeat.

Baseline process-start-to-TCP-ready was **11.491135833 s**, polled every0.1s; this external startup boundary is different from candidate internal cli/build total7.720769625s, so no startup reduction percentage is claimed. Baseline successful edit rebuild reported **11.673839417 s** including directory swap. Candidate corresponding successful edit rebuild **8.685083792 s** uses the same rebuild-to-swap boundary: **25.6% shorter in this single observation**. This is a bounded dev comparison, not a three-run median or confidence interval. Initial/edited curl response marker, zh labels and LiveReload checked. Evidence `baseline-dev.log`, `baseline-dev-ready.txt`, `baseline-dev-validation.log`, `baseline-dev-{initial,edited}.html`, launcher `baseline-dev.py`; no other processes stopped.

## Self-review and limits

Every plan acceptance step verified or literal execution substitution stated. No implementation code edited by Task 4. Fresh final compilation/full tests/vet and full actual-site comparison detect regressions rather than relying on counts; the empty-slice fix was required and tested before final acceptance. Race evidence belongs to producing tasks and is linked above. Same source snapshot and matching host/plugins used throughout. No heavy-run overlap. Raw RSS and timing tradeoff retained. Existing colliding slugs remain nondeterministic under normal parallel builds; exact all-byte identity is demonstrated by supplemental same-site single-worker builds. Timings aggregate template work and do not prove clone-only attribution. No release/admin frontend rebuild was claimed. Temporary artifacts are retained for review, not installed.

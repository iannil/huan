### Task 2: build/jit.go — resolveSourceFromURL URL 推导

**Files:**
- Create: `internal/build/jit.go` — resolveSourceFromURL 函数
- Create: `internal/build/jit_test.go` — URL 推导测试

**Interfaces:**
- Produces: `resolveSourceFromURL(pageURL string) string` — 纯函数，无 DAG 依赖

**说明：** 当 URL 不在 DAG 中时（新建 draft 等），按 huan URL 规则推导源文件路径。

- [ ] **Step 1: 编写测试**

`internal/build/jit_test.go`：

```go
package build

import "testing"

func TestResolveSourceFromURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"home", "/", "_index.md"},
		{"section", "/posts/", "posts/_index.md"},
		{"simple page", "/posts/hello/", "posts/hello.md"},
		{"nested page", "/posts/2026/new-year/", "posts/2026/new-year.md"},
		{"deep page", "/books/v1/ch1/", "books/v1/ch1.md"},
		{"explicit _index", "/posts/_index/", "posts/_index.md"},
		{"no trailing slash", "/posts/hello", "posts/hello.md"},
		{"leading+trailing slash stripped", "/posts/hello/", "posts/hello.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveSourceFromURL(tc.url)
			if got != tc.want {
				t.Errorf("resolveSourceFromURL(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/build/ -run "TestResolveSourceFromURL" -v
```
Expected: COMPILATION ERROR (no resolveSourceFromURL)

- [ ] **Step 3: 创建 jit.go 并实现 resolveSourceFromURL**

`internal/build/jit.go`：

```go
package build

import "strings"

// resolveSourceFromURL derives the source file path (relative to content/)
// from a page URL. Used by JIT rendering when the URL is not in the DAG
// (e.g., a newly-created draft not yet captured by a full build).
//
// URL conventions (match Hugo/huan content layout):
//   /                          → _index.md            (home)
//   /posts/                    → posts/_index.md      (section)
//   /posts/hello/              → posts/hello.md       (regular page)
//   /posts/2026/new-year/      → posts/2026/new-year.md
//   /posts/_index/             → posts/_index.md      (explicit)
//
// Returns "" only if the input cannot be parsed (effectively never for a
// normalized URL); callers should still verify the file exists on disk.
func resolveSourceFromURL(pageURL string) string {
	u := strings.Trim(pageURL, "/")
	if u == "" {
		return "_index.md" // home
	}
	parts := strings.Split(u, "/")
	last := parts[len(parts)-1]
	if last == "_index" {
		// /posts/_index/ → posts/_index.md
		return strings.Join(parts, "/") + ".md"
	}
	if len(parts) == 1 {
		// /posts/ → posts/_index.md (single segment = section index)
		return parts[0] + "/_index.md"
	}
	// /posts/hello/ → posts/hello.md
	return strings.Join(parts, "/") + ".md"
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/build/ -run "TestResolveSourceFromURL" -v
```
Expected: ALL PASS

- [ ] **Step 5: 提交**

```bash
git add internal/build/jit.go internal/build/jit_test.go
git commit -m "feat(build): add resolveSourceFromURL for JIT URL→source derivation"
```

---


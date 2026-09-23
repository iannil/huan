package output

import (
	"strings"
	"testing"
)

func TestCompactJSONAlreadyCompactDoesNotAllocate(t *testing.T) {
	const input = `{"key":"keep these spaces","escaped":"a\\\" b","array":[1,2]}`
	allocs := testing.AllocsPerRun(20, func() {
		if got := compactJSONPreservingOrder(input); got != input {
			t.Fatalf("JSON changed: %q", got)
		}
	})
	if allocs != 0 {
		t.Fatalf("compact JSON allocated %g times, want 0", allocs)
	}
}

func TestCanonifyUnchangedDoesNotCopyDocument(t *testing.T) {
	cases := []string{
		strings.Repeat(`<p>unchanged text</p>`, 100),
		`<a href="https://example.org/a">absolute</a><img src=//cdn.example.org/a>`,
		`<pre><code>href=/example</code></pre>`,
		`<script type=application/ld+json>{"a":"b c"}</script>`,
	}
	for _, input := range cases {
		allocs := testing.AllocsPerRun(20, func() {
			if got := Canonify(input, CanonifyOptions{BaseURL: "https://example.com/"}); got != input {
				t.Fatalf("changed input: %q", got)
			}
		})
		// Capturing code/script offsets is allowed, but copying document-sized
		// buffers for unchanged content is not. Plain documents need no allocation.
		if !strings.Contains(input, "<pre>") && !strings.Contains(input, "<script") && allocs != 0 {
			t.Errorf("unchanged document allocated %g times, want 0", allocs)
		}
	}
}

func FuzzCanonifyMatchesReference(f *testing.F) {
	seeds := []string{
		`<a href="/a" src=/b href=/ >`,
		`href = "/" src ='/x' HREF=/x data-href=/x`,
		`<pre><code>href=/inside</code>href=/outside</pre>`,
		`href="/href=/inside" src=//cdn/a href=/`,
		"src\v=\"/a\" href\f=\"/b\" src=/\u00a0x",
		`<script type="application/ld+json"> { "n": 1e999 } </script>`,
		`<script type=application/ld+json> {"s": "a \\" b", "n": 1} </script>`,
		`<head><script type='application/ld+json'> {"z": 2,"a": 1} </script>`,
		`<script type=application/ld+json>{ invalid }</script>`,
	}
	for _, seed := range seeds {
		f.Add(seed, "https://example.com/", false)
	}
	f.Add(`<a href=/>href="/a"`, "https://example.com/$1/", true)
	f.Add(`href="/x" src=/a href=/ >`, "href=/", false)
	f.Fuzz(func(t *testing.T, input, base string, home bool) {
		if len(input) > 8192 || len(base) > 1024 {
			t.Skip()
		}
		opts := CanonifyOptions{BaseURL: base, IsHome: home}
		want := legacyCanonify(input, opts)
		if got := Canonify(input, opts); got != want {
			t.Fatalf("input=%q base=%q home=%v\ngot %q\nwant %q", input, base, home, got, want)
		}
	})
}

func BenchmarkCanonifyCompared(b *testing.B) {
	fixtures := map[string]string{
		"unchanged":  strings.Repeat(`<p>unchanged text</p><a href=https://example.org/a>link</a>`, 200),
		"attributes": strings.Repeat(`<a href="/posts/entry">entry</a><img src=/assets/image.png><a href=/ >home</a>`, 200),
		"jsonld":     strings.Repeat(`<p>content</p>`, 200) + `<script type=application/ld+json>{"@context":"https://schema.org","name":"A B"}</script>`,
	}
	for name, input := range fixtures {
		for impl, fn := range map[string]func(string, CanonifyOptions) string{"reference": legacyCanonify, "optimized": Canonify} {
			b.Run(name+"/"+impl, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				for i := 0; i < b.N; i++ {
					_ = fn(input, CanonifyOptions{BaseURL: "https://example.com/"})
				}
			})
		}
	}
}

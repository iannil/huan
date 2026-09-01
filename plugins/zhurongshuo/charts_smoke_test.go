package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// parseChartPartials parses every chart partial (including _shared.html,
// which defines the "charts/node_text" sub-template) into a single
// html/template set — mirroring how huan's loader registers all template
// files in one namespace so {{ template "charts/node_text" }} resolves.
func parseChartPartials(t *testing.T) *template.Template {
	t.Helper()
	tmpl := template.New("root")
	tmpl.Funcs(chartTestFuncMap())
	_, err := tmpl.ParseFS(chartTemplateFS(), "templates/partials/charts/*.html")
	if err != nil {
		t.Fatalf("parse chart partials: %v", err)
	}
	return tmpl
}

// chartTemplateFS exposes the embedded plugin template FS.
func chartTemplateFS() fs.FS { return templateFS }

// chartTestFuncMap reproduces the huan engine funcMap entries the chart
// partials rely on: arithmetic (float64-coercing, 2-arg), dict, merge,
// scratch, and the plugin's svg text helpers (defined in funcs.go, same
// package). Must be kept in sync with internal/template/funcs.go.
func chartTestFuncMap() template.FuncMap {
	fm := template.FuncMap{
		"add": func(a, b interface{}) interface{} { return toF(a) + toF(b) },
		"sub": func(a, b interface{}) interface{} { return toF(a) - toF(b) },
		"mul": func(a, b interface{}) interface{} { return toF(a) * toF(b) },
		"div": func(a, b interface{}) interface{} {
			bf := toF(b)
			if bf == 0 {
				return 0.0
			}
			return toF(a) / bf
		},
		"mod":        func(a, b int) int { return a % b },
		"dict":       chartDict,
		"merge":      chartMerge,
		"index":      func(item interface{}, key interface{}) interface{} { return chartIndex(item, key) },
		"cond":       func(c bool, a, b interface{}) interface{} { if c { return a }; return b },
		// huan's lt/gt/eq compare via string coercion (never a type error);
		// mirror that so templates can't rely on type-strict compare.
		"lt": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b) },
		"gt": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) > fmt.Sprintf("%v", b) },
		"eq": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b) },
		"ge": func(a, b interface{}) bool { return fmt.Sprintf("%v", a) >= fmt.Sprintf("%v", b) },
		"newScratch": func() *chartScratch { return &chartScratch{m: map[string]interface{}{}} },
		"string":     func(v interface{}) string { return fmt.Sprintf("%v", v) },
		"int":        chartInt,
		"len":        chartLen,
	}
	fm["svgTextWidth"] = svgTextWidth
	fm["svgTruncate"] = svgTruncate
	fm["svgWrap"] = svgWrap
	return fm
}

func toF(v interface{}) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		return x
	}
	return 0
}

func chartDict(args ...interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for i := 0; i+1 < len(args); i += 2 {
		m[fmt.Sprintf("%v", args[i])] = args[i+1]
	}
	return m
}

func chartMerge(args ...interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	for _, a := range args {
		if mm, ok := a.(map[string]interface{}); ok {
			for k, v := range mm {
				m[k] = v
			}
		}
	}
	return m
}

func chartIndex(item, key interface{}) interface{} {
	switch c := item.(type) {
	case map[string]interface{}:
		return c[fmt.Sprintf("%v", key)]
	case []interface{}:
		if i, ok := key.(int); ok && i >= 0 && i < len(c) {
			return c[i]
		}
	}
	return nil
}

func chartLen(v interface{}) int {
	switch c := v.(type) {
	case string:
		return len([]rune(c))
	case []interface{}:
		return len(c)
	case map[string]interface{}:
		return len(c)
	}
	return 0
}

func chartInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

type chartScratch struct{ m map[string]interface{} }

func (s *chartScratch) Set(k string, v interface{}) interface{} { s.m[k] = v; return "" }
func (s *chartScratch) Get(k string) interface{}                { return s.m[k] }

// executeChart renders one partial by name with the given context.
func executeChart(t *testing.T, tmpl *template.Template, name string, ctx interface{}) string {
	t.Helper()
	// html/template.ParseFS names templates by file base name.
	tp := tmpl.Lookup(name)
	if tp == nil {
		t.Fatalf("partial not found: %s", name)
	}
	var buf bytes.Buffer
	if err := tp.Execute(&buf, ctx); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return buf.String()
}

// negativeCoordRe finds "-<digit>" inside shape/text/line tags, i.e. a
// negative coordinate in geometry output (labels/hrefs are outside these tags).
var negativeCoordRe = regexp.MustCompile(`<(polygon|rect|circle|path|text|line)\b[^>]*-\d`)

// tagRe strips XML/HTML tags and surrounding whitespace for text-content checks.
var tagRe = regexp.MustCompile(`<[^>]*>`)

func stripChartTags(s string) string {
	return strings.Join(strings.Fields(tagRe.ReplaceAllString(s, " ")), "")
}

// chartTypes is the full set of chart partials under test.
var chartTypes = []string{"funnel", "ladder", "cycle", "layers", "flow", "network", "spectrum"}

func defaultCfg() map[string]interface{} {
	return map[string]interface{}{"Width": 760, "Height": 420, "LabelSize": 16, "SubLabelSize": 13}
}

func node(label, sublabel, href string) map[string]interface{} {
	n := map[string]interface{}{"label": label}
	if sublabel != "" {
		n["sublabel"] = sublabel
	}
	if href != "" {
		n["href"] = href
	}
	return n
}

func toIface(ms []map[string]interface{}) []interface{} {
	out := make([]interface{}, len(ms))
	for i, m := range ms {
		out[i] = m
	}
	return out
}

// checkCommon runs the shared assertions on rendered chart output.
func checkCommon(t *testing.T, typ, out string, labels []string) {
	t.Helper()
	if strings.Contains(out, "NaN") || strings.Contains(out, "+Inf") {
		t.Errorf("%s: output contains NaN/Inf", typ)
	}
	if negativeCoordRe.MatchString(out) {
		t.Errorf("%s: negative coordinate in geometry: %s", typ, negativeCoordRe.FindString(out))
	}
	if !strings.Contains(out, `class="guide-chart guide-chart--`+typ+`"`) {
		t.Errorf("%s: missing svg root class", typ)
	}
	for _, l := range labels {
		// Labels may be wrapped across multiple <text> elements or truncated;
		// compare against tag/whitespace-stripped text, checking the first 4
		// runes of the label are present in order.
		runes := []rune(l)
		prefix := string(runes)
		if len(runes) > 4 {
			prefix = string(runes[:4])
		}
		stripped := stripChartTags(out)
		if !strings.Contains(stripped, prefix) {
			t.Errorf("%s: label prefix %q not found in output text", typ, prefix)
		}
	}
}

func TestChartPartialsSmoke(t *testing.T) {
	tmpl := parseChartPartials(t)

	for _, typ := range chartTypes {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			cases := []struct {
				name   string
				chart  map[string]interface{}
				labels []string
			}{
				{
					"2nodes-href",
					map[string]interface{}{
						"chart_type": typ, "title": "测试图 " + typ,
						"nodes": toIface([]map[string]interface{}{
							node("起点概念", "origin", "/books/v1/ch01/"),
							node("终点概念", "", ""),
						}),
						"edges": toIface([]map[string]interface{}{{"from": "起点概念", "to": "终点概念"}}),
					},
					[]string{"起点概念", "终点概念"},
				},
				{
					"4nodes",
					map[string]interface{}{
						"chart_type": typ, "title": "四节点 " + typ,
						"nodes": toIface([]map[string]interface{}{
							node("现象层", "观察", ""), node("规律层", "归纳", ""),
							node("原理层", "抽象", ""), node("根基层", "公理", ""),
						}),
					},
					[]string{"现象层", "规律层", "原理层", "根基层"},
				},
				{
					"10nodes-longlabel",
					map[string]interface{}{
						"chart_type": typ, "title": "极端 " + typ,
						"nodes": toIface([]map[string]interface{}{
							node("这是一个特别特别特别特别特别特别特别特别长的中文标签用来测试截断换行行为", "附注", ""),
						}),
					},
					[]string{"这是一个特别长的中文标签用来测试截断换行行为"},
				},
			}
			// fix: rebuild 10-node chart properly
			long := "这是一个特别特别特别特别特别特别特别特别长的中文标签用来测试截断换行行为"
			tenNodes := make([]map[string]interface{}, 10)
			tenLabels := make([]string, 10)
			for i := 0; i < 10; i++ {
				l := fmt.Sprintf("节点%d", i)
				if i == 5 {
					l = long
				}
				tenNodes[i] = node(l, "", "")
				tenLabels[i] = l
			}
			cases[2] = struct {
				name   string
				chart  map[string]interface{}
				labels []string
			}{"10nodes-longlabel", map[string]interface{}{
				"chart_type": typ, "title": "极端 " + typ,
				"nodes": toIface(tenNodes),
			}, tenLabels}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					out := executeChart(t, tmpl, typ+".html", map[string]interface{}{"chart": tc.chart, "cfg": defaultCfg()})
					checkCommon(t, typ, out, tc.labels)
				})
			}
		})
	}
}

// TestNetworkEdgesByLabel verifies the edges-by-label lookup path renders
// connecting lines for known labels and silently skips unknown ones.
func TestNetworkEdgesByLabel(t *testing.T) {
	tmpl := parseChartPartials(t)
	chart := map[string]interface{}{
		"chart_type": "network", "title": "网络",
		"nodes": toIface([]map[string]interface{}{
			node("核心", "", ""), node("卫星甲", "", ""), node("卫星乙", "", ""),
		}),
		"edges": toIface([]map[string]interface{}{
			{"from": "核心", "to": "卫星甲"},
			{"from": "核心", "to": "不存在"},
		}),
	}
	out := executeChart(t, tmpl, "network.html", map[string]interface{}{"chart": chart, "cfg": defaultCfg()})
	if n := strings.Count(out, `<line`); n != 1 {
		t.Errorf("expected 1 edge line for 1 valid edge, got %d", n)
	}
	if !strings.Contains(out, "卫星甲") {
		t.Error("network: satellite label missing")
	}
}

// TestNetworkRadialDegradation verifies no-edges network draws one spoke per
// satellite to the center.
func TestNetworkRadialDegradation(t *testing.T) {
	tmpl := parseChartPartials(t)
	chart := map[string]interface{}{
		"chart_type": "network", "title": "辐射",
		"nodes": toIface([]map[string]interface{}{
			node("核心", "", ""), node("甲", "", ""), node("乙", "", ""), node("丙", "", ""),
		}),
	}
	out := executeChart(t, tmpl, "network.html", map[string]interface{}{"chart": chart, "cfg": defaultCfg()})
	if n := strings.Count(out, `<line`); n != 3 {
		t.Errorf("expected 3 radial spokes, got %d", n)
	}
}

// TestSmallCfgRenders verifies concept-card small sizes (float-ish small
// dimensions) still render valid geometry.
func TestSmallCfgRenders(t *testing.T) {
	tmpl := parseChartPartials(t)
	cfg := map[string]interface{}{"Width": 340, "Height": 220, "LabelSize": 12, "SubLabelSize": 10}
	chart := map[string]interface{}{
		"chart_type": "funnel", "title": "小图",
		"nodes": toIface([]map[string]interface{}{node("甲", "", ""), node("乙", "", ""), node("丙", "", "")}),
	}
	out := executeChart(t, tmpl, "funnel.html", map[string]interface{}{"chart": chart, "cfg": cfg})
	checkCommon(t, "funnel", out, []string{"甲", "乙", "丙"})
}

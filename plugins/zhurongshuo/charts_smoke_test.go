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
	stripped := stripChartTags(out)
	for _, l := range labels {
		// Labels may be wrapped across multiple <text> elements or truncated;
		// compare against tag/whitespace-stripped text, checking the first 4
		// runes of the label are present in order.
		runes := []rune(l)
		prefix := string(runes)
		if len(runes) > 4 {
			prefix = string(runes[:4])
		}
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
			}
			// 10 nodes with one very long CJK label to stress wrap/truncate.
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
			cases = append(cases, struct {
				name   string
				chart  map[string]interface{}
				labels []string
			}{"10nodes-longlabel", map[string]interface{}{
				"chart_type": typ, "title": "极端 " + typ,
				"nodes": toIface(tenNodes),
			}, tenLabels})

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

// firstTextYAbove reports whether the first <text> in out sits above yRef
// (its y attribute is numerically smaller than yRef).
func firstTextY(t *testing.T, out, label string) float64 {
	t.Helper()
	re := regexp.MustCompile(`(?s)<text[^>]*>` + regexp.QuoteMeta(label) + `</text>`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("text element for label %q not found", label)
	}
	yRe := regexp.MustCompile(`y="(-?[\d.]+)"`)
	ym := yRe.FindStringSubmatch(m[0])
	if ym == nil {
		t.Fatalf("y attribute not found for label %q", label)
	}
	var v float64
	fmt.Sscanf(ym[1], "%f", &v)
	return v
}

// TestCycleUpperNodeLabelAbove verifies the upper-half text direction fix:
// for the top node of a cycle, the label's text baseline must be above the
// node's circle center (string-compare lt against "-" was always false,
// which pushed every label below its node).
func TestCycleUpperNodeLabelAbove(t *testing.T) {
	tmpl := parseChartPartials(t)
	chart := map[string]interface{}{
		"chart_type": "cycle", "title": "循环",
		"nodes": toIface([]map[string]interface{}{
			node("顶点", "", ""), node("右点", "", ""), node("底点", "", ""), node("左点", "", ""),
		}),
	}
	out := executeChart(t, tmpl, "cycle.html", map[string]interface{}{"chart": chart, "cfg": defaultCfg()})
	// cy = 210; top node y = 70, bottom node y = 350.
	if y := firstTextY(t, out, "顶点"); !(y < 70.0) {
		t.Errorf("cycle: top node label should be above the node (y < 70), got y=%.1f", y)
	}
	if y := firstTextY(t, out, "底点"); !(y > 350.0) {
		t.Errorf("cycle: bottom node label should be below the node (y > 350), got y=%.1f", y)
	}
}

// TestNetworkTopSatelliteLabelAbove verifies network satellites on the upper
// half place their label above the node circle.
func TestNetworkTopSatelliteLabelAbove(t *testing.T) {
	tmpl := parseChartPartials(t)
	// satellite table (3): idx0 → (1,0) east, idx1 → (-0.5, 0.866) lower-left,
	// idx2 → (-0.5,-0.866) upper-left. With 4 nodes, node 3 maps to idx2.
	chart := map[string]interface{}{
		"chart_type": "network", "title": "网络",
		"nodes": toIface([]map[string]interface{}{
			node("核心", "", ""), node("东", "", ""), node("下", "", ""), node("上", "", ""),
		}),
	}
	out := executeChart(t, tmpl, "network.html", map[string]interface{}{"chart": chart, "cfg": defaultCfg()})
	// upper satellite y = 210 - 0.866*140 ≈ 88.8; its label must be above (y < 88.8).
	if y := firstTextY(t, out, "上"); !(y < 88.8) {
		t.Errorf("network: upper-half satellite label should be above the node (y < 88.8), got y=%.1f", y)
	}
	// lower satellite y = 210 + 0.866*140 ≈ 331.2; label below (y > 331.2).
	if y := firstTextY(t, out, "下"); !(y > 331.2) {
		t.Errorf("network: lower-half satellite label should be below the node (y > 331.2), got y=%.1f", y)
	}
}

// TestLadderStepsDoNotOverlap verifies ladder step heights shrink with node
// count so consecutive rects never overlap vertically (rect height used to
// be fixed at 48 while stepY=(H-140)/n < 48 for n ≥ 7).
func TestLadderStepsDoNotOverlap(t *testing.T) {
	tmpl := parseChartPartials(t)
	nodes := make([]map[string]interface{}, 10)
	for i := 0; i < 10; i++ {
		nodes[i] = node(fmt.Sprintf("阶%d", i), "", "")
	}
	chart := map[string]interface{}{"chart_type": "ladder", "title": "阶梯", "nodes": toIface(nodes)}
	out := executeChart(t, tmpl, "ladder.html", map[string]interface{}{"chart": chart, "cfg": defaultCfg()})
	rectRe := regexp.MustCompile(`<rect x="([\d.-]+)" y="([\d.-]+)" width="[\d.]+" height="([\d.]+)"`)
	matches := rectRe.FindAllStringSubmatch(out, -1)
	if len(matches) != 10 {
		t.Fatalf("expected 10 step rects, got %d", len(matches))
	}
	type box struct{ y, h float64 }
	var boxes []box
	for _, m := range matches {
		var y, h float64
		fmt.Sscanf(m[2], "%f", &y)
		fmt.Sscanf(m[3], "%f", &h)
		boxes = append(boxes, box{y, h})
	}
	// Steps ascend left→right (top edge rises by stepY per step) and are
	// disjoint in x; the invariant to protect is the step height: a rect may
	// never be taller than the vertical rise per step, else consecutive
	// steps visually collide (h=48 > stepY for n≥7 was the original bug).
	stepY := (420.0 - 140.0) / 10.0 // 28 for the default cfg
	for i, b := range boxes {
		if b.h > stepY+1e-9 {
			t.Errorf("ladder: step %d height %.1f exceeds vertical rise per step (%.1f)", i, b.h, stepY)
		}
		if b.h < 24.0-1e-9 {
			t.Errorf("ladder: step %d height %.1f below floor 24", i, b.h)
		}
	}
	// and steps must actually ascend
	for i := 1; i < len(boxes); i++ {
		if boxes[i].y > boxes[i-1].y {
			t.Errorf("ladder: step %d top (%.1f) below step %d (%.1f) — steps must ascend", i, boxes[i].y, i-1, boxes[i-1].y)
		}
		if boxes[i].h < 24.0-1e-9 {
			t.Errorf("ladder: step %d height %.1f below floor 24", i, boxes[i].h)
		}
	}
}

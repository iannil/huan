package render

import "testing"

func TestChineseCoverTitleSemanticSplit(t *testing.T) {
	for _, tt := range []struct{ input, main, sub string }{
		{"可能性收割：作为资源的肉体与精神", "可能性收割", "作为资源的肉体与精神"},
		{"解构文明（上）：公共事实与共同解释的瓦解", "解构文明（上）", "公共事实与共同解释的瓦解"},
		{"无副标题", "无副标题", ""},
		{"：无主标题", "：无主标题", ""},
	} {
		main, sub := splitChineseCoverTitle(tt.input)
		if main != tt.main || sub != tt.sub {
			t.Errorf("%q: got (%q, %q)", tt.input, main, sub)
		}
	}
}

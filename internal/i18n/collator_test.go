package i18n

import (
	"testing"

	"golang.org/x/text/collate"
)

func TestBuildCollator_ReturnsCollator(t *testing.T) {
	c := BuildCollator("zh-cn")
	if c == nil {
		t.Fatal("BuildCollator returned nil")
	}
	// Verify it's a *collate.Collator by attempting a comparison.
	got := c.CompareString("一", "二")
	if got == 0 {
		t.Error("expected non-zero comparison for distinct strings")
	}
	_ = collate.New // ensure import is used
}

func TestBuildCollator_ZhCN_PinyinOrder(t *testing.T) {
	// CLDR zh-cn table is pinyin order.
	// Pinyin: 八(bā) 百(bǎi) 二(èr) 九(jiǔ) 六(liù) 七(qī) 千(qiān) 三(sān)
	//         十(shí) 四(sì) 万(wàn) 五(wǔ) 一(yī)
	c := BuildCollator("zh-cn")

	digits := []string{"一", "二", "三", "四", "五", "六", "七", "八", "九", "十", "百", "千", "万"}
	wantPinyin := []string{"八", "百", "二", "九", "六", "七", "千", "三", "十", "四", "万", "五", "一"}

	// Copy digits and sort using collator.
	got := append([]string(nil), digits...)
	c.SortStrings(got)

	for i, w := range wantPinyin {
		if i >= len(got) {
			t.Errorf("sorted list shorter than expected at index %d", i)
			break
		}
		if got[i] != w {
			t.Errorf("ZhCN pinyin order pos %d: got %q, want %q\nfull got: %v\nfull want: %v", i, got[i], w, got, wantPinyin)
			break
		}
	}
}

func TestBuildCollator_FallbackToEnglishForInvalidLang(t *testing.T) {
	// Hugo's behavior: invalid language tag falls back to language.English.
	c := BuildCollator("not-a-real-lang")
	if c == nil {
		t.Fatal("BuildCollator returned nil for invalid lang")
	}
	// English collator should sort ASCII alphabetically.
	if c.CompareString("apple", "banana") >= 0 {
		t.Errorf("English fallback: apple should sort before banana")
	}
}

func TestBuildCollator_EmptyStringDefaultsToEnglish(t *testing.T) {
	c := BuildCollator("")
	if c == nil {
		t.Fatal("BuildCollator returned nil for empty lang")
	}
	if c.CompareString("apple", "banana") >= 0 {
		t.Errorf("Empty lang: apple should sort before banana (English default)")
	}
}

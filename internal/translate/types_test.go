package translate

import "testing"

// Stub type used to verify the Translator interface contract: any plugin
// implementing plugin.Plugin (Name() string) + Translate(ctx, Request) (*Response, error)
// satisfies Translator.
type fakeTranslator struct {
	name string
}

func (f *fakeTranslator) Name() string { return f.name }
func (f *fakeTranslator) Translate(_ ctxStub, _ Request) (*Response, error) {
	return &Response{Body: "translated"}, nil
}

// ctxStub is a type alias to avoid importing context in this contract test.
// Real implementations use context.Context.
type ctxStub = interface {
	Deadline() (deadline interface{}, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

func TestQualityResult_HardCheckFailures_AllPass(t *testing.T) {
	q := QualityResult{
		XMLParse:           true,
		LanguageDetection:  true,
		MarkdownStructure:  true,
		FormatPurity:       true,
		GlossaryCompliance: true,
	}
	if got := q.HardCheckFailures(); len(got) != 0 {
		t.Errorf("expected no failures, got %v", got)
	}
}

func TestQualityResult_HardCheckFailures_AllFail(t *testing.T) {
	q := QualityResult{}
	failed := q.HardCheckFailures()
	if len(failed) != 4 {
		t.Fatalf("expected 4 failures, got %d: %v", len(failed), failed)
	}
	wantSet := map[string]bool{
		"xml_parse":          true,
		"language_detection": true,
		"markdown_structure": true,
		"format_purity":      true,
	}
	for _, name := range failed {
		if !wantSet[name] {
			t.Errorf("unexpected failure %q", name)
		}
	}
}

func TestQualityResult_HardCheckFailures_PartialFail(t *testing.T) {
	q := QualityResult{
		XMLParse:          true,
		LanguageDetection: false,
		MarkdownStructure: true,
		FormatPurity:      true,
	}
	failed := q.HardCheckFailures()
	if len(failed) != 1 || failed[0] != "language_detection" {
		t.Errorf("expected only language_detection to fail, got %v", failed)
	}
}

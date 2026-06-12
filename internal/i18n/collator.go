package i18n

import (
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// BuildCollator returns a collate.Collator for the given language code
// (e.g. "zh-cn", "en", "ja"). Mirrors Hugo's langs/language.go behavior:
//
//   - If langCode parses to a valid language.Tag, use it.
//   - Otherwise (including empty string), fall back to language.English.
//
// The returned Collator is safe for concurrent use (collate.Collator is
// goroutine-safe per golang.org/x/text docs).
func BuildCollator(langCode string) *collate.Collator {
	tag, err := language.Parse(langCode)
	if err != nil || tag == language.Und {
		tag = language.English
	}
	return collate.New(tag)
}

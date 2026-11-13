package config

import (
	"fmt"
	"sort"
)

// removedLanguageKeys lists `languages.<code>.*` keys that an earlier release
// accepted and that are now rejected at load time, mapped to the reason.
//
// Typed decoding ignores unknown keys, so without this list a removed option
// would be dropped silently: the config would look applied while the behavior
// changed. Failing loudly with a migration hint keeps the config honest.
var removedLanguageKeys = map[string]string{
	"skipProjectStatic": "every language output must carry its own copy of static/, because canonify prefixes root-relative asset URLs (/css/... -> /<lang>/css/...) with the language base; skipping the copy published English pages with missing stylesheets",
}

// rejectRemovedKeys returns an error when huan.yaml still sets a removed key.
// Keys are reported deterministically (language code, then key) so repeated
// runs produce identical messages.
func rejectRemovedKeys(raw map[string]any) error {
	languages, ok := raw["languages"].(map[string]any)
	if !ok {
		return nil
	}
	codes := make([]string, 0, len(languages))
	for code := range languages {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		entry, ok := languages[code].(map[string]any)
		if !ok {
			continue
		}
		keys := make([]string, 0, len(entry))
		for key := range entry {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if reason, removed := removedLanguageKeys[key]; removed {
				return fmt.Errorf("huan.yaml: languages.%s.%s was removed: %s; delete that line", code, key, reason)
			}
		}
	}
	return nil
}

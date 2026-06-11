package template

import "sync"

// i18nBundle is the shared bundle used by the i18n / T template functions.
var (
	i18nBundle   I18nBundle
	i18nBundleMu sync.RWMutex
)

// I18nBundle is the minimal interface the template package needs from an i18n bundle.
type I18nBundle interface {
	Translate(key string, args ...interface{}) string
}

// SetI18nBundle installs the bundle used by the i18n template function.
func SetI18nBundle(b I18nBundle) {
	i18nBundleMu.Lock()
	defer i18nBundleMu.Unlock()
	i18nBundle = b
}

// currentI18nBundle returns the currently installed bundle (nil if none).
func currentI18nBundle() I18nBundle {
	i18nBundleMu.RLock()
	defer i18nBundleMu.RUnlock()
	return i18nBundle
}

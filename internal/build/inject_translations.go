package build

// inject_translations.go: site_translations injection for multi-language builds.
// Called from pipeline.go::loadConfig for non-default language builds so the
// English pages see English subTitle etc.

import "github.com/iannil/huan/internal/config"

// injectSiteTranslations overrides cfg.Params with cached translations from
// the qwen3_translate plugin's site_translations config block. Called for
// non-default language builds so English pages see English subTitle etc.
//
// The plugin config block has the shape:
//
//	plugins:
//	  qwen3_translate:
//	    site_translations:
//	      en:
//	        subTitle: "..."
//	        description: "..."
//	        keywords: [...]
//	        footerSlogan: "..."
//
// Silently no-ops when the plugin block is absent or has no entry for lang.
func injectSiteTranslations(cfg *config.Config, lang string) {
	pluginCfg, ok := cfg.Plugins["qwen3_translate"]
	if !ok {
		return
	}
	siteTrans, ok := pluginCfg["site_translations"].(map[string]interface{})
	if !ok {
		return
	}
	langBlock, ok := siteTrans[lang].(map[string]interface{})
	if !ok {
		return
	}
	if v, ok := langBlock["subTitle"].(string); ok && v != "" {
		cfg.Params.SubTitle = v
	}
	if v, ok := langBlock["description"].(string); ok && v != "" {
		cfg.Params.Description = v
	}
	if v, ok := langBlock["footerSlogan"].(string); ok && v != "" {
		cfg.Params.FooterSlogan = v
	}
	if kw, ok := langBlock["keywords"].([]interface{}); ok && len(kw) > 0 {
		out := make([]string, 0, len(kw))
		for _, k := range kw {
			if s, ok := k.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			cfg.Params.Keywords = out
		}
	}
}

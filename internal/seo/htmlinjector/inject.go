package htmlinjector

import "strings"

// InjectHTML injects configured HTML fragments into the page content.
// Returns the modified HTML. If no injection is needed, returns the original.
func InjectHTML(htmlSrc string, cfg *Config, pageKind string) string {
	if cfg == nil {
		return htmlSrc
	}

	// Check kind filters
	if len(cfg.IncludeKinds) > 0 {
		if !contains(cfg.IncludeKinds, pageKind) {
			return htmlSrc
		}
	}
	if len(cfg.ExcludeKinds) > 0 {
		if contains(cfg.ExcludeKinds, pageKind) {
			return htmlSrc
		}
	}

	// Inject Head fragments (before </head>)
	if len(cfg.Head) > 0 {
		headClose := strings.Index(htmlSrc, "</head>")
		if headClose >= 0 {
			injection := "\n" + strings.Join(cfg.Head, "\n") + "\n"
			htmlSrc = htmlSrc[:headClose] + injection + htmlSrc[headClose:]
		}
	}

	// Inject BodyEnd fragments (before </body>)
	if len(cfg.BodyEnd) > 0 {
		bodyClose := strings.Index(htmlSrc, "</body>")
		if bodyClose >= 0 {
			injection := "\n" + strings.Join(cfg.BodyEnd, "\n") + "\n"
			htmlSrc = htmlSrc[:bodyClose] + injection + htmlSrc[bodyClose:]
		}
	}

	return htmlSrc
}

func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
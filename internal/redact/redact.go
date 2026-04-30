package redact

import "regexp"

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[-_ .]?key|token|secret|password|authorization)(\s*[:=]\s*)([^ \t\r\n"'` + "`" + `]+)`),
	regexp.MustCompile(`(?i)(bearer)(\s+)([A-Za-z0-9._~+/=-]{12,})`),
}

func Text(text string) string {
	for _, pattern := range patterns {
		text = pattern.ReplaceAllString(text, `$1$2[redacted]`)
	}
	return text
}

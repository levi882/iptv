package redact

import "regexp"

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)((?:UserToken|UserID|STBID|Authenticator|stbinfo|prmid)=)[^&\s"'<>]+`),
	regexp.MustCompile(`(?im)^((?:(?:HB|PROVIDER)_)?(?:USER_TOKEN|USER_ID|STBID|AUTHENTICATOR|STBINFO|PRMID)=).*$`),
	regexp.MustCompile(`(?i)(CTCSetConfig\(\s*['"]UserToken['"]\s*,\s*['"])[^'"]+`),
}

// Sensitive removes subscriber credentials from provider errors before they
// are logged or returned by the local status API.
func Sensitive(value string) string {
	for _, pattern := range sensitivePatterns {
		value = pattern.ReplaceAllString(value, `${1}[redacted]`)
	}
	return value
}

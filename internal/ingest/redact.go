package ingest

import (
	"regexp"
	"strings"
)

var (
	reBearer     = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)\S+`)
	rePEM        = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]+PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]+PRIVATE KEY-----`)
	reJWT        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	reAWSKey     = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reGitHubPAT  = regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`)
	reSlackToken = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)
	reGenericKey = regexp.MustCompile(`(?i)\b(api[_-]?key|apikey|secret|token|password|passwd|access_key)\b\s*[=:]\s*['"]?[^\s'"]+`)
)

func redactString(s string) string {
	if s == "" {
		return s
	}
	out := s
	out = rePEM.ReplaceAllString(out, "${REDACTED:pem}")
	out = reBearer.ReplaceAllString(out, "${1}${REDACTED:bearer}")
	out = reJWT.ReplaceAllString(out, "${REDACTED:jwt}")
	out = reAWSKey.ReplaceAllString(out, "${REDACTED:aws_key}")
	out = reGitHubPAT.ReplaceAllString(out, "${REDACTED:github_pat}")
	out = reSlackToken.ReplaceAllString(out, "${REDACTED:slack}")
	out = reGenericKey.ReplaceAllStringFunc(out, func(m string) string {
		for _, sep := range []string{"=", ":"} {
			if i := strings.Index(m, sep); i >= 0 {
				return m[:i+1] + "${REDACTED:secret}"
			}
		}
		return "${REDACTED:secret}"
	})
	for _, key := range []string{"API_KEY", "api_key", "PASSWORD", "password", "TOKEN", "token", "SECRET", "secret"} {
		out = redactKeyValue(out, key)
	}
	return out
}

func redactKeyValue(s, key string) string {
	for {
		i := strings.Index(s, key+"=")
		if i < 0 {
			break
		}
		start := i + len(key) + 1
		end := start
		for end < len(s) && s[end] != ' ' && s[end] != '\'' && s[end] != '"' && s[end] != '\n' {
			end++
		}
		s = s[:start] + "${REDACTED:" + strings.ToLower(key) + "}" + s[end:]
		if !strings.Contains(s[start:], key+"=") {
			break
		}
	}
	return s
}

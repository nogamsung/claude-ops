package ci

import (
	"regexp"
	"strings"
)

// secretPatterns are applied in order to a log buffer before it ever
// reaches the Claude prompt. PRD R2 motivation: gh run view --log-failed
// can echo workflow expressions or environment variables that leak
// tokens; everything below masks values rather than keys so operators
// can still see *what* was assigned without seeing the actual secret.
var secretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// GitHub Actions secret expressions: ${{ secrets.NAME }} → ${{ secrets.*** }}
	{regexp.MustCompile(`\$\{\{\s*secrets\.[A-Za-z0-9_]+\s*\}\}`), "${{ secrets.*** }}"},
	// Generic key=value style assignments. Matches `token: abc`, `API_KEY=foo`,
	// `password=bar`, etc. The key is preserved; the value redacted.
	{
		regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|access[_-]?key|private[_-]?key|bearer)\s*[:=]\s*[^\s'"]{4,}`),
		"$1=***",
	},
	// Bearer tokens in headers: Authorization: Bearer xxxxx
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{8,}`), "Bearer ***"},
	// GitHub PAT prefixes (ghp_, gho_, ghu_, ghs_, ghr_) followed by 36+ chars.
	{regexp.MustCompile(`gh[posur]_[A-Za-z0-9]{20,}`), "***"},
}

// MaskSecrets applies the configured masking patterns to a log buffer.
// Idempotent — running it twice is a no-op on already-masked output.
func MaskSecrets(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// TruncateLogTail returns at most maxLines trailing lines of s. If a
// truncation occurred a [truncated] marker is prepended so the consumer
// (the ci-fix prompt) can tell.
func TruncateLogTail(s string, maxLines int) string {
	if maxLines <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	tail := lines[len(lines)-maxLines:]
	return "[truncated " + itoa(len(lines)-maxLines) + " earlier lines]\n" + strings.Join(tail, "\n")
}

// itoa is a tiny strconv.Itoa that lets us avoid importing strconv just
// for one number formatting.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

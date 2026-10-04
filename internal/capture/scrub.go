package capture

import (
	"math"
	"regexp"
	"strings"
)

const scrubbed = "«scrubbed»"

const secretName = `[A-Za-z0-9_-]*(?i:key|token|secret|passwd|password)[A-Za-z0-9_-]*`

var (
	// scheme://user:pass@ -> group 1 is user:pass.
	reURLUser = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://([^\s/@:'"]+:[^\s/@'"]*)@`)
	// Authorization: Bearer x, "Cookie": "a=b" -> group 1 is the value.
	reHeader = regexp.MustCompile(`(?i)\b(?:authorization|cookie)["']?\s*:\s*(?:[rbfu]{0,2}["'])?([^"'\n]+)`)
	// "API_KEY": "v" -> group 1 or 2 is the value. Python string prefixes (f"...") are allowed.
	reAssignDict = regexp.MustCompile(`["']` + secretName + `["']\s*:\s*[rbfuRBFU]{0,2}(?:"([^"\n]*)"|'([^'\n]*)')`)
	// API_KEY = "v" -> group 1 or 2 is the value.
	reAssignQuoted = regexp.MustCompile(`\b` + secretName + `\s*=\s*[rbfuRBFU]{0,2}(?:"([^"\n]*)"|'([^'\n]*)')`)
	// API_KEY=v -> group 1 is the bare value.
	reAssignBare = regexp.MustCompile(`\b(` + secretName + `)=([^\s"'` + "`" + `;&|(){}\[\],]+)`)
	// Known token prefixes, replaced whole.
	rePrefix = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,})`)
	// Candidate high-entropy tokens.
	reLong = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
)

// Scrub replaces secrets with «scrubbed» per the golden list in docs/HOOK.md: URL userinfo,
// Authorization and Cookie header values, values assigned to names containing KEY, TOKEN, SECRET,
// PASSWORD or PASSWD, known token prefixes, and long high-entropy tokens. File paths, git SHAs and
// ordinary identifiers are left alone.
func Scrub(s string) string {
	s = replaceGroups(reURLUser, s, nil)
	s = replaceGroups(reHeader, s, nil)
	quoted := func(m []string) bool {
		v := m[1] + m[2]
		return !skipName(m[0]) && !pathLike(v) && !strings.HasPrefix(v, "$")
	}
	s = replaceGroups(reAssignDict, s, quoted)
	s = replaceGroups(reAssignQuoted, s, quoted)
	s = replaceGroups(reAssignBare, s, func(m []string) bool {
		return !skipName(m[1]) && !codeValue(m[2]) && !pathLike(m[2])
	})
	s = rePrefix.ReplaceAllStringFunc(s, func(t string) string {
		if strings.HasPrefix(t, "sk-") && !strings.ContainsAny(t[3:], "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			return t // sk-some-component-name, not a key
		}
		return scrubbed
	})
	return reLong.ReplaceAllStringFunc(s, scrubToken)
}

// scrubToken scrubs a 32+ character token when it is a secret. A token with slashes and no base64
// padding or plus is a path: only its own secret segments are scrubbed.
func scrubToken(t string) string {
	if strings.Contains(t, "/") && !strings.ContainsAny(t, "+=") {
		segs := strings.Split(t, "/")
		for i, seg := range segs {
			if len(seg) >= 32 && secretToken(seg) {
				segs[i] = scrubbed
			}
		}
		return strings.Join(segs, "/")
	}
	if eq := strings.IndexByte(t, '='); eq > 0 && isAssign(t) && strings.Trim(t[eq:], "=") != "" {
		if v := t[eq+1:]; len(v) >= 32 && secretToken(v) {
			return t[:eq+1] + scrubbed // NAME=value keeps the name
		}
		return t
	}
	if secretToken(t) {
		return scrubbed
	}
	return t
}

// replaceGroups replaces the last non-empty capture group of every match with «scrubbed». ok, when
// set, receives the match's submatches and may veto the replacement.
func replaceGroups(re *regexp.Regexp, s string, ok func(m []string) bool) string {
	idx := re.FindAllStringSubmatchIndex(s, -1)
	if idx == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range idx {
		if ok != nil {
			subs := make([]string, len(m)/2)
			for g := range subs {
				if m[2*g] >= 0 {
					subs[g] = s[m[2*g]:m[2*g+1]]
				}
			}
			if !ok(subs) {
				continue
			}
		}
		for g := len(m)/2 - 1; g >= 1; g-- {
			if m[2*g] >= 0 && m[2*g+1] > m[2*g] {
				b.WriteString(s[last:m[2*g]])
				b.WriteString(scrubbed)
				last = m[2*g+1]
				break
			}
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

// skipName exempts, by the name at the start of m, the ubiquitous Python sort/dict key and token
// counters (key=lambda, max_tokens).
func skipName(m string) bool {
	name := strings.TrimLeft(m, `"'`)
	if i := strings.IndexAny(name, " \t=:\"'"); i >= 0 {
		name = m[:i]
	}
	return name == "key" || name == "keys" || strings.HasSuffix(name, "tokens")
}

// codeValue reports whether an unquoted assigned value is code or a reference, not a literal secret.
func codeValue(v string) bool {
	switch v {
	case "lambda", "None", "True", "False", "not", scrubbed:
		return true
	}
	return v[0] == '$' || isDigits(v) || strings.ContainsAny(v, ".") && isDottedIdent(v)
}

func isDottedIdent(v string) bool {
	for _, part := range strings.Split(v, ".") {
		if part == "" || !isIdentStart(part[0]) {
			return false
		}
		for i := 1; i < len(part); i++ {
			if !isIdent(part[i]) {
				return false
			}
		}
	}
	return true
}

// secretToken decides whether a 32+ character token is a secret: high entropy and not identifier-like.
func secretToken(t string) bool {
	if len(t) == 40 && strings.Trim(t, "0123456789abcdef") == "" {
		return false // git SHA
	}
	return !wordy(t) && entropy(t) > 4.0
}

// wordy reports identifier-like text (snake_case, kebab-case, CamelCase): few case or letter/digit
// switches per character. Random tokens switch on about 40% of characters.
func wordy(t string) bool {
	class := func(c byte) int {
		switch {
		case c >= 'a' && c <= 'z':
			return 1
		case c >= 'A' && c <= 'Z':
			return 2
		case c >= '0' && c <= '9':
			return 3
		}
		return 0
	}
	switches := 0
	for i := 1; i < len(t); i++ {
		a, b := class(t[i-1]), class(t[i])
		if a != 0 && b != 0 && a != b && !(a == 2 && b == 1) {
			switches++
		}
	}
	return float64(switches) < 0.25*float64(len(t))
}

func entropy(t string) float64 {
	var freq [256]int
	for i := 0; i < len(t); i++ {
		freq[t[i]]++
	}
	h, n := 0.0, float64(len(t))
	for _, f := range freq {
		if f > 0 {
			p := float64(f) / n
			h -= p * math.Log2(p)
		}
	}
	return h
}

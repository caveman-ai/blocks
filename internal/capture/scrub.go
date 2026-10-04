package capture

import (
	"math"
	"regexp"
	"strings"
)

const scrubbed = "«scrubbed»"

// secretName matches a name that holds a secret. `auth` only counts as a whole word or before a
// separator (auth, oauth, auth_token), so author and authority stay.
const secretName = `[A-Za-z0-9_-]*(?i:(?:key|token|secret|passwd|password|pwd|credential)[A-Za-z0-9_-]*|auth(?:[_-][A-Za-z0-9_-]*)?)`

var (
	// scheme://user:pass@ -> group 1 is user:pass.
	reURLUser = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://([^\s/@:'"]+:[^\s/@'"]*)@`)
	// Authorization: Bearer x, "Cookie": "a=b" -> group 1 is the value.
	reHeader = regexp.MustCompile(`(?i)\b(?:authorization|cookie)["']?\s*:\s*(?:[rbfu]{0,2}["'])?([^"'\n]+)`)
	// curl -H 'X-Api-Key: v' / --header "X-Auth-Token: v" / -H X-Api-Key:v -> the last group is the value.
	reCurlHeader = regexp.MustCompile(`(?:-H|--header)\s*(?:'` + secretName + `\s*:\s*([^'\n]+)|"` +
		secretName + `\s*:\s*([^"\n]+)|` + secretName + `:\s*([^\s"']+))`)
	// "API_KEY": "v" -> group 1 or 2 is the value. Python string prefixes (f"...") are allowed.
	reAssignDict = regexp.MustCompile(`["']` + secretName + `["']\s*:\s*[rbfuRBFU]{0,2}(?:"([^"\n]*)"|'([^'\n]*)')`)
	// API_KEY = "v" -> group 1 or 2 is the value.
	reAssignQuoted = regexp.MustCompile(`\b` + secretName + `\s*=\s*[rbfuRBFU]{0,2}(?:"([^"\n]*)"|'([^'\n]*)')`)
	// API_KEY=v -> group 1 is the bare value.
	reAssignBare = regexp.MustCompile(`\b(` + secretName + `)=([^\s"'` + "`" + `;&|(){}\[\],]+)`)
	// Known token prefixes, replaced whole.
	rePrefix = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|[sr]k_(?:live|test)_[A-Za-z0-9]{10,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|xapp-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{30,})`)
	// Candidate high-entropy tokens.
	reLong = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
)

// Scrub replaces secrets with «scrubbed» per the golden list in docs/HOOK.md: URL userinfo,
// Authorization and Cookie header values, curl headers and values assigned to names containing KEY,
// TOKEN, SECRET, PASSWORD, PASSWD, PWD, CREDENTIAL or AUTH, known token prefixes, hex runs of 32+ that
// are not a git or SHA-256 digest, and long high-entropy tokens. File paths, digests and ordinary
// identifiers are left alone.
func Scrub(s string) string {
	out, _ := scrub(s)
	return out
}

// scrub is Scrub that also returns the original text of every span it replaced.
func scrub(s string) (string, []string) {
	var spans []string
	rec := func(v string) string {
		spans = append(spans, v)
		return scrubbed
	}
	// Keyword prefilters skip regexes that cannot match; the regexes dominate the hook's Extract.
	lower := strings.ToLower(s)
	if strings.Contains(s, "://") {
		s = replaceGroups(reURLUser, s, nil, rec)
	}
	if hasAny(lower, "authorization", "cookie") {
		s = replaceGroups(reHeader, s, nil, rec)
	}
	s = replaceGroups(reCurlHeader, s, nil, rec)
	if hasAny(lower, "key", "token", "secret", "passw", "pwd", "credential", "auth") {
		quoted := func(m []string) bool {
			v := m[1] + m[2]
			return (!skipName(m[0]) || hexSecret(v)) && !pathLike(v) && !strings.HasPrefix(v, "$")
		}
		s = replaceGroups(reAssignDict, s, quoted, rec)
		s = replaceGroups(reAssignQuoted, s, quoted, rec)
		s = replaceGroups(reAssignBare, s, func(m []string) bool {
			return (!skipName(m[1]) || hexSecret(m[2])) && !codeValue(m[2]) && !pathLike(m[2])
		}, rec)
	}
	s = rePrefix.ReplaceAllStringFunc(s, func(t string) string {
		if strings.HasPrefix(t, "sk-") && !strings.ContainsAny(t[3:], "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			return t // sk-some-component-name, not a key
		}
		return rec(t)
	})
	s = reLong.ReplaceAllStringFunc(s, func(t string) string { return scrubToken(t, rec) })
	// A secret scrubbed once is scrubbed everywhere: print(KEY_VALUE) after KEY = "..." must not
	// keep it. Short spans (a user:pw) are too likely to be ordinary words.
	for _, sp := range spans {
		if len(sp) >= 8 && strings.Contains(s, sp) {
			s = strings.ReplaceAll(s, sp, scrubbed)
		}
	}
	return s, spans
}

func hasAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// scrubToken scrubs a 32+ character token when it is a secret. A slashed token is a path when a
// segment looks like one (has a dot, or is a short lowercase word): only its own secret segments are
// scrubbed. Any other slashed token (base64, an AWS secret key) is judged whole.
func scrubToken(t string, rec func(string) string) string {
	if strings.Contains(t, "/") && pathSegments(t) {
		segs := strings.Split(t, "/")
		for i, seg := range segs {
			if len(seg) >= 32 && secretToken(seg) {
				segs[i] = rec(seg)
			}
		}
		return strings.Join(segs, "/")
	}
	if eq := strings.IndexByte(t, '='); eq > 0 && isAssign(t) && strings.Trim(t[eq:], "=") != "" {
		if v := t[eq+1:]; len(v) >= 32 && secretToken(v) {
			return t[:eq+1] + rec(v) // NAME=value keeps the name
		}
		return t
	}
	if secretToken(t) {
		return rec(t)
	}
	return t
}

// pathSegments reports whether a slashed token reads as a path: some segment contains a dot or is a
// plain lowercase word of at most 12 letters.
func pathSegments(t string) bool {
	for _, seg := range strings.Split(t, "/") {
		if strings.Contains(seg, ".") || seg != "" && len(seg) <= 12 && strings.Trim(seg, "abcdefghijklmnopqrstuvwxyz") == "" {
			return true
		}
	}
	return false
}

// replaceGroups replaces the last non-empty capture group of every match with rec(group). ok, when
// set, receives the match's submatches and may veto the replacement.
func replaceGroups(re *regexp.Regexp, s string, ok func(m []string) bool, rec func(string) string) string {
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
				b.WriteString(rec(s[m[2*g]:m[2*g+1]]))
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
		name = name[:i]
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

// secretToken decides whether a 32+ character token is a secret: a hex run that is not a digest, or
// high entropy and not identifier-like. A slashed token that reached here is not a path, and
// identifiers have no slashes, so only its entropy counts.
func secretToken(t string) bool {
	if isHex(t) {
		return len(t) != 40 && len(t) != 64 // git SHA-1 and SHA-256 digests stay
	}
	return (strings.Contains(t, "/") || !wordy(t)) && entropy(t) > 4.0
}

// hexSecret reports a 32+ hex value, a secret even under an exempt name like key.
func hexSecret(v string) bool { return len(v) >= 32 && isHex(v) }

func isHex(t string) bool { return t != "" && strings.Trim(t, "0123456789abcdefABCDEF") == "" }

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

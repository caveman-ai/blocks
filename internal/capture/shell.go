package capture

import "strings"

// shCmd is one simple command of a parsed shell string. Nothing is expanded.
type shCmd struct {
	words    []string
	docs     []string // heredoc and here-string bodies, in order
	hasDoc   bool     // a heredoc was declared; keeps the command even without words
	redirOut bool     // stdout goes to a file instead of the caller
	pipe     bool     // stdout is piped into the next command
}

type pendingDoc struct {
	cmd   int
	delim string
	strip bool
}

type shParser struct {
	s       string
	i       int
	cmds    []shCmd
	cur     shCmd
	pending []pendingDoc
	buf     []byte
}

// parseShell splits a shell string into simple commands. It understands single and double quotes,
// backslashes, comments, line continuations, redirections, here-strings and heredocs.
func parseShell(s string) []shCmd {
	p := shParser{s: s}
	for p.i < len(s) {
		switch c := s[p.i]; c {
		case ' ', '\t', '\r':
			p.i++
		case '\n':
			p.i++
			p.finish()
			p.readDocs()
		case ';', '(', ')':
			p.i++
			p.finish()
		case '&':
			p.i++
			switch p.at(0) {
			case '>': // &> and &>>
				p.cur.redirOut = true
				p.redirect("")
			case '&':
				p.i++
				p.finish()
			default:
				p.finish()
			}
		case '|':
			p.i++
			if p.at(0) == '|' {
				p.i++
				p.finish()
				continue
			}
			if p.at(0) == '&' {
				p.i++
			}
			p.cur.pipe = true
			p.finish()
		case '<', '>':
			p.redirect("")
		case '#':
			for p.i < len(s) && s[p.i] != '\n' {
				p.i++
			}
		default:
			w := p.word()
			if c := p.at(0); (c == '<' || c == '>') && isDigits(w) {
				p.redirect(w) // fd number, as in 2>&1
				continue
			}
			p.cur.words = append(p.cur.words, w)
		}
	}
	p.finish()
	return p.cmds
}

func (p *shParser) at(off int) byte {
	if p.i+off < len(p.s) {
		return p.s[p.i+off]
	}
	return 0
}

func (p *shParser) finish() {
	if len(p.cur.words) > 0 || p.cur.hasDoc || len(p.cur.docs) > 0 {
		p.cmds = append(p.cmds, p.cur)
	}
	p.cur = shCmd{}
}

func (p *shParser) skipBlanks() {
	for p.i < len(p.s) && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

// redirect consumes one redirection starting at '<' or '>'; fd is the explicit fd number or "".
func (p *shParser) redirect(fd string) {
	rest := p.s[p.i:]
	switch {
	case strings.HasPrefix(rest, "<<<"):
		p.i += 3
		p.skipBlanks()
		p.cur.docs = append(p.cur.docs, p.word())
		return
	case strings.HasPrefix(rest, "<<"):
		p.i += 2
		strip := p.at(0) == '-'
		if strip {
			p.i++
		}
		p.skipBlanks()
		p.pending = append(p.pending, pendingDoc{cmd: len(p.cmds), delim: p.word(), strip: strip})
		p.cur.hasDoc = true
		return
	}
	out := p.s[p.i] == '>'
	p.i++
	dup := false
	for p.i < len(p.s) && (p.s[p.i] == '>' || p.s[p.i] == '&' || p.s[p.i] == '|') {
		dup = dup || p.s[p.i] == '&'
		p.i++
	}
	if out && !dup && (fd == "" || fd == "1") {
		p.cur.redirOut = true
	}
	p.skipBlanks()
	p.word() // target, discarded
}

// readDocs attaches the bodies of heredocs declared on the line just ended.
func (p *shParser) readDocs() {
	for _, d := range p.pending {
		body := p.heredoc(d.delim, d.strip)
		if d.cmd < len(p.cmds) {
			p.cmds[d.cmd].docs = append(p.cmds[d.cmd].docs, body)
		}
	}
	p.pending = p.pending[:0]
}

func (p *shParser) heredoc(delim string, strip bool) string {
	s, start := p.s, p.i
	for pos := p.i; pos < len(s); {
		end, next := len(s), len(s)
		if j := strings.IndexByte(s[pos:], '\n'); j >= 0 {
			end, next = pos+j, pos+j+1
		}
		line := strings.TrimSuffix(s[pos:end], "\r")
		if strip {
			line = strings.TrimLeft(line, "\t")
		}
		if line == delim {
			p.i = next
			return stripTabs(s[start:pos], strip)
		}
		pos = next
	}
	p.i = len(s)
	return stripTabs(s[start:], strip)
}

func stripTabs(body string, strip bool) string {
	if !strip {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, "\t")
	}
	return strings.Join(lines, "\n")
}

// word reads one shell word and returns its unquoted value.
func (p *shParser) word() string {
	s := p.s
	p.buf = p.buf[:0]
	for p.i < len(s) {
		c := s[p.i]
		switch c {
		case ' ', '\t', '\r', '\n', ';', '&', '|', '<', '>', '(', ')':
			return string(p.buf)
		case '\'':
			j := strings.IndexByte(s[p.i+1:], '\'')
			if j < 0 {
				p.buf = append(p.buf, s[p.i+1:]...)
				p.i = len(s)
				return string(p.buf)
			}
			p.buf = append(p.buf, s[p.i+1:p.i+1+j]...)
			p.i += j + 2
		case '"':
			p.i++
			for p.i < len(s) && s[p.i] != '"' {
				if s[p.i] == '\\' && p.i+1 < len(s) {
					switch n := s[p.i+1]; n {
					case '"', '\\', '$', '`':
						p.buf = append(p.buf, n)
						p.i += 2
						continue
					case '\n':
						p.i += 2
						continue
					}
				}
				p.buf = append(p.buf, s[p.i])
				p.i++
			}
			if p.i < len(s) {
				p.i++
			}
		case '\\':
			if p.i+1 < len(s) && s[p.i+1] != '\n' {
				p.buf = append(p.buf, s[p.i+1])
			}
			p.i += 2
		case '$':
			switch p.at(1) {
			case '\'': // $'...': kept literally, escapes are not interpreted
				p.i++
			case '(':
				depth := 0
				for p.i < len(s) {
					ch := s[p.i]
					p.buf = append(p.buf, ch)
					p.i++
					if ch == '(' {
						depth++
					} else if ch == ')' {
						if depth--; depth == 0 {
							break
						}
					}
				}
			default:
				p.buf = append(p.buf, c)
				p.i++
			}
		case '`':
			j := strings.IndexByte(s[p.i+1:], '`')
			end := len(s)
			if j >= 0 {
				end = p.i + 2 + j
			}
			p.buf = append(p.buf, s[p.i:end]...)
			p.i = end
		default:
			p.buf = append(p.buf, c)
			p.i++
		}
	}
	if p.i > len(s) {
		p.i = len(s)
	}
	return string(p.buf)
}

func isDigits(w string) bool {
	if w == "" {
		return false
	}
	for i := 0; i < len(w); i++ {
		if w[i] < '0' || w[i] > '9' {
			return false
		}
	}
	return true
}

func base(w string) string {
	return w[strings.LastIndexByte(w, '/')+1:]
}

// commandWords drops env assignments and transparent prefixes (exec, time, timeout N, ...).
func commandWords(w []string) []string {
	for len(w) > 0 {
		switch x := w[0]; {
		case isAssign(x), x == "exec", x == "time", x == "nohup", x == "command", x == "env", x == "{", x == "!":
			w = w[1:]
		case x == "timeout":
			w = w[1:]
			for len(w) > 0 && strings.HasPrefix(w[0], "-") {
				w = w[1:]
			}
			if len(w) > 0 {
				w = w[1:]
			}
		default:
			return w
		}
	}
	return w
}

func isAssign(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := w[i]
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func isShell(w string) bool {
	switch base(w) {
	case "bash", "sh", "zsh", "dash":
		return true
	}
	return false
}

// shellScript returns the script of `bash -lc '<script>'` style words.
func shellScript(w []string) (string, bool) {
	if len(w) == 0 || !isShell(w[0]) {
		return "", false
	}
	for j := 1; j < len(w); j++ {
		a := w[j]
		switch {
		case a == "-o" || a == "+o":
			j++
		case len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.IndexByte(a, 'c') > 0:
			if j+1 < len(w) {
				return w[j+1], true
			}
			return "", false
		case a == "" || a[0] != '-' && a[0] != '+':
			return "", false // script file
		}
	}
	return "", false
}

// setupCmds are skipped when looking for the command a shell string "starts with".
var setupCmds = map[string]bool{
	"cd": true, "pushd": true, "popd": true, "set": true, "export": true, "unset": true,
	"source": true, ".": true, "shopt": true, "ulimit": true, "umask": true,
}

// leadCommand returns the parsed commands and the index of the first one that is not setup,
// after unwrapping a `bash -lc '...'` wrapper. k is -1 when there is none.
func leadCommand(command string, depth int) (cmds []shCmd, k int) {
	cmds = parseShell(command)
	for k := range cmds {
		w := commandWords(cmds[k].words)
		if len(w) == 0 || setupCmds[w[0]] {
			continue
		}
		if inner, ok := shellScript(w); ok && depth < 3 {
			return leadCommand(inner, depth+1)
		}
		return cmds, k
	}
	return nil, -1
}

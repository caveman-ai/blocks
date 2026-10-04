package blockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// isBlankHeader reports whether a header line is a bare `#` (any line ending or trailing spaces).
func isBlankHeader(line string) bool { return strings.TrimRight(line, " \r\n") == "#" }

// contentHash hashes the canonical form: LF source without the [stamp] table and without the bare
// `#` lines directly above it (or above the closing fence when there is no stamp). Dropping those
// separator lines keeps the hash a fixed point of WithStamp, which inserts one.
func contentHash(b *Block, fixtures map[string][]byte) string {
	lines := strings.Split(string(normalize(b.Source)), "\n")
	cut := b.FenceEnd
	if b.StampLine >= 0 {
		cut = b.StampLine
	}
	for cut-1 > b.FenceStart && isBlankHeader(lines[cut-1]) {
		cut--
	}
	kept := append(append([]string{}, lines[:cut]...), lines[b.FenceEnd:]...)

	h := sha256.New()
	h.Write([]byte(strings.Join(kept, "\n")))
	paths := make([]string, 0, len(fixtures))
	for p := range fixtures {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		sum := sha256.Sum256(fixtures[p])
		fmt.Fprintf(h, "%s\x00%s\n", p, hex.EncodeToString(sum[:]))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func withStamp(b *Block, s Stamp) []byte {
	src := string(b.Source)
	eol := "\n"
	if strings.Contains(src, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(src, "\n")

	var stamp []string
	if s != (Stamp{}) {
		stamp = append(stamp, "# [stamp]"+eol)
		if s.Verified != "" {
			stamp = append(stamp, fmt.Sprintf("# verified = %q%s", s.Verified, eol))
		}
		if s.State != "" {
			stamp = append(stamp, fmt.Sprintf("# state = %q%s", s.State, eol))
		}
	}

	from, to := b.FenceEnd, b.FenceEnd
	switch {
	case b.StampLine >= 0:
		from = b.StampLine
		if stamp == nil && from-1 > b.FenceStart && isBlankHeader(lines[from-1]) {
			from-- // removing the table removes its separator too
		}
	case stamp != nil && !isBlankHeader(lines[from-1]):
		stamp = append([]string{"#" + eol}, stamp...)
	}

	var out strings.Builder
	for _, part := range [][]string{lines[:from], stamp, lines[to:]} {
		for _, l := range part {
			out.WriteString(l)
		}
	}
	return []byte(out.String())
}

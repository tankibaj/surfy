package logview

import (
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"regexp"
	"strings"
)

const reset = "\x1b[0m"

var severity = regexp.MustCompile(`(?i)(debug|info|warn(?:ing)?|error|fatal|panic)(?:\b|:)`)
var serviceTag = regexp.MustCompile(`\[([A-Za-z0-9][A-Za-z0-9_-]{0,31})\]`)

func Enabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, e := f.Stat()
	return e == nil && info.Mode()&os.ModeCharDevice != 0
}

func Format(line string, color bool) string {
	start := 0
	if tag := serviceTag.FindStringIndex(line); tag != nil {
		start = tag[1]
	}
	match := severity.FindStringSubmatchIndex(line[start:])
	if match == nil {
		return colorServiceTag(line, color)
	}
	for i := range match {
		if match[i] >= 0 {
			match[i] += start
		}
	}
	word := strings.ToLower(line[match[2]:match[3]])
	indicator, code := "·", "90"
	switch word {
	case "info":
		indicator, code = "ℹ", "34"
	case "warn", "warning":
		indicator, code = "⚠", "33"
	case "error", "fatal", "panic":
		indicator, code = "✖", "31"
	}
	prefix := indicator + " "
	if color {
		prefix = "\x1b[" + code + "m" + indicator + reset + " "
	}
	return colorServiceTag(line[:match[0]]+prefix+line[match[0]:], color)
}

func Write(w io.Writer, data []byte, color bool) error {
	parts := strings.SplitAfter(string(data), "\n")
	for _, part := range parts {
		if part == "" {
			continue
		}
		newline := strings.HasSuffix(part, "\n")
		line := strings.TrimSuffix(part, "\n")
		if _, e := fmt.Fprint(w, Format(line, color)); e != nil {
			return e
		}
		if newline {
			if _, e := fmt.Fprint(w, "\n"); e != nil {
				return e
			}
		}
	}
	return nil
}

func colorServiceTag(line string, color bool) string {
	if !color {
		return line
	}
	m := serviceTag.FindStringSubmatchIndex(line)
	if m == nil {
		return line
	}
	name := line[m[2]:m[3]]
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	palette := []string{"31", "32", "33", "34", "35", "36", "91", "92", "93", "94", "95", "96"}
	code := palette[int(h.Sum32())%len(palette)]
	return line[:m[0]] + "\x1b[" + code + "m" + line[m[0]:m[1]] + reset + line[m[1]:]
}

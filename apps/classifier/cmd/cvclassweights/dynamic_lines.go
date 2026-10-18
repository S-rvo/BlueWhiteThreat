package main

import "strings"

// This experimental grouping signature targets repeated interfaces with small
// changing regions. It is never used as a relevance label or model feature.
type linePrint struct {
	lines map[string]int
	words int
}

func fingerprintLines(doc string) linePrint {
	p := linePrint{lines: map[string]int{}}
	for _, line := range strings.Split(doc, "\n") {
		line = strings.ToLower(strings.Join(strings.Fields(line), " "))
		if line == "" {
			continue
		}
		if _, exists := p.lines[line]; !exists {
			words := len(strings.Fields(line))
			p.lines[line] = words
			p.words += words
		}
	}
	return p
}

func similarLines(a, b linePrint) bool {
	if len(a.lines) < 30 || len(b.lines) < 30 {
		return false
	}
	common, words := 0, 0
	for line, n := range a.lines {
		if _, exists := b.lines[line]; exists {
			common++
			words += n
		}
	}
	union := len(a.lines) + len(b.lines) - common
	return common*10 >= union*9 && words*5 >= a.words*4 && words*5 >= b.words*4
}

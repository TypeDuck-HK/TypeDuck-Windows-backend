package rime

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const typeDuckLookupDictFile = "jyut6ping3_scolar.dict.yaml"

type typeDuckLookup struct {
	mu        sync.Mutex
	loaded    bool
	dictPath  string
	entriesBy map[string][]string
}

func newTypeDuckLookup(sharedDir string) *typeDuckLookup {
	if strings.TrimSpace(sharedDir) == "" {
		return nil
	}
	return &typeDuckLookup{
		dictPath:  filepath.Join(sharedDir, typeDuckLookupDictFile),
		entriesBy: map[string][]string{},
	}
}

func (l *typeDuckLookup) enrichCandidates(candidates []candidateItem) []candidateItem {
	if l == nil || len(candidates) == 0 {
		return candidates
	}
	if !l.ensureLoaded() {
		return candidates
	}
	enriched := append([]candidateItem(nil), candidates...)
	for i := range enriched {
		if strings.Contains(enriched[i].Comment, "\f") {
			continue
		}
		rows := l.rowsForText(enriched[i].Text)
		if len(rows) == 0 {
			continue
		}
		enriched[i].Comment = typeDuckRawLookupComment(enriched[i].Text, enriched[i].Comment, rows)
	}
	return enriched
}

func (l *typeDuckLookup) rowsForText(text string) []string {
	if l == nil || text == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	rows := l.entriesBy[text]
	if len(rows) == 0 {
		return nil
	}
	return append([]string(nil), rows...)
}

func (l *typeDuckLookup) ensureLoaded() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loaded {
		return len(l.entriesBy) > 0
	}
	l.loaded = true

	file, err := os.Open(l.dictPath)
	if err != nil {
		log.Printf("TypeDuck lookup dictionary unavailable: %s: %v", l.dictPath, err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	const maxLineSize = 256 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "...") {
			continue
		}
		csv, text, ok := splitTypeDuckDictLine(line)
		if !ok {
			continue
		}
		l.entriesBy[text] = append(l.entriesBy[text], csv)
	}
	if err := scanner.Err(); err != nil {
		log.Printf("TypeDuck lookup dictionary read failed: %s: %v", l.dictPath, err)
		l.entriesBy = map[string][]string{}
		return false
	}
	return len(l.entriesBy) > 0
}

func splitTypeDuckDictLine(line string) (csv string, text string, ok bool) {
	tab := strings.LastIndexByte(line, '\t')
	if tab <= 0 || tab+1 >= len(line) {
		return "", "", false
	}
	csv = strings.TrimSpace(line[:tab])
	text = strings.TrimSpace(line[tab+1:])
	if csv == "" || text == "" || strings.HasPrefix(csv, "#") {
		return "", "", false
	}
	return csv, text, true
}

func typeDuckRawLookupComment(text, note string, rows []string) string {
	var builder strings.Builder
	builder.Grow(len(note) + len(text)*len(rows) + 32*len(rows))
	builder.WriteString(note)
	builder.WriteByte('\f')
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		builder.WriteByte('\r')
		builder.WriteString("1,")
		builder.WriteString(text)
		builder.WriteByte(',')
		builder.WriteString(row)
	}
	return builder.String()
}

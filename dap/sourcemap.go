// Package dap implements a Debug Adapter Protocol proxy for the Serv language.
// It bridges VS Code (or any DAP client) to Delve — translating breakpoint
// positions and stack-frame line numbers between .srv source coordinates and
// the generated Go file coordinates.
package dap

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// SourceMap is a bidirectional mapping between line numbers in the generated
// Go file (.build/<hash>/main.go) and the original .srv source file.
//
// The codegen already emits "// .srv line N" comments immediately before each
// top-level statement, so no compiler changes are required to build this map.
type SourceMap struct {
	// srvFile is the absolute path of the original .srv source file.
	srvFile string
	// goFile is the absolute path of the generated Go file.
	goFile string

	// goToSrv maps a Go line number → .srv line number.
	goToSrv map[int]int
	// srvToGo maps a .srv line number → the first Go line that represents it.
	srvToGo map[int]int

	// sortedGoLines is a sorted slice of all Go lines in the map (for nearest lookup).
	sortedGoLines []int
}

// ParseSourceMap reads the generated Go file at genGoPath and constructs a
// SourceMap by scanning for "// .srv line N" comment markers emitted by the
// Serv codegen.
//
// Each such comment is assumed to precede the Go statement that corresponds to
// the referenced .srv line, so the mapping recorded is:
//
//	(commentLine + 1) → srvLine   (the statement is on the next line)
//	commentLine       → srvLine   (also recorded for exact-comment hits)
func ParseSourceMap(genGoPath, srvPath string) (*SourceMap, error) {
	f, err := os.Open(genGoPath)
	if err != nil {
		return nil, fmt.Errorf("sourcemap: open %q: %w", genGoPath, err)
	}
	defer f.Close()

	sm := &SourceMap{
		srvFile: srvPath,
		goFile:  genGoPath,
		goToSrv: make(map[int]int),
		srvToGo: make(map[int]int),
	}

	scanner := bufio.NewScanner(f)
	goLine := 0
	for scanner.Scan() {
		goLine++
		line := strings.TrimSpace(scanner.Text())

		// Match "// .srv line N" comments emitted by the Serv codegen.
		if strings.HasPrefix(line, "// .srv line ") {
			rest := strings.TrimPrefix(line, "// .srv line ")
			srvLine, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				continue
			}
			// Record: the comment line itself maps to srvLine.
			sm.goToSrv[goLine] = srvLine
			// Record: the Go statement on the next line maps to srvLine.
			sm.goToSrv[goLine+1] = srvLine
			// Record the first Go line for this srv line (prefer earlier lines).
			if _, exists := sm.srvToGo[srvLine]; !exists {
				sm.srvToGo[srvLine] = goLine + 1
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sourcemap: scan %q: %w", genGoPath, err)
	}

	// Build sorted list of Go lines for nearest-neighbour lookup.
	sm.sortedGoLines = make([]int, 0, len(sm.goToSrv))
	for gl := range sm.goToSrv {
		sm.sortedGoLines = append(sm.sortedGoLines, gl)
	}
	sort.Ints(sm.sortedGoLines)

	return sm, nil
}

// GoToSrv returns the .srv line number that corresponds to the given Go line
// number. ok is false when no mapping exists.
func (sm *SourceMap) GoToSrv(goLine int) (srvLine int, ok bool) {
	srvLine, ok = sm.goToSrv[goLine]
	return
}

// SrvToGo returns the Go line number that best corresponds to the given .srv
// line number. ok is false when the map is empty.
func (sm *SourceMap) SrvToGo(srvLine int) (goLine int, ok bool) {
	if gl, exists := sm.srvToGo[srvLine]; exists {
		return gl, true
	}
	// No exact match — fall through to nearest-neighbour search.
	return sm.nearestGoLine(srvLine)
}

// nearestGoLine finds the Go line whose associated .srv line is closest to
// the requested srvLine. This handles cases where a breakpoint is placed on
// a line that has no direct mapping (e.g. a blank line or comment in .srv).
func (sm *SourceMap) nearestGoLine(srvLine int) (goLine int, ok bool) {
	if len(sm.sortedGoLines) == 0 {
		return 0, false
	}

	bestGoLine := sm.sortedGoLines[0]
	bestDiff := abs(sm.goToSrv[bestGoLine] - srvLine)

	for _, gl := range sm.sortedGoLines {
		diff := abs(sm.goToSrv[gl] - srvLine)
		if diff < bestDiff {
			bestDiff = diff
			bestGoLine = gl
		}
	}
	return bestGoLine, true
}

// GoFile returns the absolute path of the generated Go source file.
func (sm *SourceMap) GoFile() string { return sm.goFile }

// SrvFile returns the absolute path of the original .srv source file.
func (sm *SourceMap) SrvFile() string { return sm.srvFile }

// Len returns the number of Go lines that have a known .srv mapping.
func (sm *SourceMap) Len() int { return len(sm.goToSrv) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

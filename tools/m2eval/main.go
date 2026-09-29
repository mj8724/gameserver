// Command m2eval turns a `go test -json` stream plus row definitions into the
// per-matrix-row verdicts used by docs/acceptance/rehearsals/m2-offline.sh.
//
// It exists so the acceptance runner has no interpreter dependency beyond the
// pinned Go toolchain: the same command works on macOS, Linux, and Windows
// (where python3 is typically absent).
//
// Usage:
//
//	m2eval -rows rowdefs.tsv -json all.json -out rows.jsonl [-gov go1.27.1]
//
// rowdefs.tsv: one row per line, tab separated: id<TAB>title<TAB>test-name-regex
// rows.jsonl:  {"id":...,"title":...,"status":"PASS|FAIL","evidence":...}
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type testEvent struct {
	Action string
	Test   string
}

type rowDef struct {
	id      string
	title   string
	pattern *regexp.Regexp
	raw     string
}

type verdict struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

func main() {
	rowsPath := flag.String("rows", "", "row definitions (id<TAB>title<TAB>regex)")
	jsonPath := flag.String("json", "", "go test -json output")
	outPath := flag.String("out", "", "output jsonl path")
	goVersion := flag.String("gov", "unknown", "go version string for evidence text")
	flag.Parse()
	if *rowsPath == "" || *jsonPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "m2eval: -rows, -json and -out are required")
		os.Exit(2)
	}

	defs, err := readRows(*rowsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "m2eval: %v\n", err)
		os.Exit(2)
	}
	seen, failed, err := readEvents(*jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "m2eval: %v\n", err)
		os.Exit(2)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)

	out, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "m2eval: %v\n", err)
		os.Exit(2)
	}
	defer out.Close()
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for _, def := range defs {
		var matched []string
		for _, name := range names {
			if def.pattern.MatchString(name) {
				matched = append(matched, name)
			}
		}
		result := verdict{ID: def.id, Title: def.title}
		switch {
		case len(matched) == 0:
			result.Status = "FAIL"
			result.Evidence = fmt.Sprintf("选择器未匹配到任何测试（%s）", def.raw)
		default:
			var broken []string
			for _, name := range matched {
				if failed[name] {
					broken = append(broken, name)
				}
			}
			if len(broken) > 0 {
				result.Status = "FAIL"
				result.Evidence = "失败用例: " + strings.Join(broken, ", ")
			} else {
				result.Status = "PASS"
				sample := matched
				if len(sample) > 3 {
					sample = sample[:3]
				}
				suffix := ""
				if len(matched) > 3 {
					suffix = "…"
				}
				result.Evidence = fmt.Sprintf("%d 个用例全通过（E-OFF, %s）: %s%s",
					len(matched), *goVersion, strings.Join(sample, ", "), suffix)
			}
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "m2eval: %v\n", err)
			os.Exit(2)
		}
		fmt.Fprintln(writer, string(encoded))
	}
}

func readRows(path string) ([]rowDef, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var defs []rowDef
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("malformed row definition: %q", line)
		}
		pattern, err := regexp.Compile(parts[2])
		if err != nil {
			return nil, fmt.Errorf("row %s: %w", parts[0], err)
		}
		defs = append(defs, rowDef{id: parts[0], title: parts[1], pattern: pattern, raw: parts[2]})
	}
	return defs, scanner.Err()
}

func readEvents(path string) (map[string]bool, map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	seen := map[string]bool{}
	failed := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event testEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Test == "" {
			continue
		}
		seen[event.Test] = true
		if event.Action == "fail" {
			failed[event.Test] = true
		}
	}
	return seen, failed, scanner.Err()
}

// govulncheck_wrapper runs govulncheck, applies an ignore list, and (on failure)
// prints only the traces for unignored vulnerability IDs.
//
// Intended usage (via Makefile):
//
//	go run ./hack/govulncheck_wrapper/ -govulncheck ./bin/govulncheck -ignore .govulnignore ./...
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

var (
	vulnHeaderRe  = regexp.MustCompile(`^Vulnerability #\d+:\s+(GO-\d{4}-\d+)\s*$`)
	summaryLineRe = regexp.MustCompile(`^(Your code is affected by |This scan found |Use '-show )`)
)

func main() {
	var (
		govulncheckPath string
		ignorePath      string
		showIgnored     bool
	)
	flag.StringVar(&govulncheckPath, "govulncheck", "govulncheck", "path to govulncheck binary")
	flag.StringVar(&ignorePath, "ignore", ".govulnignore", "path to ignore file (one GO-... ID per line)")
	flag.BoolVar(&showIgnored, "show-ignored", false, "print traces even if all vulnerabilities are ignored")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"./..."}
	}

	code := run(os.Stdout, os.Stderr, govulncheckPath, ignorePath, showIgnored, args)
	os.Exit(code)
}

func run(out io.Writer, errOut io.Writer, govulncheckPath, ignorePath string, showIgnored bool, patterns []string) int {
	ignored, err := loadIDs(ignorePath)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: %v\n", err)
		return 2
	}

	jsonOut, rc, err := runGovulncheck(govulncheckPath, append([]string{"-format=json"}, patterns...)...)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: %v\n", err)
		return 2
	}
	if rc != 0 && rc != 3 {
		// govulncheck already printed its error details (we forward combined output below).
		_, _ = errOut.Write(jsonOut)
		return rc
	}

	found, err := parseOSVIDsFromJSON(bytes.NewReader(jsonOut))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: parsing govulncheck JSON: %v\n", err)
		return 2
	}

	unignored := unignoredIDs(found, ignored)
	if len(unignored) == 0 {
		if showIgnored && len(found) > 0 {
			// Print traces for all findings (since everything is ignored).
			tracesOut, tracesRC, err := runGovulncheck(govulncheckPath, append([]string{"-show=traces"}, patterns...)...)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: %v\n", err)
				return 2
			}
			if tracesRC != 0 && tracesRC != 3 {
				_, _ = errOut.Write(tracesOut)
				return tracesRC
			}
			anyPrinted, err := filterTraces(bytes.NewReader(tracesOut), out, set(found))
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: filtering traces: %v\n", err)
				return 2
			}
			if !anyPrinted {
				_, _ = fmt.Fprintln(errOut, "govulncheck_wrapper: no traces matched filter; printing raw govulncheck output")
				_, _ = out.Write(tracesOut)
			}
		}
		return 0
	}

	_, _ = fmt.Fprintf(errOut, "govulncheck: vulnerabilities not in %s: %s\n", ignorePath, strings.Join(unignored, ", "))
	_, _ = fmt.Fprintln(errOut, "govulncheck: unignored vulnerabilities found; rerunning with traces (filtered)...")

	tracesOut, tracesRC, err := runGovulncheck(govulncheckPath, append([]string{"-show=traces"}, patterns...)...)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: %v\n", err)
		return 2
	}
	if tracesRC != 0 && tracesRC != 3 {
		_, _ = errOut.Write(tracesOut)
		return tracesRC
	}
	anyPrinted, err := filterTraces(bytes.NewReader(tracesOut), out, set(unignored))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "govulncheck_wrapper: filtering traces: %v\n", err)
		return 2
	}
	if !anyPrinted {
		_, _ = fmt.Fprintln(errOut, "govulncheck_wrapper: no traces matched filter for unignored vulnerabilities; "+
			"govulncheck output format may have changed")
		_, _ = errOut.Write(tracesOut)
		return 2
	}
	return 1
}

func runGovulncheck(path string, args ...string) (combined []byte, rc int, err error) {
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.Command(path, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	combined = buf.Bytes()
	if runErr == nil {
		return combined, 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return combined, exitErr.ExitCode(), nil
	}
	return combined, 2, runErr
}

func parseOSVIDsFromJSON(r io.Reader) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	dec := json.NewDecoder(r)
	for {
		var msg struct {
			Finding *struct {
				OSV string `json:"osv"`
			} `json:"finding"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if msg.Finding != nil && msg.Finding.OSV != "" && !seen[msg.Finding.OSV] {
			seen[msg.Finding.OSV] = true
			out = append(out, msg.Finding.OSV)
		}
	}
	sort.Strings(out)
	return out, nil
}

func loadIDs(path string) (map[string]bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	out := make(map[string]bool)
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, nil
}

func unignoredIDs(found []string, ignored map[string]bool) []string {
	var out []string
	for _, id := range found {
		if !ignored[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func set(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func filterTraces(in io.Reader, out io.Writer, ids map[string]bool) (anyPrinted bool, err error) {
	sc := bufio.NewScanner(in)
	// Increase token size to accommodate large trace lines.
	sc.Buffer(make([]byte, 1024), 1024*1024)

	w := bufio.NewWriter(out)
	defer func() {
		flushErr := w.Flush()
		if flushErr != nil && err == nil {
			err = flushErr
		}
	}()

	var (
		block         []string
		include       bool
		printedHeader bool
	)

	flush := func() {
		if !include || len(block) == 0 {
			block = block[:0]
			return
		}
		if !printedHeader {
			_, _ = fmt.Fprintln(w, "=== Symbol Results ===")
			_, _ = fmt.Fprintln(w)
			printedHeader = true
		}
		for _, l := range block {
			_, _ = fmt.Fprintln(w, l)
		}
		_, _ = fmt.Fprintln(w)
		anyPrinted = true
		block = block[:0]
	}

	for sc.Scan() {
		line := sc.Text()
		if m := vulnHeaderRe.FindStringSubmatch(line); m != nil {
			flush()
			include = ids[m[1]]
			block = append(block, line)
			continue
		}
		if len(block) == 0 {
			continue
		}
		if summaryLineRe.MatchString(line) {
			flush()
			include = false
			block = block[:0]
			continue
		}
		block = append(block, line)
	}
	if err := sc.Err(); err != nil {
		return anyPrinted, err
	}
	flush()
	return anyPrinted, nil
}

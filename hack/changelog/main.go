package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

type commit struct {
	hash    string
	subject string
	body    string
}

type parsedCommit struct {
	hash        string
	rawSubject  string
	typ         string
	scope       string
	description string
	breaking    bool
}

var (
	reHeader = regexp.MustCompile(`^(?P<type>[a-z]+)(\((?P<scope>[^)]+)\))?(?P<bang>!)?: (?P<desc>.+)$`)
)

func main() {
	var outPath string
	var includeAllTags bool
	var tagPrefix string
	var repoURL string

	flag.StringVar(&outPath, "out", "CHANGELOG.md", "Output path for generated changelog")
	flag.BoolVar(&includeAllTags, "all-tags", false, "Generate changelog for all tags (default: only Unreleased since last tag)")
	flag.StringVar(&tagPrefix, "tag-prefix", "v", "Tag prefix for releases (default: v)")
	flag.StringVar(&repoURL, "repo", "https://github.com/dc-tec/openbao-operator", "Repository base URL for commit links")
	flag.Parse()

	// Resolve tags and commit ranges.
	tags, err := listTags(tagPrefix)
	if err != nil {
		fatalf("list tags: %v", err)
	}

	var buf bytes.Buffer
	buf.WriteString("# Changelog\n\n")
	buf.WriteString("This changelog is generated from git history using Conventional Commits.\n\n")

	if includeAllTags {
		// All tags, newest first, plus Unreleased.
		lastTag := ""
		if len(tags) > 0 {
			lastTag = tags[len(tags)-1]
		}
		if err := writeUnreleased(&buf, lastTag, repoURL); err != nil {
			fatalf("generate Unreleased: %v", err)
		}
		// Walk tags newest -> oldest.
		for i := len(tags) - 1; i >= 0; i-- {
			var from string
			if i > 0 {
				from = tags[i-1]
			}
			to := tags[i]
			if err := writeReleaseRange(&buf, from, to, repoURL); err != nil {
				fatalf("generate release %s: %v", to, err)
			}
		}
	} else {
		// Only Unreleased since last tag.
		lastTag := ""
		if len(tags) > 0 {
			lastTag = tags[len(tags)-1]
		}
		if err := writeUnreleased(&buf, lastTag, repoURL); err != nil {
			fatalf("generate Unreleased: %v", err)
		}
	}

	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		fatalf("write %s: %v", outPath, err)
	}
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func runGit(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func listTags(prefix string) ([]string, error) {
	// We use a stable sort by version string. For experimental releases we expect "vX.Y.Z".
	// If tags are absent, we just return an empty list.
	out, err := runGit("tag", "--list", prefix+"*")
	if err != nil {
		// If this isn't a git repo, surface clearly.
		return nil, err
	}
	var tags []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		t := strings.TrimSpace(sc.Text())
		if t == "" {
			continue
		}
		tags = append(tags, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Strings(tags)
	return tags, nil
}

func writeUnreleased(buf *bytes.Buffer, sinceTag string, repoURL string) error {
	buf.WriteString("## Unreleased\n\n")
	if sinceTag != "" {
		buf.WriteString(fmt.Sprintf("_Changes since `%s`._\n\n", sinceTag))
	}
	commits, err := commitsInRange(sinceTag, "HEAD")
	if err != nil {
		return err
	}
	return writeSectionsByScope(buf, commits, repoURL)
}

func writeReleaseRange(buf *bytes.Buffer, fromTag, toTag string, repoURL string) error {
	// Date from tag commit.
	date := ""
	if out, err := runGit("show", "-s", "--format=%cs", toTag); err == nil {
		date = strings.TrimSpace(string(out))
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	buf.WriteString(fmt.Sprintf("## %s (%s)\n\n", toTag, date))
	commits, err := commitsInRange(fromTag, toTag)
	if err != nil {
		return err
	}
	return writeSectionsByScope(buf, commits, repoURL)
}

func commitsInRange(fromRef, toRef string) ([]commit, error) {
	var rangeArg string
	switch {
	case fromRef == "" && (toRef == "" || toRef == "HEAD"):
		rangeArg = "HEAD"
	case fromRef == "":
		rangeArg = toRef
	default:
		rangeArg = fmt.Sprintf("%s..%s", fromRef, toRef)
	}

	// Use NUL separators for safety.
	out, err := runGit("log", "--no-merges", "--format=%H%x00%s%x00%b%x00", rangeArg)
	if err != nil {
		return nil, err
	}

	parts := bytes.Split(out, []byte{0})
	// Triples: hash, subject, body, ... last element may be empty.
	var commits []commit
	for i := 0; i+2 < len(parts); i += 3 {
		h := strings.TrimSpace(string(parts[i]))
		s := strings.TrimSpace(string(parts[i+1]))
		b := strings.TrimSpace(string(parts[i+2]))
		if h == "" && s == "" && b == "" {
			continue
		}
		commits = append(commits, commit{hash: h, subject: s, body: b})
	}
	return commits, nil
}

func parseConventional(c commit) parsedCommit {
	pc := parsedCommit{
		hash:       c.hash,
		rawSubject: c.subject,
	}

	m := reHeader.FindStringSubmatch(c.subject)
	if m == nil {
		// Keep as "other".
		pc.typ = "other"
		pc.description = c.subject
		return pc
	}

	// Map named groups.
	idx := map[string]int{}
	for i, name := range reHeader.SubexpNames() {
		if name == "" {
			continue
		}
		idx[name] = i
	}
	pc.typ = m[idx["type"]]
	pc.scope = m[idx["scope"]]
	pc.description = m[idx["desc"]]
	pc.breaking = m[idx["bang"]] == "!"
	if strings.Contains(c.body, "BREAKING CHANGE:") || strings.Contains(c.body, "BREAKING-CHANGE:") {
		pc.breaking = true
	}
	return pc
}

func commitURL(repoURL, hash string) string {
	return strings.TrimRight(repoURL, "/") + "/commit/" + hash
}

func writeSectionsByScope(buf *bytes.Buffer, commits []commit, repoURL string) error {
	parsed := make([]parsedCommit, 0, len(commits))
	for _, c := range commits {
		parsed = append(parsed, parseConventional(c))
	}

	// Group by scope (primary). Within each scope, group by type (secondary).
	type typeGroup struct {
		title string
		order int
	}
	typeGroups := map[string]typeGroup{
		"breaking": {title: "Breaking Changes", order: 0},
		"feat":     {title: "Features", order: 1},
		"fix":      {title: "Fixes", order: 2},
		"perf":     {title: "Performance", order: 3},
		"refactor": {title: "Refactors", order: 4},
		"docs":     {title: "Documentation", order: 5},
		"test":     {title: "Tests", order: 6},
		"build":    {title: "Build", order: 7},
		"ci":       {title: "CI", order: 8},
		"chore":    {title: "Chores", order: 9},
		"other":    {title: "Other", order: 10},
	}

	byScope := map[string]map[string][]parsedCommit{} // scope -> typeKey -> commits
	scopeOrder := map[string]int{}
	for _, c := range parsed {
		scope := c.scope
		if scope == "" {
			scope = "misc"
		}
		typeKey := c.typ
		if c.breaking {
			typeKey = "breaking"
		}
		if _, ok := typeGroups[typeKey]; !ok {
			typeKey = "other"
		}
		if _, ok := byScope[scope]; !ok {
			byScope[scope] = map[string][]parsedCommit{}
		}
		byScope[scope][typeKey] = append(byScope[scope][typeKey], c)
		scopeOrder[scope]++
	}

	if len(byScope) == 0 {
		buf.WriteString("_No changes._\n\n")
		return nil
	}

	// Deterministic scope order: most commits first, then alphabetical.
	scopes := make([]string, 0, len(byScope))
	for s := range byScope {
		scopes = append(scopes, s)
	}
	sort.Slice(scopes, func(i, j int) bool {
		ci, cj := scopeOrder[scopes[i]], scopeOrder[scopes[j]]
		if ci != cj {
			return ci > cj
		}
		return scopes[i] < scopes[j]
	})

	for _, scope := range scopes {
		buf.WriteString("### " + scope + "\n\n")

		// Type groups in defined order.
		typeKeys := make([]string, 0, len(typeGroups))
		for k := range typeGroups {
			typeKeys = append(typeKeys, k)
		}
		sort.Slice(typeKeys, func(i, j int) bool { return typeGroups[typeKeys[i]].order < typeGroups[typeKeys[j]].order })

		wroteAny := false
		for _, typeKey := range typeKeys {
			list := byScope[scope][typeKey]
			if len(list) == 0 {
				continue
			}
			wroteAny = true
			buf.WriteString("#### " + typeGroups[typeKey].title + "\n\n")
			for _, c := range list {
				shortHash := c.hash
				if len(shortHash) > 7 {
					shortHash = shortHash[:7]
				}
				link := commitURL(repoURL, c.hash)
				buf.WriteString(fmt.Sprintf("- %s ([`%s`](%s))\n", c.description, shortHash, link))
			}
			buf.WriteString("\n")
		}
		if !wroteAny {
			buf.WriteString("_No changes._\n\n")
		}
	}

	return nil
}



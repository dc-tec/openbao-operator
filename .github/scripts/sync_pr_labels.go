package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultLabelColor = "ededed"

type labelRule struct {
	ChangedFiles []globRule `yaml:"changed-files"`
}

type globRule struct {
	Any []string `yaml:"any-glob-to-any-file"`
	All []string `yaml:"all-globs-to-any-file"`
}

type sizeConfig struct {
	Thresholds []sizeThreshold `yaml:"thresholds"`
	Ignore     []string        `yaml:"ignore"`
	Comment    sizeComment     `yaml:"comment"`
}

type sizeThreshold struct {
	Label string `yaml:"label"`
	Max   *int   `yaml:"max"`
}

type sizeComment struct {
	Marker string `yaml:"marker"`
	Body   string `yaml:"body"`
}

type pullRequestFile struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

type issueLabel struct {
	Name string `json:"name"`
}

type issueComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

type repoLabel struct {
	Name string `json:"name"`
}

type githubClient struct {
	repo   string
	dryRun bool
}

type labelSyncer struct {
	client           *githubClient
	prNumber         string
	labelConfig      map[string][]labelRule
	sizeConfig       sizeConfig
	repoLabelsCached []string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repo := os.Getenv("REPO")
	if repo == "" {
		return errors.New("REPO is required")
	}
	prNumber := os.Getenv("PR_NUMBER")
	if prNumber == "" {
		return errors.New("PR_NUMBER is required")
	}

	labelConfigPath := envOrDefault("LABELER_CONFIG_PATH", ".github/labeler.yml")
	sizeConfigPath := envOrDefault("SIZE_CONFIG_PATH", ".github/size-labeler.yml")

	labelConfig, err := loadLabelConfig(labelConfigPath)
	if err != nil {
		return fmt.Errorf("load label config: %w", err)
	}

	szConfig, err := loadSizeConfig(sizeConfigPath)
	if err != nil {
		return fmt.Errorf("load size config: %w", err)
	}

	syncer := &labelSyncer{
		client: &githubClient{
			repo:   repo,
			dryRun: os.Getenv("DRY_RUN") == "1",
		},
		prNumber:    prNumber,
		labelConfig: labelConfig,
		sizeConfig:  szConfig,
	}

	return syncer.sync()
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadLabelConfig(path string) (map[string][]labelRule, error) {
	var cfg map[string][]labelRule

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadSizeConfig(path string) (sizeConfig, error) {
	var cfg sizeConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (s *labelSyncer) sync() error {
	files, err := s.pullRequestFiles()
	if err != nil {
		return err
	}

	currentLabels, err := s.issueLabels()
	if err != nil {
		return err
	}

	desiredContentLabels := s.matchingContentLabels(files)
	if err := s.syncManagedLabels(currentLabels, desiredContentLabels, mapsKeysSorted(s.labelConfig)); err != nil {
		return err
	}

	sizeLabel := s.calculateSizeLabel(files)
	currentLabels, err = s.issueLabels()
	if err != nil {
		return err
	}
	if err := s.syncManagedLabels(currentLabels, []string{sizeLabel}, s.sizeLabels()); err != nil {
		return err
	}

	return s.syncSizeComment(sizeLabel)
}

func (s *labelSyncer) pullRequestFiles() ([]pullRequestFile, error) {
	var files []pullRequestFile
	if err := s.client.getJSONPaginated(fmt.Sprintf("/repos/%s/pulls/%s/files", s.client.repo, s.prNumber), &files); err != nil {
		return nil, fmt.Errorf("list pull request files: %w", err)
	}
	return files, nil
}

func (s *labelSyncer) issueLabels() ([]string, error) {
	var labels []issueLabel
	if err := s.client.getJSONPaginated(fmt.Sprintf("/repos/%s/issues/%s/labels", s.client.repo, s.prNumber), &labels); err != nil {
		return nil, fmt.Errorf("list issue labels: %w", err)
	}

	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names, nil
}

func (s *labelSyncer) issueComments() ([]issueComment, error) {
	var comments []issueComment
	if err := s.client.getJSONPaginated(fmt.Sprintf("/repos/%s/issues/%s/comments", s.client.repo, s.prNumber), &comments); err != nil {
		return nil, fmt.Errorf("list issue comments: %w", err)
	}
	return comments, nil
}

func (s *labelSyncer) repoLabels() ([]string, error) {
	if s.repoLabelsCached != nil {
		return append([]string(nil), s.repoLabelsCached...), nil
	}

	var labels []repoLabel
	if err := s.client.getJSONPaginated(fmt.Sprintf("/repos/%s/labels", s.client.repo), &labels); err != nil {
		return nil, fmt.Errorf("list repo labels: %w", err)
	}

	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	s.repoLabelsCached = names
	return append([]string(nil), names...), nil
}

func (s *labelSyncer) matchingContentLabels(files []pullRequestFile) []string {
	matches := make([]string, 0)

	for label, rules := range s.labelConfig {
		for _, rule := range rules {
			if ruleMatchesFiles(rule, files) {
				matches = append(matches, label)
				break
			}
		}
	}

	slices.Sort(matches)
	return matches
}

func ruleMatchesFiles(rule labelRule, files []pullRequestFile) bool {
	for _, changedFiles := range rule.ChangedFiles {
		if globRuleMatchesFiles(changedFiles, files) {
			return true
		}
	}

	return false
}

func globRuleMatchesFiles(rule globRule, files []pullRequestFile) bool {
	anyMatch := len(rule.Any) == 0
	for _, pattern := range rule.Any {
		if anyFileMatches(pattern, files) {
			anyMatch = true
			break
		}
	}

	if !anyMatch {
		return false
	}

	for _, pattern := range rule.All {
		if !anyFileMatches(pattern, files) {
			return false
		}
	}

	return true
}

func anyFileMatches(pattern string, files []pullRequestFile) bool {
	for _, file := range files {
		if pathMatches(pattern, file.Filename) {
			return true
		}
	}
	return false
}

func pathMatches(pattern, path string) bool {
	path = filepath.ToSlash(path)
	pattern = filepath.ToSlash(pattern)

	re, err := globToRegexp(pattern)
	if err != nil {
		return false
	}

	return re.MatchString(path)
}

func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")

	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i += 2
				if i < len(pattern) && pattern[i] == '/' {
					b.WriteString("(?:.*/)?")
					i++
				} else {
					b.WriteString(".*")
				}
				continue
			}
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
		i++
	}

	b.WriteString("$")
	return regexp.Compile(b.String())
}

func (s *labelSyncer) sizeLabels() []string {
	labels := make([]string, 0, len(s.sizeConfig.Thresholds))
	for _, threshold := range s.sizeConfig.Thresholds {
		labels = append(labels, threshold.Label)
	}
	return labels
}

func (s *labelSyncer) calculateSizeLabel(files []pullRequestFile) string {
	total := 0
	for _, file := range files {
		if s.ignoredForSize(file.Filename) {
			continue
		}
		total += file.Additions + file.Deletions
	}

	for _, threshold := range s.sizeConfig.Thresholds {
		if threshold.Max == nil || total <= *threshold.Max {
			return threshold.Label
		}
	}

	return ""
}

func (s *labelSyncer) ignoredForSize(path string) bool {
	for _, pattern := range s.sizeConfig.Ignore {
		if pathMatches(pattern, path) {
			return true
		}
	}
	return false
}

func (s *labelSyncer) syncManagedLabels(currentLabels, desiredLabels, managedLabels []string) error {
	desired := uniqueSorted(filterEmpty(desiredLabels))
	currentManaged := intersect(currentLabels, managedLabels)

	labelsToAdd := difference(desired, currentLabels)
	labelsToRemove := difference(currentManaged, desired)

	if err := s.ensureRepoLabels(labelsToAdd); err != nil {
		return err
	}
	if len(labelsToAdd) > 0 {
		if err := s.client.postJSON(fmt.Sprintf("/repos/%s/issues/%s/labels", s.client.repo, s.prNumber), map[string]any{
			"labels": labelsToAdd,
		}); err != nil {
			return fmt.Errorf("add issue labels: %w", err)
		}
	}

	for _, label := range labelsToRemove {
		if err := s.client.delete(fmt.Sprintf("/repos/%s/issues/%s/labels/%s", s.client.repo, s.prNumber, escapePathSegment(label))); err != nil {
			return fmt.Errorf("remove issue label %q: %w", label, err)
		}
	}

	return nil
}

func (s *labelSyncer) ensureRepoLabels(labels []string) error {
	if len(labels) == 0 {
		return nil
	}

	existing, err := s.repoLabels()
	if err != nil {
		return err
	}

	for _, label := range labels {
		if slices.Contains(existing, label) {
			continue
		}

		if err := s.client.postJSON(fmt.Sprintf("/repos/%s/labels", s.client.repo), map[string]any{
			"name":        label,
			"color":       defaultLabelColor,
			"description": "",
		}); err != nil {
			return fmt.Errorf("create repo label %q: %w", label, err)
		}
		existing = append(existing, label)
	}

	s.repoLabelsCached = uniqueSorted(existing)
	return nil
}

func (s *labelSyncer) syncSizeComment(sizeLabel string) error {
	marker := strings.TrimSpace(s.sizeConfig.Comment.Marker)
	body := strings.TrimSpace(s.sizeConfig.Comment.Body)
	if marker == "" || body == "" {
		return nil
	}

	comments, err := s.issueComments()
	if err != nil {
		return err
	}

	var existing *issueComment
	for i := range comments {
		if strings.Contains(comments[i].Body, marker) {
			existing = &comments[i]
			break
		}
	}

	if sizeLabel == "size/XL" {
		desired := strings.TrimSpace(marker + "\n" + body)
		if existing == nil {
			return s.client.postJSON(fmt.Sprintf("/repos/%s/issues/%s/comments", s.client.repo, s.prNumber), map[string]any{
				"body": desired,
			})
		}
		if existing.Body != desired {
			return s.client.patchJSON(fmt.Sprintf("/repos/%s/issues/comments/%d", s.client.repo, existing.ID), map[string]any{
				"body": desired,
			})
		}
		return nil
	}

	if existing != nil {
		return s.client.delete(fmt.Sprintf("/repos/%s/issues/comments/%d", s.client.repo, existing.ID))
	}

	return nil
}

func (c *githubClient) getJSONPaginated(path string, out any) error {
	stdout, err := c.runAPI(path, "GET", nil, true)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(stdout)) == 0 {
		return nil
	}
	return json.Unmarshal(stdout, out)
}

func (c *githubClient) postJSON(path string, payload any) error {
	_, err := c.runAPI(path, "POST", payload, false)
	return err
}

func (c *githubClient) patchJSON(path string, payload any) error {
	_, err := c.runAPI(path, "PATCH", payload, false)
	return err
}

func (c *githubClient) delete(path string) error {
	_, err := c.runAPI(path, "DELETE", nil, false)
	return err
}

func (c *githubClient) runAPI(path, method string, payload any, paginate bool) ([]byte, error) {
	if c.dryRun && method != "GET" {
		body := ""
		if payload != nil {
			data, _ := json.Marshal(payload)
			body = " " + string(data)
		}
		fmt.Fprintf(os.Stderr, "DRY_RUN %s %s%s\n", method, path, body)
		return nil, nil
	}

	args := []string{"api", "-H", "Accept: application/vnd.github+json"}
	if paginate {
		args = append(args, "--paginate")
	}
	if method != "GET" {
		args = append(args, "--method", method)
	}

	var inputFile *os.File
	if payload != nil {
		file, err := os.CreateTemp("", "gh-api-payload-*.json")
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			file.Close()
			os.Remove(file.Name())
			return nil, err
		}
		if _, err := file.Write(encoded); err != nil {
			file.Close()
			os.Remove(file.Name())
			return nil, err
		}
		if err := file.Close(); err != nil {
			os.Remove(file.Name())
			return nil, err
		}
		inputFile = file
		args = append(args, "--input", file.Name())
	}

	args = append(args, path)
	cmd := exec.Command("gh", args...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if inputFile != nil {
		_ = os.Remove(inputFile.Name())
	}
	if err != nil {
		return nil, fmt.Errorf("gh api %s %s failed: %w: %s", method, path, err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func mapsKeysSorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	slices.Sort(values)
	result := values[:0]
	var previous string
	for i, value := range values {
		if i == 0 || value != previous {
			result = append(result, value)
			previous = value
		}
	}

	return append([]string(nil), result...)
}

func difference(left, right []string) []string {
	result := make([]string, 0)
	for _, value := range left {
		if !slices.Contains(right, value) {
			result = append(result, value)
		}
	}
	return result
}

func intersect(left, right []string) []string {
	result := make([]string, 0)
	for _, value := range left {
		if slices.Contains(right, value) {
			result = append(result, value)
		}
	}
	return result
}

func filterEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func escapePathSegment(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '~' {
			builder.WriteRune(r)
			continue
		}

		for _, b := range []byte(string(r)) {
			builder.WriteString("%")
			builder.WriteString(strings.ToUpper(strconv.FormatInt(int64(b), 16)))
		}
	}
	return builder.String()
}

func fetchRepoFile(repo, ref, path, destination string) error {
	path = strings.TrimPrefix(path, "/")
	stdout, stderr, err := runGHCommand(
		"api",
		"-H", "Accept: application/vnd.github+json",
		fmt.Sprintf("/repos/%s/contents/%s?ref=%s", repo, path, ref),
	)
	if err != nil {
		return fmt.Errorf("fetch %s: %w: %s", path, err, strings.TrimSpace(stderr))
	}

	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		return fmt.Errorf("decode %s payload: %w", path, err)
	}

	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(payload.Content, "\n", ""))
	if err != nil {
		return fmt.Errorf("decode %s content: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	return os.WriteFile(destination, content, 0o644)
}

func runGHCommand(args ...string) (string, string, error) {
	cmd := exec.Command("gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

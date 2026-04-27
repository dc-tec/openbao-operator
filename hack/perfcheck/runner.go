package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func runCapture(opts options) error {
	scenarios, err := selectedScenarios(opts)
	if err != nil {
		return err
	}

	baseline := baselineDocument{
		Version:     "v1",
		CapturedAt:  time.Now().UTC(),
		NodeImage:   opts.NodeImage,
		RunsPerCase: opts.Runs,
		Multipliers: multiplierConfig{
			P95: opts.P95Multiplier,
			Max: opts.MaxMultiplier,
		},
		MetricSchema: append([]string(nil), metricKeys...),
		Scenarios:    make(map[string]scenarioBaseline, len(scenarios)),
	}

	for _, scenario := range scenarios {
		base := scenarioBaseline{
			LabelFilter:    scenario.LabelFilter,
			MetricPolicies: scenario.MetricPolicies,
			Runs:           make([]runResult, 0, opts.Runs),
			MaxMetrics:     make(map[string]float64, len(metricKeys)),
		}
		for _, key := range metricKeys {
			base.MaxMetrics[key] = 0
		}

		for run := 1; run <= opts.Runs; run++ {
			res, err := executeScenarioRun(opts, scenario, run)
			if err != nil {
				return err
			}
			base.Runs = append(base.Runs, res)
			for _, key := range metricKeys {
				if res.Metrics[key] > base.MaxMetrics[key] {
					base.MaxMetrics[key] = res.Metrics[key]
				}
			}
		}

		baseline.Scenarios[scenario.Name] = base
	}

	thresholds := buildThresholds(baseline)
	if err := writeBaseline(opts.BaselinePath, baseline); err != nil {
		return err
	}
	if err := writeThresholds(opts.ThresholdsPath, thresholds); err != nil {
		return err
	}

	fmt.Printf("wrote baseline: %s\n", opts.BaselinePath)
	fmt.Printf("wrote thresholds: %s\n", opts.ThresholdsPath)
	return nil
}

func runVerify(opts options) error {
	scenarios, err := selectedScenarios(opts)
	if err != nil {
		return err
	}

	thresholds, err := readThresholds(opts.ThresholdsInput)
	if err != nil {
		return err
	}

	var findings []string
	var warnings []string
	for _, scenario := range scenarios {
		th, ok := thresholds.Scenarios[scenario.Name]
		if !ok {
			return fmt.Errorf("thresholds missing scenario %q", scenario.Name)
		}
		if err := validateScenarioThresholds(th, scenario); err != nil {
			return err
		}
		th = applyScenarioPolicy(th, scenario)
		res, runErr := executeScenarioRun(opts, scenario, 1)
		if runErr != nil {
			return runErr
		}
		scenarioResult := compareScenarioMetricsDetailed(scenario.Name, res.Metrics, th)
		findings = append(findings, scenarioResult.Findings...)
		warnings = append(warnings, scenarioResult.Warnings...)

		fmt.Printf("scenario=%s metrics=%s\n", scenario.Name, formatMetrics(res.Metrics))
	}

	if len(warnings) > 0 {
		sort.Strings(warnings)
		fmt.Println("performance diagnostic warnings:")
		for _, w := range warnings {
			fmt.Printf("- %s\n", w)
		}
	}

	if len(findings) > 0 {
		sort.Strings(findings)
		fmt.Println("performance regression findings:")
		for _, f := range findings {
			fmt.Printf("- %s\n", f)
		}
		return fmt.Errorf("performance thresholds violated (%d findings)", len(findings))
	}

	fmt.Println("performance verification passed")
	return nil
}

func selectedScenarios(opts options) ([]scenarioSpec, error) {
	manifest, err := loadScenarioManifest(opts.ScenarioPath)
	if err != nil {
		return nil, err
	}
	if len(opts.ScenarioNames) == 0 {
		return append([]scenarioSpec(nil), manifest.Scenarios...), nil
	}

	byName := scenarioMap(manifest.Scenarios)
	out := make([]scenarioSpec, 0, len(opts.ScenarioNames))
	for _, name := range opts.ScenarioNames {
		spec, ok := byName[name]
		if !ok {
			available := strings.Join(sortedScenarioNames(manifest.Scenarios), ", ")
			return nil, fmt.Errorf("unknown scenario %q (available: %s)", name, available)
		}
		out = append(out, spec)
	}
	return out, nil
}

func executeScenarioRun(opts options, scenario scenarioSpec, runIndex int) (runResult, error) {
	cluster := clusterNameForScenario(scenario.Name, runIndex)
	started := time.Now().UTC()
	fmt.Printf("running scenario=%s run=%d cluster=%s\n", scenario.Name, runIndex, cluster)

	if err := setupKindCluster(opts, cluster); err != nil {
		return runResult{}, err
	}

	shouldCleanup := true
	cleanup := func() {
		if err := cleanupKindCluster(opts, cluster); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cleanup failed for cluster %s: %v\n", cluster, err)
		}
	}
	defer func() {
		if shouldCleanup {
			cleanup()
		}
	}()

	beforeCtx, cancelBefore := context.WithTimeout(context.Background(), 90*time.Second)
	before, beforePresent, err := scrapeMetricsSnapshot(beforeCtx, opts, cluster, true)
	cancelBefore()
	if err != nil {
		if !beforePresent {
			before = emptySnapshot()
		} else {
			return runResult{}, err
		}
	}

	scenarioCtx, cancelScenario := context.WithTimeout(context.Background(), opts.ScenarioTimeout)
	err = runScenarioTests(scenarioCtx, opts, cluster, scenario)
	cancelScenario()
	if err != nil {
		if opts.KeepOnFailure {
			shouldCleanup = false
			fmt.Fprintf(os.Stderr, "keeping cluster %s for debugging\n", cluster)
		}
		return runResult{}, err
	}

	afterCtx, cancelAfter := context.WithTimeout(context.Background(), 2*time.Minute)
	after, _, err := scrapeMetricsSnapshot(afterCtx, opts, cluster, false)
	cancelAfter()
	if err != nil {
		return runResult{}, err
	}

	metrics := computeScenarioMetrics(before, after)
	return runResult{
		Scenario:      scenario.Name,
		LabelFilter:   scenario.LabelFilter,
		Run:           runIndex,
		Cluster:       cluster,
		StartedAt:     started,
		Duration:      time.Since(started),
		Metrics:       metrics,
		BeforePresent: beforePresent,
	}, nil
}

func clusterNameForScenario(name string, run int) string {
	replacer := strings.NewReplacer("_", "-", "/", "-", " ", "-", ".", "-")
	slug := replacer.Replace(strings.ToLower(strings.TrimSpace(name)))
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "scenario"
	}
	cluster := fmt.Sprintf("perf-%s-%d", slug, run)
	if len(cluster) > 60 {
		cluster = cluster[:60]
	}
	return cluster
}

func setupKindCluster(opts options, cluster string) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.ClusterTimeout)
	defer cancel()

	env := map[string]string{
		"KIND":            opts.KindBin,
		"KIND_CLUSTER":    cluster,
		"KIND_NODE_IMAGE": opts.NodeImage,
	}
	_, err := runCommand(ctx, env, opts.MakeBin, "setup-test-e2e")
	if err != nil {
		return fmt.Errorf("setup kind cluster %q: %w", cluster, err)
	}
	return nil
}

func cleanupKindCluster(opts options, cluster string) error {
	ctx, cancel := context.WithTimeout(context.Background(), opts.CleanupTimeout)
	defer cancel()
	env := map[string]string{
		"KIND":         opts.KindBin,
		"KIND_CLUSTER": cluster,
	}
	_, err := runCommand(ctx, env, opts.MakeBin, "cleanup-test-e2e")
	if err != nil {
		return fmt.Errorf("cleanup kind cluster %q: %w", cluster, err)
	}
	return nil
}

func runScenarioTests(ctx context.Context, opts options, cluster string, scenario scenarioSpec) error {
	env := map[string]string{
		"KIND":               opts.KindBin,
		"KIND_CLUSTER":       cluster,
		"E2E_LABEL_FILTER":   scenario.LabelFilter,
		"E2E_PARALLEL_NODES": "1",
		"E2E_SKIP_CLEANUP":   "true",
		"E2E_TIMEOUT":        opts.ScenarioTimeout.String(),
	}
	_, err := runCommand(ctx, env, opts.MakeBin, "test-e2e-ci")
	if err != nil {
		return fmt.Errorf("run e2e scenario %q: %w", scenario.Name, err)
	}
	return nil
}

func runCommand(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	// This helper runs only repo-managed tools selected by validated perfcheck options.
	// nosemgrep
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for key := range env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, env[key]))
		}
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, output.String())
	}
	return output.String(), nil
}

func emptySnapshot() metricsSnapshot {
	return metricsSnapshot{
		Counters:   make(map[string]float64),
		GaugeMax:   make(map[string]float64),
		Histograms: make(map[string]map[float64]float64),
	}
}

func scrapeMetricsSnapshot(
	ctx context.Context,
	opts options,
	cluster string,
	allowMissing bool,
) (metricsSnapshot, bool, error) {
	clusterContext := fmt.Sprintf("kind-%s", cluster)
	if err := ensureMetricsRoleBinding(ctx, opts, clusterContext); err != nil {
		if allowMissing && looksLikeMissingResource(err) {
			return emptySnapshot(), false, nil
		}
		if allowMissing {
			return emptySnapshot(), false, nil
		}
		return metricsSnapshot{}, false, err
	}

	token, err := createMetricsToken(ctx, opts, clusterContext)
	if err != nil {
		if allowMissing {
			return emptySnapshot(), false, nil
		}
		return metricsSnapshot{}, false, err
	}

	metricsText, err := fetchMetricsViaPortForward(ctx, opts, clusterContext, token)
	if err != nil {
		if allowMissing && looksLikeMissingResource(err) {
			return emptySnapshot(), false, nil
		}
		if allowMissing {
			return emptySnapshot(), false, nil
		}
		return metricsSnapshot{}, false, err
	}

	snapshot, err := parseMetricsSnapshot(metricsText)
	if err != nil {
		return metricsSnapshot{}, true, err
	}
	return snapshot, true, nil
}

func ensureMetricsRoleBinding(ctx context.Context, opts options, clusterContext string) error {
	roleNames := []string{"openbao-operator-metrics-reader", "metrics-reader"}
	var lastErr error
	for _, role := range roleNames {
		_, err := runCommand(ctx, nil,
			"kubectl",
			"--context", clusterContext,
			"create", "clusterrolebinding", opts.BindingName,
			fmt.Sprintf("--clusterrole=%s", role),
			fmt.Sprintf("--serviceaccount=%s:%s", opts.OperatorNS, opts.ServiceAccount),
		)
		if err == nil {
			return nil
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "alreadyexists") || strings.Contains(errStr, "already exists") {
			return nil
		}
		lastErr = err
		if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
			continue
		}
	}
	if lastErr != nil {
		return fmt.Errorf("ensure metrics role binding: %w", lastErr)
	}
	return fmt.Errorf("ensure metrics role binding: no role candidates succeeded")
}

func createMetricsToken(ctx context.Context, opts options, clusterContext string) (string, error) {
	deadline := time.Now().Add(45 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		out, err := runCommand(ctx, nil,
			"kubectl",
			"--context", clusterContext,
			"create", "token", opts.ServiceAccount,
			"-n", opts.OperatorNS,
			"--duration=1h",
		)
		if err == nil {
			token := strings.TrimSpace(out)
			if token != "" {
				return token, nil
			}
			lastErr = fmt.Errorf("token response was empty")
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out creating metrics token")
	}
	return "", fmt.Errorf("create metrics token: %w", lastErr)
}

func fetchMetricsViaPortForward(ctx context.Context, opts options, clusterContext, token string) (string, error) {
	port, err := findFreeLocalPort()
	if err != nil {
		return "", err
	}

	serviceRef := fmt.Sprintf("service/%s", opts.MetricsService)
	portArg := fmt.Sprintf("%d:8443", port)
	cmd := exec.CommandContext(ctx,
		"kubectl",
		"--context", clusterContext,
		"port-forward",
		"--namespace", opts.OperatorNS,
		serviceRef,
		portArg,
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start port-forward: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	if err := waitForLocalPort(ctx, port, 20*time.Second); err != nil {
		return "", err
	}

	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", port)
	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			//nolint:gosec // metrics are fetched via localhost port-forward in test infrastructure.
			TLSClientConfig: &tls.Config{ // nosemgrep
				MinVersion:         tls.VersionTLS13,
				InsecureSkipVerify: true, // nosemgrep
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch metrics: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read metrics body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("metrics endpoint status %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

func findFreeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen for free local port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener addr type %T", listener.Addr())
	}
	return addr.Port, nil
}

func waitForLocalPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for local port %d", port)
}

func looksLikeMissingResource(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		return true
	}
	if strings.Contains(errStr, "connection refused") {
		return true
	}
	if strings.Contains(errStr, "serviceaccount") && strings.Contains(errStr, "not found") {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func formatMetrics(metrics map[string]float64) string {
	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%.3f", key, metrics[key]))
	}
	return strings.Join(parts, " ")
}

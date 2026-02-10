package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "capture":
		opts, err := parseCaptureFlags(os.Args[2:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				os.Exit(0)
			}
			exitWithError(err)
		}
		if err := runCapture(opts); err != nil {
			exitWithError(err)
		}
	case "verify":
		opts, err := parseVerifyFlags(os.Args[2:])
		if err != nil {
			if errors.Is(err, flag.ErrHelp) {
				os.Exit(0)
			}
			exitWithError(err)
		}
		if err := runVerify(opts); err != nil {
			exitWithError(err)
		}
	default:
		printUsage()
		exitWithError(fmt.Errorf("unknown subcommand %q", subcommand))
	}
}

func parseCaptureFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		runs            = fs.Int("runs", 5, "number of runs per scenario")
		scenarios       = fs.String("scenarios", "all", "comma-separated scenarios (all|lifecycle|backup-restore|rolling-upgrade)")
		nodeImage       = fs.String("node-image", "kindest/node:v1.34.3", "kind node image")
		kindBin         = fs.String("kind", "kind", "path to kind binary")
		makeBin         = fs.String("make", "make", "path to make binary")
		baselinePath    = fs.String("baseline-out", "hack/perf/baseline/kind-v1.34.3-baseline.json", "output path for baseline JSON")
		thresholdsPath  = fs.String("thresholds-out", "hack/perf/thresholds/kind-v1.34.3.yaml", "output path for thresholds YAML")
		scenarioTimeout = fs.Duration("scenario-timeout", 90*time.Minute, "per-scenario timeout")
		clusterTimeout  = fs.Duration("cluster-timeout", 20*time.Minute, "kind setup timeout")
		cleanupTimeout  = fs.Duration("cleanup-timeout", 10*time.Minute, "kind cleanup timeout")
		keepOnFailure   = fs.Bool("keep-on-failure", false, "keep kind clusters if a scenario run fails")
		p95Mult         = fs.Float64("p95-multiplier", 1.25, "multiplier applied to p95 metrics")
		maxMult         = fs.Float64("max-multiplier", 1.40, "multiplier applied to max/churn metrics")
	)

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if *runs <= 0 {
		return options{}, errors.New("runs must be > 0")
	}
	if *p95Mult <= 0 || *maxMult <= 0 {
		return options{}, errors.New("multipliers must be > 0")
	}
	selected, err := parseScenarioSelection(*scenarios)
	if err != nil {
		return options{}, err
	}

	return baseOptions(*nodeImage, *kindBin, *makeBin, *scenarioTimeout, *clusterTimeout, *cleanupTimeout, *keepOnFailure).
		withCapture(*runs, selected, *baselinePath, *thresholdsPath, *p95Mult, *maxMult), nil
}

func parseVerifyFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		scenarios       = fs.String("scenarios", "all", "comma-separated scenarios (all|lifecycle|backup-restore|rolling-upgrade)")
		nodeImage       = fs.String("node-image", "kindest/node:v1.34.3", "kind node image")
		kindBin         = fs.String("kind", "kind", "path to kind binary")
		makeBin         = fs.String("make", "make", "path to make binary")
		thresholdsInput = fs.String("thresholds", "hack/perf/thresholds/kind-v1.34.3.yaml", "input thresholds YAML path")
		scenarioTimeout = fs.Duration("scenario-timeout", 90*time.Minute, "per-scenario timeout")
		clusterTimeout  = fs.Duration("cluster-timeout", 20*time.Minute, "kind setup timeout")
		cleanupTimeout  = fs.Duration("cleanup-timeout", 10*time.Minute, "kind cleanup timeout")
		keepOnFailure   = fs.Bool("keep-on-failure", false, "keep kind clusters if a scenario run fails")
	)

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	selected, err := parseScenarioSelection(*scenarios)
	if err != nil {
		return options{}, err
	}

	base := baseOptions(*nodeImage, *kindBin, *makeBin, *scenarioTimeout, *clusterTimeout, *cleanupTimeout, *keepOnFailure)
	base.Mode = "verify"
	base.ScenarioNames = selected
	base.ThresholdsInput = *thresholdsInput
	return base, nil
}

func baseOptions(nodeImage, kindBin, makeBin string, scenarioTimeout, clusterTimeout, cleanupTimeout time.Duration, keepOnFailure bool) options {
	return options{
		Mode:            "",
		NodeImage:       nodeImage,
		KindBin:         kindBin,
		MakeBin:         makeBin,
		ScenarioTimeout: scenarioTimeout,
		ClusterTimeout:  clusterTimeout,
		CleanupTimeout:  cleanupTimeout,
		KeepOnFailure:   keepOnFailure,
		OperatorNS:      "openbao-operator-system",
		MetricsService:  "openbao-operator-controller-metrics-service",
		ServiceAccount:  "openbao-operator-controller",
		BindingName:     "openbao-operator-metrics-binding",
	}
}

func (o options) withCapture(runs int, scenarios []string, baselinePath, thresholdsPath string, p95Mult, maxMult float64) options {
	o.Mode = "capture"
	o.Runs = runs
	o.ScenarioNames = scenarios
	o.BaselinePath = baselinePath
	o.ThresholdsPath = thresholdsPath
	o.P95Multiplier = p95Mult
	o.MaxMultiplier = maxMult
	return o
}

func parseScenarioSelection(input string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || trimmed == "all" {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if name == "all" {
			return nil, nil
		}
		if _, ok := scenarioByName[name]; !ok {
			return nil, fmt.Errorf("unknown scenario %q", name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no scenarios selected")
	}
	return out, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: perfcheck <capture|verify> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "capture: run scenarios and write baseline + thresholds")
	fmt.Fprintln(os.Stderr, "verify: run scenarios and fail if thresholds are exceeded")
}

func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "perfcheck: %v\n", err)
	os.Exit(1)
}

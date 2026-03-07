---
description: Measure and compare benchmark results for performance-sensitive changes
---

Use this workflow when a change may affect reconcile latency, config rendering cost, memory allocations, or hot code paths.

# Run a focused benchmark

Use a narrow package and benchmark filter when possible:

```bash
make bench BENCH_PKG=./internal/... BENCH_FILTER=BenchmarkName BENCH_COUNT=10
```

# Save comparable runs

Capture a baseline and a candidate run with the same package, filter, and count:

```bash
make bench-save BENCH_PKG=./internal/... BENCH_FILTER=BenchmarkName BENCH_COUNT=10
```

Outputs are written under `dist/bench/`.

# Compare with benchstat

```bash
make bench-compare OLD=dist/bench/old.txt NEW=dist/bench/new.txt
```

# Review guidance

- Keep the benchmark scope identical between runs.
- Ignore one-off noise; rerun if the delta is small or unstable.
- Treat allocation regressions as first-class signals, not just slower ns/op.

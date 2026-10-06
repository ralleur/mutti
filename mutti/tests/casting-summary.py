#!/usr/bin/env python3
"""Summarises casting results.jsonl like hub.summarize, also for aborted runs.

Usage: casting-summary.py <results.jsonl> [...]
Prints per model: category pass counts, failing cases with repetitions,
harness interventions and latency (first record per model = cold load, skipped).
"""
import json
import math
import sys
from collections import Counter, defaultdict


def pct(values, p):
    if not values:
        return 0
    values = sorted(values)
    return values[max(0, min(math.ceil(p * len(values)) - 1, len(values) - 1))]


for path in sys.argv[1:]:
    by_model = defaultdict(list)
    for line in open(path):
        r = json.loads(line)
        by_model[r['model']].append(r)
    for model, rs in by_model.items():
        cats, fails, corr = defaultdict(lambda: [0, 0]), defaultdict(list), Counter()
        first, tool, tps = [], [], []
        for i, r in enumerate(rs):
            cats[r['category']][1] += 1
            cats[r['category']][0] += r['passed']
            if not r['passed']:
                fails[r['case']].append(r['repeat'])
            corr.update(r.get('interventions') or [])
            if i:
                first.append(r['firstSeconds'])
                if r['category'] == 'tools':
                    tool.append(r['seconds'])
                if r['tokensPerSecond']:
                    tps.append(r['tokensPerSecond'])
        total = sum(v[0] for v in cats.values()), sum(v[1] for v in cats.values())
        print(f"{path}\n  {model}: {total[0]}/{total[1]}  " + "  ".join(f"{k} {v[0]}/{v[1]}" for k, v in sorted(cats.items())))
        print(f"  p95 first {pct(first, .95):.2f}s  p95 tool answer {pct(tool, .95):.2f}s  median {pct(tps, .5):.1f} tok/s  "
              f"errors {sum(1 for r in rs if r.get('error'))}  invalid markers {sum(r['invalidMarkers'] for r in rs)}")
        print(f"  failing: {dict(sorted(fails.items()))}")
        print(f"  interventions: {dict(corr)}")

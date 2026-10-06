#!/usr/bin/env python3
"""Derives the v4 development fixture from v3 (docs/mutti/evidence/casting-v4-rubric.md).

Data, prompts, expected arguments and answers stay unchanged. The v4 scorer
compares movie filters by meaning (minutes/seconds, inclusive/exclusive,
watched/unwatched), so only one new form needs an explicit alternative:
- T20 may use the new `year` filter instead of an unfiltered search.
"""
import json
from pathlib import Path

base = Path(__file__).parent / 'fixtures'
fixture = json.loads((base / 'model-casting-v3.json').read_text())
fixture['version'] = 'model-casting-v4-dev'
fixture['prompt'] = 'mutti-assistant-v4'
for case in fixture['cases']:
    if case['id'] == 'T20':
        case['checks']['calls'][0]['alternatives'] = [{'year': 2023}]
(base / 'model-casting-v4-dev.json').write_text(json.dumps(fixture, ensure_ascii=False, indent=1) + '\n')
print('v4 dev cases', len(fixture['cases']))

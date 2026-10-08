#!/usr/bin/env python3
"""Derives the v5 development and regression fixtures (docs/mutti/evidence/casting-v5-rubric.md).

Only the prompt version changes; cases, data and expectations stay identical:
- model-casting-v5-dev.json        <- model-casting-v4-dev.json
- model-casting-v5-regression.json <- model-casting-v4-holdout.json (already seen)
"""
import json
from pathlib import Path

base = Path(__file__).parent / 'fixtures'
for source, target in (('model-casting-v4-dev.json', 'model-casting-v5-dev.json'),
                       ('model-casting-v4-holdout.json', 'model-casting-v5-regression.json')):
    fixture = json.loads((base / source).read_text())
    fixture['version'] = target.removesuffix('.json')
    fixture['prompt'] = 'mutti-assistant-v5'
    (base / target).write_text(json.dumps(fixture, ensure_ascii=False, indent=1) + '\n')
    print(target, len(fixture['cases']))

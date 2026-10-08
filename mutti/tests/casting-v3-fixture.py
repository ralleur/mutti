#!/usr/bin/env python3
"""Derives the frozen v3 fixture from v2 (docs/mutti/evidence/casting-v3-rubric.md).

Only documented contract corrections; the data and all other cases are unchanged:
- runtime filters may use the new minutes parameter (T02, T08, T18);
- T17: the assistant cannot play media, so declining without a follow-up
  question is correct; claims of playback still fail;
- U05: searching documents for a bank statement is allowed.
"""
import json
from pathlib import Path

base = Path(__file__).parent / 'fixtures'
fixture = json.loads((base / 'model-casting-v2.json').read_text())
fixture['version'] = 'model-casting-v3'
fixture['prompt'] = 'mutti-assistant-v3'
alternatives = {'T02': [{'runtime_below_minutes': 90}], 'T08': [{'runtime_below_minutes': 120, 'unwatched': True}],
                'T18': [{'runtime_below_minutes': 90, 'unwatched': False}]}
for case in fixture['cases']:
    if case['id'] in alternatives:
        call = case['checks']['calls'][0]
        call['alternatives'] = alternatives[case['id']]
    if case['id'] == 'T17':
        case['checks'].pop('question', None)
    if case['id'] == 'U05':
        case['checks'].pop('no_calls', None)
(base / 'model-casting-v3.json').write_text(json.dumps(fixture, ensure_ascii=False, indent=1) + '\n')
print('v3 cases', len(fixture['cases']))

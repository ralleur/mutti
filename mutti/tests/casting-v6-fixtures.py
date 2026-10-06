#!/usr/bin/env python3
"""Derive the v6 casting sets from v5 without changing cases or expectations.

v6 moves the assistant contract to English; users in these sets still write
German, so each set gets language "de". Only the prompt version, the
language field, the legacy "german" check name and the tool argument name
"quelle" -> "source" change. The v5 holdout stays a holdout for v6: it was
frozen before any v6 work and is not used for development.
"""
import json, sys
from pathlib import Path

root = Path(__file__).resolve().parent / 'fixtures'
for name in ('dev', 'regression', 'holdout'):
    src = root / f'model-casting-v5-{name}.json'
    data = json.loads(src.read_text())
    data['version'] = data['version'].replace('v5', 'v6') if 'v5' in data['version'] else data['version'] + '-v6'
    data['prompt'] = 'mutti-assistant-v6'
    data['language'] = 'de'
    for case in data['cases']:
        checks = case.get('checks', {})
        if checks.pop('german', False):
            checks['language'] = True
        for call in checks.get('calls', []):
            for args in [call.get('args', {})] + call.get('alternatives', []):
                if 'quelle' in args:
                    args['source'] = args.pop('quelle')
    out = root / f'model-casting-v6-{name}.json'
    out.write_text(json.dumps(data, ensure_ascii=False, indent=1) + '\n')
    print(out.name, len(data['cases']))

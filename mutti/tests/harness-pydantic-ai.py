#!/usr/bin/env python3
"""D02 comparison: the same casting cases through Pydantic AI instead of the Mutti harness.

Uses the exported contract (system prompt and JSON schemas) and the frozen v2
fixture data, a running local engine and an isolated virtual environment.
Scores tool calls and document answers with the same rules as the Go runner
for the categories `tools` and `documents`. Development qualification only.
"""
import argparse
import asyncio
import json
import re
import time
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--engine', required=True, help='http://127.0.0.1:PORT of a local engine')
parser.add_argument('--model', required=True)
parser.add_argument('--contract', type=Path, required=True)
parser.add_argument('--fixture', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
if args.output.exists():
    raise SystemExit('Use a new result file')

from pydantic_ai import Agent, Tool  # noqa: E402
from pydantic_ai.messages import ModelRequest, ModelResponse, ToolCallPart  # noqa: E402
from pydantic_ai.models.openai import OpenAIChatModel  # noqa: E402
from pydantic_ai.providers.ollama import OllamaProvider  # noqa: E402

contract = json.loads(args.contract.read_text())
fixture = json.loads(args.fixture.read_text())
data = fixture['data']
system = contract['system']


def runtime(seconds):
    if seconds < 60:
        return f'{seconds} s'
    if seconds < 3600:
        return f'{seconds // 60} min' if seconds % 60 == 0 else f'{seconds // 60} min {seconds % 60} s'
    return f'{seconds // 3600} h {(seconds % 3600) // 60} min'


class Book:
    def __init__(self):
        self.items, self.next = {}, 0

    def add(self, service, oid, title):
        for ref, item in self.items.items():
            if item == (service, oid):
                return ref
        self.next += 1
        ref = f'Q{self.next}'
        self.items[ref] = (service, oid)
        return ref


def make_tools(book, calls):
    def record(name):
        def wrap(fn):
            def inner(**kwargs):
                calls.append({'name': name, 'arguments': kwargs})
                return fn(**kwargs)
            return inner
        return wrap

    @record('search_movies')
    def search_movies(query='', runtime_below_seconds=0, unwatched=None, genre='', sort='', limit=0):
        hits = []
        for m in data['movies']:
            if query and query.lower() not in m['title'].lower() and query.lower() not in m['overview'].lower():
                continue
            if runtime_below_seconds and m['seconds'] >= runtime_below_seconds:
                continue
            if unwatched is not None and unwatched == m['watched']:
                continue
            if genre and not any(genre.lower() in g.lower() for g in m['genres']):
                continue
            hits.append(m)
        key = {'runtime': lambda m: m['seconds'], 'year': lambda m: -m['year'], 'added': lambda m: m['added']}.get(sort, lambda m: m['title'].lower())
        hits.sort(key=key, reverse=(sort == 'added'))
        hits = hits[:limit if 0 < (limit or 0) <= 10 else 10]
        return json.dumps({'anzahl': len(hits), 'treffer': [{'quelle': book.add('media', m['key'], m['title']), 'titel': m['title'], 'jahr': m['year'],
                           'laufzeit': runtime(m['seconds']), 'laufzeit_sekunden': m['seconds'], 'gesehen': m['watched'], 'genres': m['genres']} for m in hits]}, ensure_ascii=False)

    @record('get_movie')
    def get_movie(quelle):
        item = book.items.get(quelle.strip('[]'))
        movie = next((m for m in data['movies'] if item and m['key'] == item[1]), None)
        if not movie:
            return json.dumps({'fehler': 'Unbekannte Quellenmarke.'})
        return json.dumps({'quelle': quelle, 'titel': movie['title'], 'jahr': movie['year'], 'laufzeit': runtime(movie['seconds']),
                           'beschreibung_daten': movie['overview']}, ensure_ascii=False)

    @record('propose_favorite')
    def propose_favorite(quelle, favorite):
        return json.dumps({'vorschlag': 'angelegt', 'status': 'wartet auf Bestätigung durch den Nutzer; noch nichts geändert', 'quelle': quelle}, ensure_ascii=False)

    @record('search_documents')
    def search_documents(query):
        words = [w for w in re.findall(r'[\w-]{3,}', query.lower())]
        hits = []
        for d in data['documents']:
            hay = (d['title'] + ' ' + d['text']).lower()
            score = sum(1 for w in words if w in hay)
            if score:
                hits.append((score, d))
        hits.sort(key=lambda h: -h[0])
        return json.dumps({'anzahl': len(hits[:5]), 'treffer': [{'quelle': book.add('documents', d['id'], d['title']), 'titel': d['title'], 'datum': d['created'],
                           'auszug_daten': (d['title'] + ' ' + d['text'])[:180]} for _, d in hits[:5]]}, ensure_ascii=False)

    @record('read_document')
    def read_document(quelle):
        item = book.items.get(quelle.strip('[]'))
        doc = next((d for d in data['documents'] if item and item[0] == 'documents' and d['id'] == item[1]), None)
        if not doc:
            return json.dumps({'fehler': 'Unbekannte Quellenmarke.'})
        return json.dumps({'quelle': quelle, 'titel': doc['title'], 'datum': doc['created'], 'text_daten': doc['text'][:6000]}, ensure_ascii=False)

    @record('search_photos')
    def search_photos(query='', **_):
        hits = [p for p in data['photos'] if query.lower() in (p['description'] + ' ' + p['city']).lower()]
        return json.dumps({'anzahl': len(hits), 'treffer': [{'quelle': book.add('photos', p['id'], p['description']), 'beschreibung_daten': p['description']} for p in hits]}, ensure_ascii=False)

    functions = {f.__name__: f for f in (search_movies, get_movie, propose_favorite, search_documents, read_document, search_photos)}
    tools = []
    for t in contract['tools']:
        fn = t['function']
        tools.append(Tool.from_schema(functions[fn['name']], name=fn['name'], description=fn['description'], json_schema=fn['parameters']))
    return tools


def score(case, answer, calls, book):
    fails = []
    checks = case['checks']
    for i, want in enumerate(checks.get('calls', [])):
        if i >= len(calls) or calls[i]['name'] != want['name']:
            fails.append(f'call {i + 1} {calls[i]["name"] if i < len(calls) else "missing"} want {want["name"]}')
            continue
        got = calls[i]['arguments']
        for key, value in want['args'].items():
            if key == 'quelle':
                kind, name = value.split(':')
                ref = next((r for r, it in book.items.items() if str(it[1]) in (name, str(next((d['id'] for d in data['documents'] if d['key'] == name), '')))), '')
                if str(got.get(key, '')).strip('[]') != ref:
                    fails.append(f'quelle {got.get(key)} want {ref}')
            elif isinstance(value, str):
                if value.lower() not in str(got.get(key, '')).lower():
                    fails.append(f'{key}={got.get(key)}')
            elif got.get(key) != value:
                fails.append(f'{key}={got.get(key)} want {value}')
        for key, value in got.items():
            if key not in want['args'] and key not in want['allow_extra'] and value not in (None, '', 0, False):
                fails.append(f'unexpected {key}={value}')
    if checks.get('no_calls') and calls:
        fails.append('unexpected tool call')
    for s in checks.get('answer_all', []):
        if s.lower() not in answer.lower():
            fails.append(f'lacks {s}')
    if checks.get('answer_any') and not any(s.lower() in answer.lower() for s in checks['answer_any']):
        fails.append('lacks any')
    for t in checks.get('titles_all', []):
        if t.lower() not in answer.lower():
            fails.append(f'title {t}')
    markers = re.findall(r'\[(Q\d+)\]', answer)
    if any(m not in book.items for m in markers):
        fails.append('invalid markers')
    return fails


async def main():
    model = OpenAIChatModel(args.model, provider=OllamaProvider(base_url=args.engine + '/v1'))
    results = []
    for case in fixture['cases']:
        if case['category'] not in ('tools', 'documents') or case.get('attachment') or case.get('history') or case.get('fail_tools'):
            continue
        book, calls = Book(), []
        agent = Agent(model, instructions=system, tools=make_tools(book, calls),
                      model_settings={'temperature': 0, 'seed': 42, 'max_tokens': 1024})
        start = time.monotonic()
        first = None
        text = ''
        error = None
        try:
            async with agent.run_stream(case['prompt']) as run:
                async for delta in run.stream_text(delta=True):
                    if first is None:
                        first = time.monotonic() - start
                    text += delta
        except Exception as e:  # noqa: BLE001 - recorded as a failed case
            error = f'{type(e).__name__}: {e}'
        fails = score(case, text, calls, book) + ([error] if error else [])
        result = {'case': case['id'], 'category': case['category'], 'passed': not fails, 'failures': fails, 'calls': calls, 'answer': text,
                  'seconds': time.monotonic() - start, 'firstSeconds': first}
        results.append(result)
        print(case['id'], result['passed'], round(result['seconds'], 1), '; '.join(fails)[:200], flush=True)
    args.output.write_text('\n'.join(json.dumps(r, ensure_ascii=False) for r in results) + '\n')
    summary = {}
    for r in results:
        s = summary.setdefault(r['category'], [0, 0])
        s[1] += 1
        s[0] += r['passed']
    print(json.dumps({k: f'{v[0]}/{v[1]}' for k, v in summary.items()}))


asyncio.run(main())

// SPDX-License-Identifier: GPL-2.0-or-later
// Kurt is maintained centrally; this is only the Mutti update entry point.
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
const consumer = fileURLToPath(new URL('../../', import.meta.url));
const kurt = resolve(process.argv[2] || resolve(consumer, '../kurt'));
for (const args of [[resolve(kurt, 'tools/export.py'), 'mutti'],
    [resolve(kurt, 'tools/sync.py'), '--consumer', 'mutti', '--root', consumer]]) {
    const result = spawnSync('python3', args, {stdio: 'inherit'});
    if (result.error) throw result.error;
    if (result.status !== 0) process.exit(result.status || 1);
}

// SPDX-License-Identifier: GPL-2.0-or-later
// Extract unchanged drawings from the owner's Hauser atlas into a small,
// self-contained Mutti bundle. Run only when deliberately updating artwork.
import {createRequire} from 'node:module';
import {readFile, writeFile, mkdir} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {resolve, join} from 'node:path';
import {fileURLToPath} from 'node:url';

const hauser = resolve(process.argv[2] || '../smart-home-hmi');
const sharp = createRequire(join(hauser, 'app/package.json'))('sharp');
const source = join(hauser, 'docs/kurt/animation-v11/ios-assets');
const out = new URL('../migrate/web/kurt/', import.meta.url);
await mkdir(out, {recursive: true});
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const inputs = {};
async function read(name) {
    const bytes = await readFile(join(source, name));
    inputs[name] = sha(bytes);
    return bytes;
}
const original = JSON.parse(await read('manifest.json'));
const ids = ['idle-sleep', 'wake-seat', 'rise-seat', 'scoot-floor', 'walk-floor', 'run-floor', 'eat-floor', 'vomit-floor', 'poop-floor', 'turn-floor'];
const manifest = {version: original.version, fps: original.fps, cell: original.cell, columns: 8, anchor: original.anchor, bodyRatio: original.bodyWidth / original.sourceSize, pages: [], clips: {}};
const pages = new Map(), tiles = [], hashes = new Map(), locations = new Map();
async function tile(frame) {
    const location = [frame.page, frame.column, frame.row].join(':');
    if (locations.has(location)) return locations.get(location);
    const file = original.pages[frame.page];
    if (!pages.has(file)) pages.set(file, await read(file));
    const {data, info} = await sharp(pages.get(file)).extract({left: frame.column * original.cell, top: frame.row * original.cell, width: original.cell, height: original.cell}).ensureAlpha().raw().toBuffer({resolveWithObject: true});
    const digest = sha(data);
    if (!hashes.has(digest)) {
        hashes.set(digest, tiles.length);
        tiles.push(await sharp(data, {raw: info}).png().toBuffer());
    }
    const index = hashes.get(digest);
    locations.set(location, index);
    return index;
}
for (const id of ids) {
    const sequence = original.sequences.find(s => s.id === id);
    if (!sequence) throw new Error('Missing source clip: ' + id);
    const clip = {frames: []};
    if (sequence.distance !== undefined) clip.distanceInBodies = sequence.distance * 1280 / 104;
    if (sequence.waste) {
        clip.waste = sequence.waste;
        clip.deposit = sequence.deposit - sequence.first;
    }
    for (const f of original.frames.slice(sequence.first, sequence.first + sequence.count)) {
        const result = {tile: await tile(f)};
        for (const key of ['travel', 'facing', 'falling']) if (f[key] !== undefined) result[key] = f[key];
        // Source lift is normalized to its 906px room / 104px body width.
        if (f.lift) result.liftInBodies = f.lift * 906 / 104;
        clip.frames.push(result);
    }
    manifest.clips[id] = clip;
}
for (let start = 0; start < tiles.length; start += 64) {
    const name = `atlas-${manifest.pages.length}.webp`;
    await sharp({create: {width: 2048, height: 2048, channels: 4, background: {r: 0, g: 0, b: 0, alpha: 0}}})
        .composite(tiles.slice(start, start + 64).map((input, i) => ({input, left: i % 8 * 256, top: Math.floor(i / 8) * 256})))
        .webp({lossless: true}).toFile(fileURLToPath(new URL(name, out)));
    manifest.pages.push(name);
}
for (const name of ['poop.png', 'vomit.png', 'poop-drop.png', 'vomit-drop.png']) await writeFile(new URL(name, out), await read(name));
const stand = manifest.clips['rise-seat'].frames.at(-1).tile;
await writeFile(new URL('rest.png', out), tiles[stand]);
await writeFile(new URL('manifest.json', out), JSON.stringify(manifest) + '\n');
await writeFile(new URL('provenance.json', out), JSON.stringify({source: 'Hauser private Kurt lab, docs/kurt/animation-v11/ios-assets', version: original.version, authorization: 'Owner requested reuse in Mutti import, 2026-10-05', changes: 'Selected original frames, lossless atlas repacking and pixel-identical deduplication; no redrawing.', uniqueTiles: tiles.length, inputs}, null, 2) + '\n');
console.log({version: manifest.version, clips: ids.length, uniqueTiles: tiles.length, pages: manifest.pages.length});

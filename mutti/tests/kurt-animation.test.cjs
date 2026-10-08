// SPDX-License-Identifier: GPL-2.0-or-later
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.resolve(__dirname, '../migrate/web');
const assets = JSON.parse(fs.readFileSync(path.join(root, 'kurt/manifest.json')));
function fixture(reduced = false) {
    const events = {}, requests = new Map(); let next = 1, time = 0;
    const context = new Proxy({}, {get: () => () => {}});
    const canvas = {dataset: {}, getContext: () => context};
    const poster = {hidden: false};
    const button = {addEventListener: (name, fn) => events[name] = fn, setAttribute() {}};
    const figure = {dataset: {}, querySelector: selector => ({canvas, img: poster, button})[selector]};
    const media = {matches: reduced, addEventListener: (_, fn) => events.motion = fn};
    const document = {hidden: false, addEventListener: (_, fn) => events.visibility = fn};
    const window = {};
    vm.runInNewContext(fs.readFileSync(path.join(root, 'kurt.js'), 'utf8'), {window, document, matchMedia: () => media,
        requestAnimationFrame: fn => {const id = next++; requests.set(id, fn); return id;}, cancelAnimationFrame: id => requests.delete(id)});
    const animator = new window.MuttiKurt(figure);
    animator.assets = assets;
    animator.images = Object.fromEntries([...assets.pages, 'poop.png', 'vomit.png', 'poop-drop.png', 'vomit-drop.png'].map(name => [name, {width: 128, height: 128}]));
    animator.setVisible(true);
    function step(value, job = 'job-1', connected = true) {animator.setState({step: value, job, connected});}
    function advance(ms) {
        for (let i = 0; i < ms; i += 50) {
            time += 50;
            const callbacks = [...requests.values()]; requests.clear();
            for (const callback of callbacks) callback(time);
        }
    }
    return {animator, canvas, figure, events, document, media, requests, advance, step};
}
test('each actual import step selects its action; reload does not replay old steps', () => {
    for (const [step, id] of [[1,'idle-sleep'],[2,'scoot-floor'],[3,'walk-floor'],[4,'run-floor'],[5,'eat-floor'],[6,'vomit-floor'],[7,'poop-floor']]) {
        const f = fixture(); f.step(step); f.advance(100);
        assert.equal(f.canvas.dataset.clip, id);
        assert.equal(f.figure.dataset.animation, 'playing');
        assert.equal(f.requests.size, 1);
    }
});
test('sleep exits through wake and rise once, then follows the newest step', () => {
    const f = fixture(); f.step(1); f.advance(100); f.step(2);
    assert.equal(f.canvas.dataset.clip, 'wake-seat');
    f.advance(1650); assert.equal(f.canvas.dataset.clip, 'rise-seat');
    f.step(5); f.advance(1700); assert.equal(f.canvas.dataset.clip, 'eat-floor');
    f.advance(6000); assert.equal(f.canvas.dataset.clip, 'eat-floor');
    f.step(1, 'new-job'); assert.equal(f.canvas.dataset.clip, 'idle-sleep');
});
test('pause, disconnection and hidden pages freeze without accumulating time', () => {
    const f = fixture(); f.step(3); f.advance(600);
    for (const suspend of [
        value => f.events.click(),
        value => f.step(3, 'job-1', !value),
        value => {f.document.hidden = value; f.events.visibility();},
        value => f.animator.setVisible(!value)
    ]) {
        const elapsed = f.animator.elapsed;
        suspend(true); assert.equal(f.requests.size, 0); f.advance(3000);
        assert.equal(f.animator.elapsed, elapsed);
        suspend(false); assert.equal(f.requests.size, 1); f.advance(100);
        assert.ok(f.animator.elapsed - elapsed < .2);
    }
});
test('reduced motion uses a stable pose and reacts to preference changes', () => {
    const f = fixture(true); f.step(6); f.advance(500);
    assert.equal(f.canvas.dataset.clip, 'rise-seat');
    assert.equal(f.canvas.dataset.frame, String(assets.clips['rise-seat'].frames.length - 1));
    assert.equal(f.requests.size, 0);
    f.media.matches = false; f.events.motion(); f.advance(100);
    assert.equal(f.canvas.dataset.clip, 'vomit-floor');
    f.media.matches = true; f.events.motion();
    assert.equal(f.requests.size, 0);
});
test('travel reverses through the existing turn clip with no loop-boundary teleport', () => {
    for (const step of [2,3,4]) {
        const f = fixture(); f.step(step);
        let previous;
        const ids = new Set();
        for (let i = 0; i < 500; i++) {
            f.animator.elapsed = i / assets.fps;
            const pose = f.animator.sample(); ids.add(pose.id);
            assert.ok(pose.x >= 100 && pose.x <= 540, JSON.stringify(pose));
            if (previous) assert.ok(Math.abs(pose.x - previous.x) < 40, 'position jumped at a loop boundary');
            previous = pose;
        }
        assert.ok(ids.has('turn-floor'));
    }
});
test('waste is per-frame, drops precede deposition and the next cycle starts clean', () => {
    for (const [step, id] of [[6,'vomit-floor'],[7,'poop-floor']]) {
        const f = fixture(); f.step(step); const clip = assets.clips[id];
        assert.equal(clip.frames.slice(0, clip.deposit).filter(x => x.falling).length, 6);
        f.animator.elapsed = (clip.frames.length - 1) / assets.fps;
        assert.ok(f.animator.sample().index >= clip.deposit);
        f.animator.elapsed = clip.frames.length / assets.fps;
        assert.equal(f.animator.sample().index, 0);
    }
});
test('every referenced local image exists and every atlas location is valid', () => {
    for (const file of [...assets.pages, 'rest.png', 'poop.png', 'vomit.png', 'poop-drop.png', 'vomit-drop.png']) assert.ok(fs.statSync(path.join(root, 'kurt', file)).size > 0);
    for (const clip of Object.values(assets.clips)) for (const frame of clip.frames) {
        assert.ok(Number.isInteger(frame.tile) && frame.tile >= 0 && frame.tile < assets.pages.length * 64);
    }
});

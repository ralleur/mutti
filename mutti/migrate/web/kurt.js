// SPDX-License-Identifier: GPL-2.0-or-later
// A decorative observer of import progress. Never controls the import job.
(() => {
    const clips = ['', 'idle-sleep', 'scoot-floor', 'walk-floor', 'run-floor', 'eat-floor', 'vomit-floor', 'poop-floor'];
    class KurtAnimator {
        constructor(figure) {
            this.figure = figure;
            this.canvas = figure.querySelector('canvas');
            this.context = this.canvas.getContext('2d');
            this.poster = figure.querySelector('img');
            this.button = figure.querySelector('button');
            this.motion = matchMedia('(prefers-reduced-motion: reduce)');
            this.state = {step: 0, job: '', connected: false};
            this.visible = false;
            this.paused = false;
            this.elapsed = 0;
            this.waking = false;
            this.timer = 0;
            this.lastTime = null;
            this.lastFrame = '';
            this.button.addEventListener('click', () => { this.paused = !this.paused; this.sync(); });
            this.motion.addEventListener('change', () => { this.waking = false; this.sync(); this.draw(); });
            document.addEventListener('visibilitychange', () => this.sync());
            this.sync();
        }
        async load() {
            if (this.loading || this.assets || !this.context) return;
            this.loading = true;
            try {
                const response = await fetch('kurt/manifest.json', {signal: AbortSignal.timeout(10000)});
                if (!response.ok) throw new Error('Kurt artwork unavailable');
                const manifest = await response.json();
                const names = [...manifest.pages, 'poop.png', 'vomit.png', 'poop-drop.png', 'vomit-drop.png'];
                const images = await Promise.all(names.map(name => new Promise((resolve, reject) => {
                    const img = new Image();
                    const timeout = setTimeout(() => reject(new Error('Kurt image timeout')), 10000);
                    img.onload = () => { clearTimeout(timeout); resolve(img); };
                    img.onerror = () => { clearTimeout(timeout); reject(new Error('Kurt image unavailable')); };
                    img.src = 'kurt/' + name;
                })));
                this.assets = manifest;
                this.images = Object.fromEntries(names.map((name, i) => [name, images[i]]));
                this.poster.hidden = true;
                this.canvas.hidden = false;
                this.draw();
                this.sync();
            } catch {
                // The real progress UI and import stay available if decoration fails.
                this.figure.dataset.animation = 'unavailable';
                this.button.hidden = true;
            }
        }
        setVisible(visible) {
            this.visible = visible;
            if (visible) void this.load();
            this.sync();
        }
        setState(state) {
            const previous = this.state;
            const fresh = state.job !== previous.job;
            if (fresh || state.step !== previous.step) {
                if (fresh) this.waking = false;
                if (!fresh && previous.step === 1 && state.step > 1 && !this.motion.matches) this.waking = true;
                if (!this.waking || previous.step === 1) this.elapsed = 0;
                this.lastFrame = '';
            }
            this.state = state;
            this.sync();
            this.draw();
        }
        sync() {
            const running = !!(this.assets && this.visible && this.state.connected && clips[this.state.step] && !this.paused && !this.motion.matches && !document.hidden);
            this.button.disabled = this.motion.matches;
            this.button.textContent = this.motion.matches ? 'Bewegung reduziert' : this.paused ? 'Kurt weitermachen lassen' : 'Kurt pausieren';
            this.button.setAttribute('aria-pressed', String(this.paused || this.motion.matches));
            this.figure.dataset.animation = running ? 'playing' : 'paused';
            if (running && !this.timer) {
                this.lastTime = null;
                this.timer = requestAnimationFrame(now => this.tick(now));
            } else if (!running && this.timer) {
                cancelAnimationFrame(this.timer);
                this.timer = 0;
                this.lastTime = null;
            }
        }
        tick(now) {
            if (this.lastTime !== null) this.elapsed += Math.min(250, now - this.lastTime) / 1000;
            this.lastTime = now;
            this.draw();
            this.timer = requestAnimationFrame(time => this.tick(time));
        }
        sample() {
            const a = this.assets;
            if (this.motion.matches) return {id: 'rise-seat', index: a.clips['rise-seat'].frames.length - 1, x: 320, facing: -1};
            let frame = Math.floor(this.elapsed * a.fps);
            if (this.waking) {
                for (const id of ['wake-seat', 'rise-seat']) {
                    if (frame < a.clips[id].frames.length) return {id, index: frame, x: 320, facing: -1};
                    frame -= a.clips[id].frames.length;
                }
                this.waking = false;
                this.elapsed = frame / a.fps;
            }
            const id = clips[this.state.step] || 'idle-sleep', clip = a.clips[id];
            if (clip.distanceInBodies !== undefined) {
                const turn = a.clips['turn-floor'];
                const legLength = clip.frames.length + turn.frames.length;
                const cycle = frame % (2 * legLength), returning = cycle >= legLength;
                const local = cycle % legLength, distance = clip.distanceInBodies * 180;
                if (local >= clip.frames.length) {
                    const index = local - clip.frames.length;
                    return {id: 'turn-floor', index, x: 320 + (returning ? -1 : 1) * distance / 2, facing: (returning ? -1 : 1) * turn.frames[index].facing};
                }
                const travel = clip.frames[local].travel || 0;
                return {id, index: local, x: 320 + (returning ? -1 : 1) * distance * (travel - .5), facing: returning ? 1 : -1};
            }
            return {id, index: frame % clip.frames.length, x: 320, facing: -1};
        }
        draw() {
            if (!this.assets || !this.visible) return;
            const pose = this.sample();
            const key = [pose.id, pose.index, pose.x, pose.facing].join(':');
            if (key === this.lastFrame) return;
            this.lastFrame = key;
            const a = this.assets, clip = a.clips[pose.id], frame = clip.frames[pose.index];
            const ctx = this.context, width = 180, size = width / a.bodyRatio, y = 215;
            const tile = frame.tile, page = Math.floor(tile / 64), column = tile % 8, row = Math.floor(tile % 64 / 8);
            ctx.clearRect(0, 0, 640, 240);
            ctx.strokeStyle = '#484843'; ctx.lineWidth = 1;
            ctx.beginPath(); ctx.moveTo(30, y + 1); ctx.lineTo(610, y + 1); ctx.stroke();
            ctx.fillStyle = 'rgba(0, 0, 0, .22)';
            ctx.beginPath(); ctx.ellipse(pose.x, y, width * .36, 6, 0, 0, 2 * Math.PI); ctx.fill();
            ctx.save(); ctx.translate(pose.x, y - (frame.liftInBodies || 0) * width); ctx.scale(pose.facing, 1);
            ctx.drawImage(this.images[a.pages[page]], column * a.cell, row * a.cell, a.cell, a.cell, -a.anchor.x * size, -a.anchor.y * size, size, size);
            ctx.restore();
            if (frame.falling) {
                const f = frame.falling;
                ctx.save(); ctx.translate(pose.x - f.x * width * pose.facing, y + f.y * width); ctx.scale(-pose.facing, 1);
                ctx.drawImage(this.images[f.kind + '-drop.png'], -f.width * width / 2, -f.height * width, f.width * width, f.height * width);
                ctx.restore();
            }
            if (clip.waste && pose.index >= clip.deposit) {
                const mark = this.images[clip.waste + '.png'], w = 72, h = w * mark.height / mark.width;
                const offset = clip.waste === 'poop' ? -.32 : .38, bottom = clip.waste === 'poop' ? 114 / 128 : 105 / 128;
                ctx.drawImage(mark, pose.x - offset * width * pose.facing - w / 2, y - h * bottom, w, h);
            }
            this.canvas.dataset.clip = pose.id;
            this.canvas.dataset.frame = String(pose.index);
        }
    }
    window.MuttiKurt = KurtAnimator;
})();

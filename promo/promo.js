/* ============================================================================
   ReX API — 产品动画 / timeline engine
   ----------------------------------------------------------------------------
   One clock. render(t) is a pure function of absolute seconds, so scrubbing to
   any t (URL ?t=, arrow keys, progress bar) is exact and replayable — nothing
   accumulates between frames.

   Every cue is a list of absolute-time keyframes. Props:
     x, y   px offset          yp   % offset of the element's own height (mask reveals)
     s      uniform scale      sx, sy  axis scale (used for hairline draw-on)
     r      degrees            o    opacity      b  blur px
   Omitted props fall back to DEF, so each key only states what it changes.
   ========================================================================= */
(() => {
'use strict';

const DUR = 60.0;

/* ── easing ─────────────────────────────────────────────────────────────── */
const E = {
  glide: t => 1 - Math.pow(1 - t, 5),   // long, weighted settle — the default entrance
  soft:  t => 1 - Math.pow(1 - t, 4),
  out:   t => 1 - Math.pow(1 - t, 3),
  inOut: t => t < .5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2,
  none:  t => t,
};

/* ── scene windows ──────────────────────────────────────────────────────── */
const SCENES = [
  { id: 's1', in: 0.0,  out: 8.0,  fade: 0.40 },
  { id: 's2', in: 8.0,  out: 19.0, fade: 0.35 },
  { id: 's3', in: 19.0, out: 31.0, fade: 0.35 },
  { id: 's4', in: 31.0, out: 48.0, fade: 0.35 },
  { id: 's5', in: 48.0, out: 60.0, fade: 0.35 },
];

/* ── copy that the layout is generated from ─────────────────────────────── */
/* Masked key labels. Deliberately provider-neutral: no upstream prefix is
   implied, only the shape of a key you have to keep track of. */
const KEY_LABELS = [
  'sk-••••••••4f2a', 'sk-••••••••91c3', 'sk-••••••••0d7e', 'sk-••••••••b218',
  'sk-••••••••6a55', 'sk-••••••••e304', 'sk-••••••••7c9f', 'sk-••••••••25b1',
  'sk-••••••••fa60', 'sk-••••••••3e8d', 'sk-••••••••c142', 'sk-••••••••88aa',
  'sk-••••••••5d0b', 'sk-••••••••a7f6',
];

/* Scatter layout, % of frame. Hand-placed to read as sprawl, not a grid.
   Kept clear of the headline band (top 30%) and the collapse point. */
const KEY_POS = [
  { l: 11, t: 41 }, { l: 30, t: 37 }, { l: 51, t: 42 }, { l: 72, t: 38 }, { l: 89, t: 43 },
  { l: 19, t: 57 }, { l: 39, t: 52 }, { l: 61, t: 58 }, { l: 82, t: 53 },
  { l:  8, t: 73 }, { l: 27, t: 79 }, { l: 47, t: 72 }, { l: 68, t: 80 }, { l: 88, t: 74 },
];

const ENDPOINTS = ['对话', '向量化', '重排', '图像生成', '语音合成', '视频生成', '3D 生成', 'Claude 兼容', 'Gemini 兼容'];

const GROUPS_A = ['ClaudeMax', 'CodexPro', 'Gemini', '国模高速', 'ClaudeKiro', 'CodexTeam', 'NIM', 'CodexPlus'];
const GROUPS_B = ['Grok', 'CodexCyber', 'ClaudeMax2', 'free1', 'CodexOfficial', 'CodexTeam1', 'free2', 'CodexTeam2'];
const HOT = new Set(['国模高速', 'NIM', 'free1', 'free2']);
const CJK = /[一-鿿]/;

/* ── the timeline ───────────────────────────────────────────────────────── */
const CUES = [
  /* 01 · cold open — a hairline draws, the wordmark rises out of a mask */
  { cue: 's1-rule', keys: [
    { t: 0.35, sx: 0, o: 0 },
    { t: 1.75, sx: 1, o: 1 },
    { t: 6.55, sx: 1, o: 1 },
    { t: 7.45, sx: 0, o: 0, e: E.inOut } ] },

  { cue: 's1-mark', keys: [
    { t: 1.00, yp: 126, o: 0 },
    { t: 2.55, yp: 0,   o: 1 },
    { t: 6.40, yp: 0,   o: 1 },
    { t: 7.35, yp: -108, o: 0, e: E.inOut } ] },

  { cue: 's1-sub', keys: [
    { t: 2.90, y: 22, o: 0 },
    { t: 4.15, y: 0,  o: 1 },
    { t: 6.40, y: 0,  o: 1 },
    { t: 7.35, y: -18, o: 0, e: E.inOut } ] },

  /* 02 · the problem — sprawl, then collapse */
  { cue: 's2-label', keys: [
    { t: 8.25, x: -10, o: 0 },
    { t: 9.25, x: 0,   o: 1 },
    { t: 17.50, o: 1 },
    { t: 18.40, o: 0, e: E.inOut } ] },

  { cue: 's2-q', keys: [
    { t: 8.70, y: 26, o: 0 },
    { t: 10.10, y: 0, o: 1 },
    { t: 13.90, y: 0, o: 1 },
    { t: 14.70, y: -24, o: 0, e: E.inOut } ] },

  { cue: 's2-ring', keys: [
    { t: 15.35, s: 0.20, o: 0 },
    { t: 15.65, s: 0.55, o: 0.85 },
    { t: 16.50, s: 1.60, o: 0, e: E.out } ] },

  { cue: 's2-one', keys: [
    { t: 16.20, y: 24, o: 0 },
    { t: 17.35, y: 0,  o: 1 },
    { t: 18.35, y: 0,  o: 1 },
    { t: 19.15, y: -20, o: 0, e: E.inOut } ] },

  /* 03 · the turn — the emotional centre: three statements, then the diff */
  { cue: 's3-label', keys: [
    { t: 19.25, x: -10, o: 0 },
    { t: 20.25, x: 0,   o: 1 },
    { t: 29.00, o: 1 },
    { t: 29.90, o: 0, e: E.inOut } ] },

  { cue: 's3-l1', keys: [
    { t: 19.70, yp: 126, o: 0 },
    { t: 21.10, yp: 0,   o: 1 },
    { t: 21.45, yp: 0,   o: 1 },
    { t: 21.95, yp: -108, o: 0, e: E.inOut } ] },

  { cue: 's3-l2', keys: [
    { t: 21.85, yp: 126, o: 0 },
    { t: 23.25, yp: 0,   o: 1 },
    { t: 23.60, yp: 0,   o: 1 },
    { t: 24.10, yp: -108, o: 0, e: E.inOut } ] },

  { cue: 's3-l3', keys: [
    { t: 24.00, yp: 126, o: 0 },
    { t: 25.40, yp: 0,   o: 1 },
    { t: 25.95, yp: 0,   o: 1 },
    { t: 26.45, yp: -108, o: 0, e: E.inOut } ] },

  { cue: 's3-diff', keys: [
    { t: 26.35, y: 22, o: 0 },
    { t: 27.70, y: 0,  o: 1 },
    { t: 29.90, y: 0,  o: 1 },
    { t: 30.60, y: -16, o: 0, e: E.inOut } ] },

  { cue: 's3-diff-a', keys: [
    { t: 26.50, x: -8, o: 0 },
    { t: 27.50, x: 0,  o: 1 },
    { t: 29.90, o: 1 },
    { t: 30.60, o: 0, e: E.inOut } ] },

  { cue: 's3-diff-b', keys: [
    { t: 27.15, x: -8, o: 0 },
    { t: 28.15, x: 0,  o: 1 },
    { t: 29.90, o: 1 },
    { t: 30.60, o: 0, e: E.inOut } ] },

  { cue: 's3-note', keys: [
    { t: 28.30, y: 16, o: 0 },
    { t: 29.50, y: 0,  o: 1 },
    { t: 29.90, y: 0,  o: 1 },
    { t: 30.60, y: -12, o: 0, e: E.inOut } ] },

  /* 04a · models */
  { cue: 's4-label', keys: [
    { t: 31.25, x: -10, o: 0 },
    { t: 32.25, x: 0,   o: 1 },
    { t: 46.40, o: 1 },
    { t: 47.30, o: 0, e: E.inOut } ] },

  { cue: 's4-num', keys: [
    { t: 31.00, y: 44, o: 0, s: 0.95 },
    { t: 32.40, y: 0,  o: 1, s: 1 },
    { t: 33.80, y: 0,  o: 1, s: 1 },
    { t: 34.50, y: -30, o: 0, s: 1.03, e: E.inOut } ] },

  { cue: 's4-num-cap', keys: [
    { t: 31.70, y: 16, o: 0 },
    { t: 32.90, y: 0,  o: 1 },
    { t: 33.80, y: 0,  o: 1 },
    { t: 34.50, y: -12, o: 0, e: E.inOut } ] },

  /* 04b · endpoint families */
  { cue: 's4-eps', keys: [
    { t: 34.70, y: 18, o: 0 },
    { t: 35.90, y: 0,  o: 1 },
    { t: 38.70, y: 0,  o: 1 },
    { t: 39.40, y: -16, o: 0, e: E.inOut } ] },

  { cue: 's4-ep-cap', keys: [
    { t: 36.30, y: 14, o: 0 },
    { t: 37.40, y: 0,  o: 1 },
    { t: 38.70, y: 0,  o: 1 },
    { t: 39.40, y: -10, o: 0, e: E.inOut } ] },

  /* 04c · channel groups */
  { cue: 's4-groups-h', keys: [
    { t: 40.30, y: 18, o: 0 },
    { t: 41.50, y: 0,  o: 1 },
    { t: 43.90, y: 0,  o: 1 },
    { t: 44.60, y: -14, o: 0, e: E.inOut } ] },

  { cue: 's4-groups', keys: [
    { t: 39.70, y: 16, o: 0 },
    { t: 40.80, y: 0,  o: 1 },
    { t: 43.90, y: 0,  o: 1 },
    { t: 44.60, y: -14, o: 0, e: E.inOut } ] },

  /* 04d · highlights */
  { cue: 's4-hl1', keys: [
    { t: 44.85, y: 20, o: 0 },
    { t: 46.00, y: 0,  o: 1 },
    { t: 47.30, y: 0,  o: 1 },
    { t: 47.90, y: -14, o: 0, e: E.inOut } ] },

  { cue: 's4-hl2', keys: [
    { t: 45.55, y: 20, o: 0 },
    { t: 46.70, y: 0,  o: 1 },
    { t: 47.30, y: 0,  o: 1 },
    { t: 47.90, y: -14, o: 0, e: E.inOut } ] },

  /* 05 · close — the poster frame, held to the end */
  { cue: 's5-l1', keys: [
    { t: 48.30, y: 26, o: 0 },
    { t: 49.60, y: 0,  o: 1 },
    { t: 51.30, y: 0,  o: 1 },
    { t: 51.95, y: -20, o: 0, e: E.inOut } ] },

  { cue: 's5-l2', keys: [
    { t: 49.00, y: 20, o: 0 },
    { t: 50.30, y: 0,  o: 1 },
    { t: 51.30, y: 0,  o: 1 },
    { t: 51.95, y: -16, o: 0, e: E.inOut } ] },

  { cue: 's5-rule', keys: [
    { t: 51.90, sx: 0, o: 0 },
    { t: 53.00, sx: 1, o: 1 },
    { t: 60.00, sx: 1, o: 1 } ] },

  { cue: 's5-mark', keys: [
    { t: 52.50, yp: 126, o: 0 },
    { t: 54.10, yp: 0,   o: 1 },
    { t: 60.00, yp: 0,   o: 1 } ] },

  { cue: 's5-url', keys: [
    { t: 54.70, y: 18, o: 0 },
    { t: 56.00, y: 0,  o: 1 },
    { t: 60.00, y: 0,  o: 1 } ] },

  { cue: 's5-tag', keys: [
    { t: 56.50, y: 14, o: 0 },
    { t: 57.70, y: 0,  o: 1 },
    { t: 60.00, y: 0,  o: 1 } ] },
];

/* The 251 counter, driven by the same clock. */
const COUNTER = { el: null, from: 0, to: 251, t0: 31.05, t1: 32.45, last: -1 };

/* ── DOM build ──────────────────────────────────────────────────────────── */
const film = document.getElementById('film');
const stage = document.getElementById('stage');
const hud = document.getElementById('hud');
const fill = document.getElementById('fill');
const bar = document.getElementById('bar');
const clockEl = document.getElementById('clock');

const keyfield = document.getElementById('keyfield');
const keyInners = [];

function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}

/* masked keys, scattered and absolutely placed */
KEY_LABELS.forEach((label, i) => {
  const p = KEY_POS[i];
  const slot = el('div', 'chip');
  slot.style.left = p.l + '%';
  slot.style.top = p.t + '%';
  const inner = el('i', 'a', label);
  slot.appendChild(inner);
  keyfield.appendChild(slot);

  // Enter from a golden-angle radial scatter — reads as a swarm arriving.
  const ang = i * 2.39996;
  const rad = 24 + (i % 3) * 7;
  const dx = Math.cos(ang) * rad;
  const dy = Math.sin(ang) * rad * 0.72;
  const rot = (i % 2 ? 1 : -1) * (3 + (i % 4) * 1.6);

  const c = { el: inner, keys: [
    { t: 9.90 + i * 0.105, x: dx, y: dy, r: rot, s: 0.92, o: 0 },
    { t: 10.85 + i * 0.105, x: 0, y: 0, r: 0, s: 1, o: 1 },
    { t: 14.75, x: 0, y: 0, r: 0, s: 1, o: 1 },
    { t: 15.35 + i * 0.030, x: 0, y: 0, r: 0, s: 0.30, o: 0, e: E.inOut } ] };
  // The collapse target is the frame centre, which is a px offset that only
  // layout can tell us — filled in by layoutCollapse().
  c.collapseKey = c.keys[3];
  c.slot = slot;
  // Idle swarm drift, faded out as the chip fades out so it never fights the exit.
  c.drift = { amp: 2.6, freq: 1.15, phase: i * 0.7, axis: 'y' };
  CUES.push(c);
  keyInners.push(c);
});

/* endpoint chips */
const epWrap = document.getElementById('epchips');
ENDPOINTS.forEach((name, i) => {
  const chip = el('span', 'ep a', name);
  epWrap.appendChild(chip);
  CUES.push({ el: chip, keys: [
    { t: 34.85 + i * 0.075, y: 14, s: 0.94, o: 0 },
    { t: 35.60 + i * 0.075, y: 0,  s: 1,    o: 1 },
    { t: 38.70, y: 0, s: 1, o: 1 },
    { t: 39.40, y: -10, s: 0.98, o: 0, e: E.inOut } ] });
});

/* group wall — two rows drifting in opposite directions */
const rows = [];
[['a', GROUPS_A, -1], ['b', GROUPS_B, 1]].forEach(([key, list, dir]) => {
  const row = document.querySelector(`.row[data-row="${key}"]`);
  // three copies so the wrap is seamless at any width
  for (let r = 0; r < 3; r++) {
    list.forEach(name => {
      const cls = 'grp' + (HOT.has(name) ? ' hot' : '') + (CJK.test(name) ? ' cjk' : '');
      row.appendChild(el('span', cls, name));
    });
  }
  rows.push({ el: row, dir, unit: 0, speed: dir * 3.4 });
});

/* ── measurement ────────────────────────────────────────────────────────── */
function layoutFrame() {
  const w = film.clientWidth, h = film.clientHeight;
  film.style.setProperty('--fw', w + 'px');
  film.style.setProperty('--fh', h + 'px');
}

function layoutCollapse() {
  const f = keyfield.getBoundingClientRect();
  const cx = f.left + f.width / 2;
  const cy = f.top + f.height / 2;
  keyInners.forEach(c => {
    const r = c.slot.getBoundingClientRect();
    c.collapseKey.x = cx - (r.left + r.width / 2);
    c.collapseKey.y = cy - (r.top + r.height / 2);
  });
}

function layoutRows() {
  rows.forEach(r => { r.unit = r.el.scrollWidth / 3 || 1; });
}

let emPx = 12.8;
function layoutEm() { emPx = parseFloat(getComputedStyle(film).fontSize) || 12.8; }

function layoutAll() { layoutFrame(); layoutEm(); layoutCollapse(); layoutRows(); }

/* ── sampling ───────────────────────────────────────────────────────────── */
const DEF = { x: 0, y: 0, yp: 0, s: 1, sx: 1, sy: 1, r: 0, o: 1, b: 0 };
const PROPS = ['x', 'y', 'yp', 's', 'sx', 'sy', 'r', 'o', 'b'];

function mix(key, a, b, e) {
  const va = a[key] === undefined ? DEF[key] : a[key];
  const vb = b[key] === undefined ? DEF[key] : b[key];
  return va + (vb - va) * e;
}

const out = {};
function sample(keys, t, ease) {
  const n = keys.length;
  let a = keys[0], b = keys[n - 1], e = 1;
  if (t <= a.t) { b = a; }
  else if (t >= b.t) { a = b; }
  else {
    for (let i = 0; i < n - 1; i++) {
      if (t >= keys[i].t && t <= keys[i + 1].t) {
        a = keys[i]; b = keys[i + 1];
        const span = b.t - a.t || 1e-6;
        e = (b.e || ease)((t - a.t) / span);
        break;
      }
    }
  }
  for (let i = 0; i < PROPS.length; i++) {
    const k = PROPS[i];
    out[k] = mix(k, a, b, e);
  }
  return out;
}

/* ── render ─────────────────────────────────────────────────────────────── */
const r2 = v => Math.round(v * 100) / 100;
const r3 = v => Math.round(v * 1000) / 1000;
const clamp = (v, lo, hi) => v < lo ? lo : v > hi ? hi : v;

function apply(v, extraY) {
  const x = v.x;
  const y = v.y + (extraY || 0);
  let tf = '';
  if (x || y) tf += `translate3d(${r2(x)}px,${r2(y)}px,0) `;
  if (v.yp) tf += `translateY(${r2(v.yp)}%) `;
  const sx = v.s * v.sx, sy = v.s * v.sy;
  if (sx !== 1 || sy !== 1) tf += `scale(${r3(sx)},${r3(sy)}) `;
  if (v.r) tf += `rotate(${r2(v.r)}deg)`;
  return tf;
}

function render(t) {
  /* scenes: cross-faded at the seams, hard-hidden when fully out so the
     compositor is not carrying five full-screen layers at once */
  const live = {};
  for (let i = 0; i < SCENES.length; i++) {
    const s = SCENES[i];
    const node = document.getElementById(s.id);
    let o = 1;
    if (t < s.in) o = 0;
    else if (t < s.in + s.fade) o = (t - s.in) / s.fade;
    else if (t > s.out) o = 0;
    else if (t > s.out - s.fade) o = (s.out - t) / s.fade;
    o = clamp(o, 0, 1);
    node.style.opacity = r3(o);
    const on = o > 0.002;
    if (node.classList.contains('on') !== on) node.classList.toggle('on', on);
    live[s.id] = on;
  }

  /* cues */
  for (let i = 0; i < CUES.length; i++) {
    const c = CUES[i];
    if (c.scene && !live[c.scene]) continue;
    const keys = c.keys;
    if (!live[sceneOf(keys[0].t)]) continue;

    let extraY = 0;
    const v = sample(keys, t, c.ease || E.glide);
    if (c.drift) {
      extraY = Math.sin(t * c.drift.freq + c.drift.phase) * c.drift.amp * clamp(v.o, 0, 1);
    }
    const target = c.el || document.querySelector(`[data-cue="${c.cue}"]`);
    if (!target) continue;
    c.el = target;                                   // cache the lookup
    const tf = apply(v, extraY);
    target.style.transform = tf || 'none';
    target.style.opacity = r3(v.o);
  }

  /* marquee rows */
  for (let i = 0; i < rows.length; i++) {
    const r = rows[i];
    const on = live.s4;
    if (!on) { r.el.style.transform = 'none'; continue; }
    const span = r.unit || 1;
    let off = ((t - 39.0) * r.speed * emPx) % span;
    if (off < 0) off += span;
    r.el.style.transform = `translate3d(${r2(-off)}px,0,0)`;
  }

  /* grain: an oversized layer drifting a few px — texture that never sits still */
  const g = document.getElementById('grain');
  g.style.transform = `translate3d(${r2(Math.sin(t * 0.11) * 9)}px,${r2(Math.cos(t * 0.09) * 9)}px,0)`;

  /* the 251 count-up */
  if (live.s4) {
    const p = clamp((t - COUNTER.t0) / (COUNTER.t1 - COUNTER.t0), 0, 1);
    const n = Math.round(COUNTER.from + (COUNTER.to - COUNTER.from) * E.soft(p));
    if (n !== COUNTER.last) { COUNTER.el.textContent = String(n); COUNTER.last = n; }
  }
}

function sceneOf(t) {
  for (let i = 0; i < SCENES.length; i++) {
    const s = SCENES[i];
    if (t >= s.in && t < s.out) return s.id;
  }
  return t >= SCENES[SCENES.length - 1].out ? 's5' : 's1';
}

/* ── clock + transport ──────────────────────────────────────────────────── */
let t = 0;
let playing = true;
let dirty = true;
let last = 0;
let dragging = false;

function setPlaying(v) {
  playing = v;
  if (v && t >= DUR) t = 0;      // pressing play on the last frame replays
  dirty = true;
  if (!v) showHud();
}

function seek(v, quiet) {
  t = clamp(v, 0, DUR);
  dirty = true;
  if (!quiet) showHud();
}

function fmt(s) {
  const m = Math.floor(s / 60);
  const r = s - m * 60;
  return String(m).padStart(2, '0') + ':' + (r < 10 ? '0' : '') + r.toFixed(1);
}

function tick(now) {
  const dt = Math.min((now - last) / 1000, 0.25);   // clamp so a stalled tab
  last = now;                                        // cannot jump the film
  if (playing) {
    t += dt;
    if (t >= DUR) { t = DUR; playing = false; showHud(); }
    dirty = true;
  }
  if (dirty) {
    render(t);
    const p = t / DUR;
    fill.style.transform = `scaleX(${r3(p)})`;
    clockEl.textContent = fmt(t) + ' / ' + fmt(DUR);
    dirty = false;
  }
  requestAnimationFrame(tick);
}

/* ── HUD ────────────────────────────────────────────────────────────────── */
let hudTimer = 0;
function showHud() {
  hud.classList.add('on');
  clearTimeout(hudTimer);
  hudTimer = setTimeout(() => { if (playing) hud.classList.remove('on'); }, 2600);
}

function pointerSeek(clientX) {
  const r = bar.getBoundingClientRect();
  seek(clamp((clientX - r.left) / r.width, 0, 1) * DUR, true);
}

bar.addEventListener('pointerdown', e => {
  dragging = true;
  bar.setPointerCapture(e.pointerId);
  pointerSeek(e.clientX);
});
bar.addEventListener('pointermove', e => { if (dragging) pointerSeek(e.clientX); });
bar.addEventListener('pointerup', e => {
  dragging = false;
  bar.releasePointerCapture(e.pointerId);
  showHud();
});

/* click anywhere on the film toggles playback */
stage.addEventListener('click', e => {
  if (e.target.closest('#hud')) return;
  setPlaying(!playing);
});

/* ── input ──────────────────────────────────────────────────────────────── */
window.addEventListener('keydown', e => {
  const step = e.shiftKey ? 5 : 1;
  switch (e.key) {
    case ' ': e.preventDefault(); setPlaying(!playing); break;
    case 'r': case 'R': t = 0; setPlaying(true); break;
    case 'ArrowLeft':  e.preventDefault(); setPlaying(false); seek(t - step); break;
    case 'ArrowRight': e.preventDefault(); setPlaying(false); seek(t + step); break;
    case 'Home': e.preventDefault(); setPlaying(false); seek(0); break;
    case 'End':  e.preventDefault(); setPlaying(false); seek(DUR); break;
    default: break;
  }
  showHud();
});

window.addEventListener('mousemove', showHud, { passive: true });

let rt = 0;
window.addEventListener('resize', () => {
  clearTimeout(rt);
  rt = setTimeout(() => { layoutAll(); dirty = true; }, 90);
});

/* ── boot ───────────────────────────────────────────────────────────────── */
const params = new URLSearchParams(location.search);
const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;

COUNTER.el = document.getElementById('counter');
layoutAll();
if (document.fonts && document.fonts.ready) {
  document.fonts.ready.then(() => { layoutAll(); dirty = true; });
}
window.addEventListener('load', () => { layoutAll(); dirty = true; });

if (params.get('hud') === '0' || params.get('rec') === '1') {
  document.body.classList.add('nohud');
} else {
  showHud();
}

if (params.has('t')) {
  // Hold a specific frame: ?t=12 parks the film there, paused.
  seek(parseFloat(params.get('t')) || 0, true);
  playing = params.get('play') === '1';
} else if (reduce) {
  // Reduced motion: no animation, just the finished composition.
  seek(59.6, true);
  playing = false;
  hud.classList.add('on');
}

requestAnimationFrame(now => { last = now; tick(now); });

/* Scriptable control, for recording rigs and for holding an exact frame:
     RexPromo.seek(30)      park on 00:30.0
     RexPromo.play() / pause() / restart() / duration */
window.RexPromo = {
  duration: DUR,
  seek: v => { setPlaying(false); seek(v); render(t); dirty = false; },
  play: () => setPlaying(true),
  pause: () => setPlaying(false),
  restart: () => { t = 0; setPlaying(true); },
  time: () => t,
};
})();

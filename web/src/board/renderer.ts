import { createCapture } from './gen/capture.js';
import { createPlace } from './gen/place.js';
import type { GameEvent, GameState } from '../store';
import { createMoveSignal, type Signal } from './moveSignal';

// Timing shared with the server (server/internal/app/game.go): the capture chain starts when
// the capturing stone has separated from the board; each next link starts 120 ms later.
const CAPTURE_START = 0.95; // the stone forms in 0.9 s; the previous last stone has turned to coal / marble
const CAPTURE_STEP = 0.12;
const PLACE_END = 1.0;  // the placement animation is over (the stone forms in 0.9 s)
const GO_OUT = 1.0;     // after the next move the last stone turns into coal / marble (0.9 s) and its seal goes out

type Placed = { idx: number; color: 0 | 1; st: any; placedAt: number };
type Group = { start: number; list: any[]; end: number };

const now = () => performance.now() / 1000;

/** Glyph atlas: 火 on the left half, 冰 on the right half (white on transparent). */
function makeAtlas(): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = 512; c.height = 256;
  const a = c.getContext('2d')!;
  a.fillStyle = '#fff'; a.textAlign = 'center'; a.textBaseline = 'middle';
  a.font = '180px "Yuji Syuku", serif';
  a.fillText('火', 128, 136);
  a.fillText('冰', 384, 136);
  return c;
}

/**
 * Draws the stones on the board with WebGL2: settled stones are cached in an offscreen texture,
 * the stones that are appearing, the last stone's live seal and capture chains are drawn every frame.
 */
export class BoardRenderer {
  private gl: WebGL2RenderingContext;
  private place: any;
  private capture: any;
  private signal: ReturnType<typeof createMoveSignal>;
  private signals: Signal[] = []; // the glow wave of the newest move
  private canvas: HTMLCanvasElement;
  private size = 19;
  private margin = 0;
  private cell = 1;
  private cssW = 1;
  private cssH = 1;
  private dpr = 1;

  private stones = new Map<number, Placed>();
  private live: Placed | null = null;
  private pending: { p: Placed; until: number }[] = [];
  private groups: Group[] = [];
  private gameId = '';
  private staticDirty = true;
  private settledCount = -1;

  private fbo: WebGLFramebuffer | null = null;
  private fboTex: WebGLTexture | null = null;
  private blit: { p: WebGLProgram; vao: WebGLVertexArrayObject; uTex: WebGLUniformLocation | null };
  private raf = 0;

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    const gl = canvas.getContext('webgl2', { premultipliedAlpha: true, antialias: true, alpha: true });
    if (!gl) throw new Error('WebGL2 is not available');
    this.gl = gl;
    const atlas = makeAtlas();
    this.place = createPlace(gl, atlas);
    this.capture = createCapture(gl, atlas);
    this.signal = createMoveSignal(gl);
    this.blit = this.makeBlit();
    const loop = () => { this.frame(); this.raf = requestAnimationFrame(loop); };
    this.raf = requestAnimationFrame(loop);
  }

  destroy() { cancelAnimationFrame(this.raf); }

  private makeBlit() {
    const gl = this.gl;
    const vs = `#version 300 es
      out vec2 uv;
      void main() { vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2); uv = p; gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0); }`;
    const fs = `#version 300 es
      precision mediump float; in vec2 uv; out vec4 o; uniform sampler2D uTex;
      void main() { o = texture(uTex, uv); }`;
    const sh = (t: number, s: string) => { const x = gl.createShader(t)!; gl.shaderSource(x, s); gl.compileShader(x); return x; };
    const p = gl.createProgram()!;
    gl.attachShader(p, sh(gl.VERTEX_SHADER, vs)); gl.attachShader(p, sh(gl.FRAGMENT_SHADER, fs)); gl.linkProgram(p);
    return { p, vao: gl.createVertexArray()!, uTex: gl.getUniformLocation(p, 'uTex') };
  }

  /** Board geometry in CSS pixels: the canvas covers the wooden area; intersections start at `margin`. */
  resize(cssW: number, cssH: number, dpr: number, size: number, margin: number, cell: number) {
    const gl = this.gl;
    this.cssW = cssW; this.cssH = cssH; this.dpr = dpr; this.size = size; this.margin = margin; this.cell = cell;
    this.canvas.width = Math.round(cssW * dpr);
    this.canvas.height = Math.round(cssH * dpr);
    this.place.setSize(cssW, cssH, dpr);
    this.capture.setSize(cssW, cssH, dpr);
    if (this.fboTex) gl.deleteTexture(this.fboTex);
    if (this.fbo) gl.deleteFramebuffer(this.fbo);
    this.fboTex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, this.fboTex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, this.canvas.width, this.canvas.height, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    this.fbo = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, this.fbo);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, this.fboTex, 0);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    // move existing stones to the new geometry
    for (const p of this.stones.values()) this.position(p.st, p.idx);
    for (const q of this.pending) this.position(q.p.st, q.p.idx);
    this.groups = []; // capture chains in flight are dropped on resize
    this.staticDirty = true;
    // a resized canvas is blank until it is drawn again: draw now, before the page is painted,
    // so the stones do not blink while the board grows or shrinks (the side panel animation)
    this.frame();
  }

  private xy(idx: number) { return [idx % this.size, Math.floor(idx / this.size)] as const; }
  private position(st: any, idx: number) {
    const [x, y] = this.xy(idx);
    st.cx = this.margin + x * this.cell;
    st.cy = this.margin + y * this.cell;
    st.r = this.cell * 0.47;
  }

  private newStone(idx: number, color: 0 | 1, seed: number): Placed {
    const [x, y] = this.xy(idx);
    const st = this.place.makeStone(color, this.margin + x * this.cell, this.margin + y * this.cell, this.cell * 0.47, seed);
    return { idx, color, st, placedAt: -1000 };
  }

  /** Apply a server update. Events animate; anything else is drawn as it is. */
  update(g: GameState, e: GameEvent | null) {
    const t = now();
    if (g.id !== this.gameId) {
      this.gameId = g.id;
      this.rebuild(g);
      return;
    }
    if (e && e.kind === 'move') {
      const idx = e.y * g.size + e.x;
      if (this.live) this.live.st.ext = t - this.live.placedAt;
      this.signals.push({ idx, color: e.color === 'b' ? 0 : 1, start: t });
      const p = this.newStone(idx, e.color === 'b' ? 0 : 1, 1.3 + idx * 0.731 + g.moves * 0.173);
      p.placedAt = t;
      this.stones.set(idx, p);
      this.live = p;
      if (e.captured.length) this.startCapture(idx, e.captured, t, g.moves);
      this.staticDirty = true;
    }
    // anything out of sync (reconnect, missed message) is redrawn from the server's board
    if (!this.matches(g.board)) this.rebuild(g);
  }

  /** Runs the glow wave of the last move once more (Shift). */
  replaySignal() {
    const p = this.live;
    if (!p) return;
    this.signals = this.signals.filter((s) => s.idx !== p.idx);
    this.signals.push({ idx: p.idx, color: p.color, start: now() });
  }

  private matches(board: string) {
    const capturing = new Set(this.pending.map((q) => q.p.idx));
    for (let i = 0; i < board.length; i++) {
      const p = this.stones.get(i);
      const want = board[i];
      if (want === '.') { if (p && !capturing.has(i)) return false; continue; }
      if (!p || (p.color === 0 ? 'b' : 'w') !== want) return false;
    }
    return true;
  }

  private rebuild(g: GameState) {
    this.stones.clear();
    this.pending = [];
    this.groups = [];
    this.live = null;
    for (let i = 0; i < g.board.length; i++) {
      const ch = g.board[i];
      if (ch === '.') continue;
      const p = this.newStone(i, ch === 'b' ? 0 : 1, 1.3 + i * 0.731);
      p.st.ext = 0; // settled
      this.stones.set(i, p);
    }
    if (g.last >= 0 && this.stones.has(g.last)) {
      const p = this.stones.get(g.last)!;
      delete p.st.ext; // the last stone keeps its live seal
      this.live = p;
    }
    this.signals = [];
    this.staticDirty = true;
  }

  /** Captured stones turn into a chain: neighbours of the move first, then outward, 120 ms per link. */
  private startCapture(move: number, captured: number[], t: number, moves: number) {
    // moves are not blocked by animations: a stone captured while still forming first
    // finishes forming and turning into coal / marble (0.9 s), then breaks
    let start = t + CAPTURE_START;
    for (const idx of captured) {
      const p = this.stones.get(idx);
      if (p) start = Math.max(start, p.placedAt + Math.max(t - p.placedAt, PLACE_END) + 0.9);
    }
    const set = new Set(captured);
    const depth = new Map<number, number>();
    const from = new Map<number, number>();
    const queue: number[] = [];
    const nb = (i: number) => {
      const [x, y] = this.xy(i); const r: number[] = [];
      if (x > 0) r.push(i - 1); if (x < this.size - 1) r.push(i + 1);
      if (y > 0) r.push(i - this.size); if (y < this.size - 1) r.push(i + this.size);
      return r;
    };
    for (const q of nb(move)) if (set.has(q)) { depth.set(q, 0); from.set(q, move); queue.push(q); }
    while (queue.length) {
      const p = queue.shift()!;
      for (const q of nb(p)) if (set.has(q) && !depth.has(q)) { depth.set(q, depth.get(p)! + 1); from.set(q, p); queue.push(q); }
    }
    const list: any[] = [];
    for (const idx of captured) {
      const p = this.stones.get(idx);
      if (!p) continue;
      this.stones.delete(idx);
      this.pending.push({ p, until: start });
      const [x, y] = this.xy(idx);
      const st = this.capture.makeStone(p.color, this.margin + x * this.cell, this.margin + y * this.cell, this.cell * 0.47, 2.1 + idx * 0.37 + moves * 0.11);
      // the breaking stone must look exactly like the stone it replaces: same glyph angle,
      // same facets, outline and veins/layers (all derived from the stone's seed)
      st.seed = p.st.seed;
      st.glyphRot = p.st.glyphRot;
      st.depth = depth.get(idx) ?? 0;
      const src = from.get(idx) ?? move;
      const [sx, sy] = this.xy(src);
      st.wave = [x - sx || 0.8, y - sy || 0.6];
      list.push(st);
    }
    if (!list.length) return;
    const c = this.capture;
    const end = start + c.windStartOf(list, CAPTURE_STEP) + c.WIND_SWEEP + c.WIND_DUR + 0.2;
    this.groups.push({ start, list, end });
  }

  private frame() {
    const gl = this.gl;
    const t = now();
    // pending captured stones become a capture chain at their start time
    this.pending = this.pending.filter((q) => q.until > t);
    this.groups = this.groups.filter((g) => g.end > t);

    // stones whose placement and go-out are over join the cached static layer
    let settled = 0;
    const dynamic: Placed[] = [];
    for (const p of this.stones.values()) {
      const local = t - p.placedAt;
      const ext = p.st.ext;
      const done = local > PLACE_END && ext !== undefined && local > Math.max(ext, 1.4) + GO_OUT;
      if (done) settled++; else dynamic.push(p);
    }
    if (settled !== this.settledCount) { this.settledCount = settled; this.staticDirty = true; }

    const W = this.canvas.width, H = this.canvas.height;
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);

    if (this.staticDirty && this.fbo) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, this.fbo);
      gl.viewport(0, 0, W, H);
      gl.clearColor(0, 0, 0, 0); gl.clear(gl.COLOR_BUFFER_BIT);
      this.place.bindTex();
      // drawn at their own time: a stone restored on reload (placedAt -1000) may have finished
      // its last-stone state only after t = 1000
      for (const p of this.stones.values()) if (!dynamic.includes(p)) this.place.drawPlacement(p.st, t - p.placedAt);
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      this.staticDirty = false;
    }

    gl.viewport(0, 0, W, H);
    gl.clearColor(0, 0, 0, 0); gl.clear(gl.COLOR_BUFFER_BIT);
    // under the stones: the glow wave of the newest move
    this.signals = this.signal.draw(this.signals, t, { w: this.cssW, h: this.cssH, size: this.size, margin: this.margin, cell: this.cell });

    // cached settled stones
    gl.useProgram(this.blit.p);
    gl.bindVertexArray(this.blit.vao);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, this.fboTex);
    gl.uniform1i(this.blit.uTex, 0);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    this.place.bindTex();
    for (const q of this.pending) this.place.drawPlacement(q.p.st, t - q.p.placedAt);
    for (const p of dynamic) this.place.drawPlacement(p.st, t - p.placedAt);

    if (this.groups.length) {
      const c = this.capture;
      c.bindTex();
      for (const g of this.groups) {
        const rel = t - g.start;
        if (rel < 0) continue;
        for (const st of g.list) {
          const raw = (rel - st.depth * CAPTURE_STEP - st.tJitter) / (c.DURATION * st.durMul);
          c.drawStone(st, Math.min(1, Math.max(0, raw)), 1, raw, c.windOf(st, g.list, rel, CAPTURE_STEP));
        }
      }
    }
  }
}

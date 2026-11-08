import { createEffect, createSignal, For, onCleanup, onMount, Show } from 'solid-js';
import { app, onGameUpdate } from '../store';
import { send } from '../net';
import { BoardRenderer } from './renderer';

const LETTERS = 'ABCDEFGHJKLMNOPQRST';
const HOSHI: Record<number, number[][]> = {
  9: [[2, 2], [2, 6], [6, 2], [6, 6], [4, 4]],
  13: [3, 6, 9].flatMap((a) => [3, 6, 9].map((b) => [a, b])),
  15: [3, 7, 11].flatMap((a) => [3, 7, 11].map((b) => [a, b])),
  19: [3, 9, 15].flatMap((a) => [3, 9, 15].map((b) => [a, b])),
};
const GUTTER = 24; // frame strip with coordinates

/** The board: wooden surface with grid (2D canvas), stones and effects (WebGL), hover and scoring marks (2D). */
export function Board(props: { blurred: boolean }) {
  let host!: HTMLDivElement;
  let gridC!: HTMLCanvasElement;
  let glC!: HTMLCanvasElement;
  let uiC!: HTMLCanvasElement;
  let renderer: BoardRenderer | null = null;
  const [px, setPx] = createSignal(600); // wooden area size, CSS px
  const [hover, setHover] = createSignal(-1);
  const [sent, setSent] = createSignal(-1); // the point of a move sent to the server, until it answers
  const [error, setError] = createSignal('');
  const size = () => app.game?.size ?? 19;
  const geom = () => {
    const n = size();
    const cell = px() / (n - 1 + 2 * 0.72);
    return { n, cell, margin: cell * 0.72 };
  };

  const layout = () => {
    const r = host.getBoundingClientRect();
    const side = Math.max(240, Math.floor(Math.min(r.width, r.height)) - GUTTER * 2);
    setPx(side);
  };

  const drawGrid = () => {
    const { n, cell, margin } = geom();
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const s = px();
    for (const c of [gridC, uiC]) { c.width = Math.round(s * dpr); c.height = Math.round(s * dpr); }
    const ctx = gridC.getContext('2d')!;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, s, s);
    ctx.strokeStyle = 'rgba(58, 40, 18, .85)';
    for (let i = 0; i < n; i++) {
      const w = i === 0 || i === n - 1 ? 2.5 : 1.5;
      ctx.lineWidth = w;
      const o = margin + i * cell;
      ctx.beginPath(); ctx.moveTo(margin, o); ctx.lineTo(margin + (n - 1) * cell, o); ctx.stroke();
      ctx.beginPath(); ctx.moveTo(o, margin); ctx.lineTo(o, margin + (n - 1) * cell); ctx.stroke();
    }
    ctx.fillStyle = 'rgba(58, 40, 18, .9)';
    for (const [x, y] of HOSHI[n] ?? []) {
      ctx.beginPath(); ctx.arc(margin + x * cell, margin + y * cell, Math.max(2.5, cell * 0.1) + 0.3, 0, Math.PI * 2); ctx.fill();
    }
    renderer?.resize(s, s, dpr, n, margin, cell);
  };

  // the hover outline and the marks of the counting phase
  const drawUi = () => {
    const g = app.game;
    const { cell, margin } = geom();
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const ctx = uiC.getContext('2d')!;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, px(), px());
    if (!g) return;
    const at = (i: number) => [margin + (i % g.size) * cell, margin + Math.floor(i / g.size) * cell];
    if (g.phase === 'scoring' || g.phase === 'result') {
      for (const i of g.dead ?? []) {
        const [x, y] = at(i);
        ctx.fillStyle = 'rgba(214, 170, 104, .62)';
        ctx.beginPath(); ctx.arc(x, y, cell * 0.47, 0, Math.PI * 2); ctx.fill();
      }
      // territory: small translucent discs in the colour of the owner's stones (coal / marble)
      const tr = cell * 0.24;
      for (const [side, fill] of [['b', 'rgba(20, 20, 23, .55)'], ['w', 'rgba(242, 240, 234, .72)']] as const) {
        ctx.fillStyle = fill;
        for (const i of g.territory?.[side] ?? []) {
          const [x, y] = at(i);
          ctx.beginPath(); ctx.arc(x, y, tr, 0, Math.PI * 2); ctx.fill();
        }
      }
    }
    const h = hover();
    if (h >= 0 && h !== sent() && canPlay() && g.board[h] === '.') {
      const [x, y] = at(h);
      const black = g.you === 'b';
      ctx.lineWidth = 2;
      ctx.strokeStyle = black ? 'rgba(12, 12, 14, .9)' : 'rgba(255, 255, 255, .95)';
      ctx.fillStyle = black ? 'rgba(12, 12, 14, .16)' : 'rgba(255, 255, 255, .24)';
      ctx.beginPath(); ctx.arc(x, y, cell * 0.45, 0, Math.PI * 2); ctx.fill(); ctx.stroke();
    }
  };

  const canPlay = () => {
    const g = app.game;
    return !!g && g.phase === 'play' && g.you === g.turn && !g.pauseReq;
  };

  const pointToIndex = (ev: PointerEvent) => {
    const g = app.game;
    if (!g) return -1;
    const r = uiC.getBoundingClientRect();
    const { cell, margin } = geom();
    const x = Math.round((ev.clientX - r.left - margin) / cell);
    const y = Math.round((ev.clientY - r.top - margin) / cell);
    if (x < 0 || y < 0 || x >= g.size || y >= g.size) return -1;
    const cx = margin + x * cell, cy = margin + y * cell;
    if (Math.hypot(ev.clientX - r.left - cx, ev.clientY - r.top - cy) > cell * 0.5) return -1;
    return y * g.size + x;
  };

  onMount(async () => {
    try { await document.fonts.load('180px "Yuji Syuku"', '火冰'); } catch { /* fall back to a serif */ }
    try {
      renderer = new BoardRenderer(glC);
    } catch (e) {
      setError('Ваш браузер не поддерживает WebGL2 — доска не может быть показана.');
      return;
    }
    const ro = new ResizeObserver(() => { layout(); drawGrid(); drawUi(); });
    ro.observe(host);
    layout(); drawGrid();
    if (app.game) renderer.update(app.game, null);
    const off = onGameUpdate((g, e) => { renderer?.update(g, e); setSent(-1); drawUi(); });
    onCleanup(() => { ro.disconnect(); off(); renderer?.destroy(); });
  });

  createEffect(() => { hover(); app.game?.phase; app.game?.seq; drawUi(); });
  createEffect(() => { size(); if (renderer) drawGrid(); });

  const cursor = () => (canPlay() ? 'pointer' : 'default');

  return (
    <div ref={host} class="board-host">
      <div class="frame" style={{ width: `${px() + GUTTER * 2}px`, height: `${px() + GUTTER * 2}px` }}>
        <For each={Array.from({ length: size() }, (_, i) => i)}>
          {(i) => {
            const pos = () => GUTTER + geom().margin + i * geom().cell;
            return (
              <>
                <span class="coord" style={{ left: `${pos() - 12}px`, top: '0px' }}>{LETTERS[i]}</span>
                <span class="coord" style={{ left: `${pos() - 12}px`, bottom: '0px' }}>{LETTERS[i]}</span>
                <span class="coord" style={{ top: `${pos() - 12}px`, left: '0px' }}>{size() - i}</span>
                <span class="coord" style={{ top: `${pos() - 12}px`, right: '0px' }}>{size() - i}</span>
              </>
            );
          }}
        </For>
        <div class="wood" classList={{ blurred: props.blurred }} style={{ left: `${GUTTER}px`, top: `${GUTTER}px`, width: `${px()}px`, height: `${px()}px` }}>
          <canvas ref={gridC} class="layer" />
          <canvas ref={glC} class="layer" />
          <canvas
            ref={uiC}
            class="layer"
            style={{ cursor: cursor() }}
            onPointerMove={(ev) => setHover(pointToIndex(ev))}
            onPointerLeave={() => setHover(-1)}
            onClick={(ev) => {
              const i = pointToIndex(ev as unknown as PointerEvent);
              const g = app.game;
              if (i < 0 || !g || !canPlay() || g.board[i] !== '.') return;
              send({ t: 'move', x: i % g.size, y: Math.floor(i / g.size) });
              // keep the hovered point: the cursor stays hidden over the stone that now appears there
              setSent(i);
              setTimeout(() => setSent((s) => (s === i ? -1 : s)), 1500); // a refused move frees the point
            }}
          />
        </div>
        <Show when={error()}><div class="board-error">{error()}</div></Show>
      </div>
    </div>
  );
}

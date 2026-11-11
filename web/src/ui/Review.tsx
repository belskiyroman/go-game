import { createEffect, createMemo, createSignal, on, onCleanup, Show, type JSX } from 'solid-js';
import { app, komiLabel, type GameEvent, type GameState, type HistoryGame, type Move3, type Side } from '../store';
import { Goban, replay } from '../replay';
import { Board } from '../board/Board';
import { Topbar } from './Lobby';
import { dateTime, mySide, PairLine, stone } from './History';

type Listener = (g: GameState, e: GameEvent | null) => void;
/** Feeds one board: the position after each change, with the move to animate when there is one. */
function bus() {
  const ls = new Set<Listener>();
  return {
    subscribe: (fn: Listener) => { ls.add(fn); return () => { ls.delete(fn); }; },
    emit: (g: GameState, e: GameEvent | null) => ls.forEach((fn) => fn(g, e)),
  };
}

const num = (v: number) => String(v).replace('.', ',');
/** Who moves after n moves of the sequence. */
const turnAfter = (seq: Move3[], n: number): Side => (n === 0 ? 'b' : seq[n - 1][0] === 1 ? 'w' : 'b');

/** A history game drawn as a game state, so the board can show it. */
function state(id: string, r: HistoryGame, seq: Move3[], n: number): GameState {
  const p = replay(r.size, seq, n);
  return {
    id, seq: n, size: r.size, komi: r.komi, moveTime: r.moveTime, you: '',
    black: { id: '', nick: r.black, tag: '' }, white: { id: '', nick: r.white, tag: '' },
    board: p.board, turn: turnAfter(seq, n), last: p.last, passes: 0, caps: p.caps,
    phase: 'play', pauseReq: '', ready: { b: false, w: false }, agree: { b: false, w: false },
    serverNow: 0, deadline: 0, remaining: 0, moves: n, played: 0, playedRunning: false,
    away: '', parent: '', from: 0, alts: 0, mainOpen: false,
  };
}

/** The move from n-1 to n as an event for the renderer (a stone appears, captured stones break). */
function moveEvent(size: number, seq: Move3[], n: number): GameEvent | null {
  const [c, x, y] = seq[n - 1];
  if (x < 0) return null;
  const b = new Goban(size);
  for (const m of seq.slice(0, n - 1)) b.play(...m);
  const captured = b.play(c, x, y) ?? [];
  return { kind: 'move', color: c === 1 ? 'b' : 'w', x, y, captured };
}

/** Sends a board its position whenever it changes; one step forward is played as a move. */
function drive(feed: ReturnType<typeof bus>, view: () => { g: GameState; seq: Move3[]; n: number } | null) {
  let prev = -1, prevId = '';
  createEffect(() => {
    const v = view();
    if (!v) return;
    const e = v.g.id === prevId && v.n === prev + 1 ? moveEvent(v.g.size, v.seq, v.n) : null;
    prev = v.n; prevId = v.g.id;
    feed.emit(v.g, e);
  });
}

function Caps(props: { g: GameState }) {
  return (
    <span class="caps"><span class="muted">Пленные</span>{stone('b')}<strong>{props.g.caps.b}</strong>{stone('w')}<strong>{props.g.caps.w}</strong></span>
  );
}

function Stepper(props: { min: number; max: number; value: number; set: (n: number) => void; label: string }) {
  return (
    <>
      <input class="slider" type="range" min={props.min} max={props.max} value={props.value} aria-label={props.label} onInput={(e) => props.set(Number(e.currentTarget.value))} />
      <div class="grid3">
        <button class="btn small" disabled={props.value <= props.min} onClick={() => props.set(props.value - 1)}>◀ Ход</button>
        <button class="btn small" disabled={props.value >= props.max} onClick={() => props.set(props.value + 1)}>Ход ▶</button>
        <button class="btn small" disabled={props.value >= props.max} onClick={() => props.set(props.max)}>Последний</button>
      </div>
    </>
  );
}

/** Looking through a finished game of the history, with a local alternative play on a second board. */
export function Review() {
  const rec = () => app.history?.find((h) => h.id === app.review?.id) ?? null;
  const moves = () => app.review?.moves ?? null;
  const total = () => moves()?.length ?? 0;

  const [step, setStep] = createSignal(0);
  createEffect(on(moves, (m) => setStep(m?.length ?? 0)));

  // the alternative play: a start move and the player's own moves after it; aStep of them are shown
  const [alt, setAlt] = createSignal<{ start: number; moves: Move3[] } | null>(null);
  const [aStep, setAStep] = createSignal(0);
  const altSeq = () => { const a = alt(), m = moves(); return a && m ? [...m.slice(0, a.start), ...a.moves] : []; };

  // phones: one board at a time
  const mq = window.matchMedia('(max-width: 640px)');
  const [narrow, setNarrow] = createSignal(mq.matches);
  const onMq = () => setNarrow(mq.matches);
  mq.addEventListener('change', onMq);
  onCleanup(() => mq.removeEventListener('change', onMq));
  const [tab, setTab] = createSignal<'game' | 'alt'>('alt');

  const mainView = createMemo(() => {
    const r = rec(), m = moves();
    return r && m ? { g: state(`rev-${r.id}`, r, m, step()), seq: m, n: step() } : null;
  });
  const altView = createMemo(() => {
    const r = rec(), a = alt();
    if (!r || !a) return null;
    const seq = altSeq(), n = a.start + aStep();
    return { g: state(`alt-${r.id}-${a.start}`, r, seq, n), seq, n };
  });
  const mainBus = bus(), altBus = bus();
  drive(mainBus, mainView);
  drive(altBus, altView);

  const startAlt = () => { setAlt({ start: step(), moves: [] }); setAStep(0); setTab('alt'); };

  // a move on the alternative board: the stone of the side to move; the moves after the shown one go
  const play = (i: number) => {
    const r = rec(), a = alt();
    if (!r || !a) return;
    const seq = altSeq(), n = a.start + aStep();
    const b = new Goban(r.size);
    for (const m of seq.slice(0, n)) b.play(...m);
    const c = turnAfter(seq, n) === 'b' ? 1 : 2;
    const x = i % r.size, y = Math.floor(i / r.size);
    if (b.play(c, x, y) === null) return; // occupied or suicide
    if (n >= 1 && b.board() === replay(r.size, seq, n - 1).board) return; // ko: the position may not repeat
    const own: Move3[] = [...a.moves.slice(0, aStep()), [c, x, y]];
    setAlt({ start: a.start, moves: own });
    setAStep(own.length);
    setStep(Math.min(a.start + own.length, total())); // the game's slider follows the alternative
  };

  const downloadSgf = () => {
    const r = rec();
    const blob = new Blob([app.review?.sgf ?? ''], { type: 'application/x-go-sgf' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `go-${r?.black}-vs-${r?.white}.sgf`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  };

  const opp = () => { const r = rec()!; return mySide(r) === 'b' ? r.white : r.black; };
  const verdict = () => {
    const r = rec()!;
    if (!r.result) return '';
    const won = r.result.winner === mySide(r);
    if (r.result.reason === 'resign') return won ? 'Победа: соперник сдался' : 'Поражение: вы сдались';
    return `${won ? 'Победа' : 'Поражение'} на ${num(r.result.diff)} очка`;
  };

  const gamePanel = (): JSX.Element => (
    <div class="box">
      <div class="between"><Caps g={mainView()!.g} /><span class="step">{step()} из {total()}</span></div>
      <Stepper min={0} max={total()} value={step()} set={setStep} label="Ход партии" />
      <Show when={!alt()}>
        <button class="btn primary" onClick={startAlt}>Альтернативный розыгрыш с хода {step()}</button>
      </Show>
    </div>
  );
  const altPanel = (): JSX.Element => {
    const a = alt()!;
    return (
      <div class="box">
        <div class="between"><Caps g={altView()!.g} /><span class="step">{a.start + aStep()} · ходы {a.start}–{a.start + a.moves.length}</span></div>
        <Stepper min={a.start} max={a.start + a.moves.length} value={a.start + aStep()} set={(n) => setAStep(n - a.start)} label="Ход розыгрыша" />
        <div class="between"><span class="note">Ваших ходов: {aStep()}</span><span class="row-gap" style={{ gap: '6px' }}>Ходят {stone(altView()!.g.turn)}</span></div>
      </div>
    );
  };
  const gameBoard = () => <div class="rv-board"><Board blurred={false} src={() => mainView()?.g ?? null} subscribe={mainBus.subscribe} /></div>;
  const altBoard = () => <div class="rv-board"><Board blurred={false} src={() => altView()?.g ?? null} subscribe={altBus.subscribe} onPoint={play} /></div>;
  const altHead = () => (
    <div class="between rv-label">
      <span class="label" style={{ color: 'var(--accent)' }}>Альтернативный розыгрыш · с хода {alt()!.start}</span>
      <button class="btn small rv-close" title="Закрыть розыгрыш" aria-label="Закрыть розыгрыш" onClick={() => setAlt(null)}>✕</button>
    </div>
  );

  return (
    <div>
      <Topbar center={<Show when={rec()}>{(r) => <PairLine h={r()} />}</Show>} who={<span />} />
      <Show when={rec() && mainView()} fallback={<p class="muted" style={{ padding: '24px 16px', margin: 0 }}>Загружаем партию…</p>}>
        <Show
          when={alt()}
          fallback={
            <main class="review">
              <div class="rv-main">{gameBoard()}</div>
              <aside class="side">
                <div class="box">
                  <div class="between"><span class="label">Партия с {opp()}</span><span class="chip done">Закончена</span></div>
                  <span class="note">{dateTime(rec()!.started)} · {rec()!.size}×{rec()!.size} · коми {komiLabel(rec()!.komi)}<Show when={verdict()}><br />{verdict()}</Show></span>
                </div>
                {gamePanel()}
                <button class="btn small" style={{ 'align-self': 'flex-start' }} onClick={downloadSgf}>Скачать SGF</button>
              </aside>
            </main>
          }
        >
          <Show
            when={!narrow()}
            fallback={
              <main class="review one">
                <div class="seg" style={{ 'grid-template-columns': '1fr 1fr' }}>
                  <button classList={{ on: tab() === 'game' }} onClick={() => setTab('game')}>Партия · {step()}</button>
                  <button classList={{ on: tab() === 'alt' }} onClick={() => setTab('alt')}>Розыгрыш · {alt()!.start + aStep()}</button>
                </div>
                <Show when={tab() === 'alt'} fallback={<>{gameBoard()}{gamePanel()}</>}>
                  {altHead()}{altBoard()}{altPanel()}
                </Show>
              </main>
            }
          >
            <main class="review two">
              <div class="rv-col">
                <div class="between rv-label"><span class="label">Партия</span></div>
                {gameBoard()}
                {gamePanel()}
              </div>
              <div class="rv-col">
                {altHead()}
                {altBoard()}
                {altPanel()}
              </div>
            </main>
          </Show>
        </Show>
      </Show>
    </div>
  );
}

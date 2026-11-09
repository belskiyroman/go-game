import { createEffect, createMemo, createSignal, For, on, onCleanup, Show } from 'solid-js';
import { app, komiLabel, onGameUpdate, serverNow, timeLabel, type Side } from '../store';
import { send } from '../net';
import { Board, type BoardView } from '../board/Board';
import { replay } from '../replay';
import { Topbar } from './Lobby';
import { movesWord } from './MiniBoard';

const fmt = (ms: number) => {
  const s = Math.max(0, Math.ceil(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
};
const num = (v: number) => String(v).replace('.', ',');
// game time: 12:34 or 1:02:03
const fmtLong = (ms: number) => {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = String(s % 60).padStart(2, '0');
  return h ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${String(m).padStart(2, '0')}:${sec}`;
};
const sideName = (s: Side) => (s === 'b' ? 'чёрных' : 'белых');

export function Game() {
  const [now, setNow] = createSignal(serverNow());
  const iv = setInterval(() => setNow(serverNow()), 200);
  onCleanup(() => clearInterval(iv));

  // the opponent's pass is announced in the middle of the board for 2 s
  const [passNote, setPassNote] = createSignal<{ s: Side; timeout: boolean } | null>(null);
  let passTimer: number | undefined;
  const offPass = onGameUpdate((st, e) => {
    if (e?.kind !== 'pass' || e.color === st.you) return;
    setPassNote({ s: e.color, timeout: !!e.timeout });
    clearTimeout(passTimer);
    passTimer = window.setTimeout(() => setPassNote(null), 2000);
  });
  onCleanup(() => { offPass(); clearTimeout(passTimer); });

  // "Игра началась": both players see it for 2 s when an alternative game starts
  const [startNote, setStartNote] = createSignal(false);
  let startTimer: number | undefined;
  const offStart = onGameUpdate((_, e) => {
    if (e?.kind !== 'started') return;
    setStartNote(true);
    clearTimeout(startTimer);
    startTimer = window.setTimeout(() => setStartNote(false), 2000);
  });
  onCleanup(() => { offStart(); clearTimeout(startTimer); });

  const g = () => app.game!;
  const me = () => g().you as Side;
  const opp = () => (me() === 'b' ? 'w' : 'b') as Side;
  const nickOf = (s: Side) => (s === 'b' ? g().black.nick : g().white.nick);
  const moveMs = () => g().moveTime * 1000;

  // time left for the player to move; it runs from the opponent's move, animations included
  const timeLeft = (s: Side) => {
    const st = g();
    if (st.phase === 'paused' && st.turn === s) return st.remaining;
    if ((st.phase === 'play' && st.turn === s) || st.phase === 'scoring') return st.deadline - now();
    return moveMs();
  };
  const active = (s: Side) => g().phase === 'play' && g().turn === s;
  const played = () => (g().played ?? 0) + (g().playedRunning ? now() - g().serverNow : 0);

  // a player as on the board: the stone and the nickname
  const Who = (p: { s: Side; size?: number }) => (
    <span class="who">
      <span class={`mini ${p.s}`} style={p.size ? { width: `${p.size}px`, height: `${p.size}px`, 'font-size': `${Math.round(p.size * 0.56)}px` } : undefined}>{p.s === 'b' ? '火' : '冰'}</span>
      <span class="who-nick">{nickOf(p.s)}</span>
    </span>
  );
  const downloadSgf = () => {
    const blob = new Blob([g().sgf ?? ''], { type: 'application/x-go-sgf' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `go-${g().black.nick}-vs-${g().white.nick}.sgf`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  };
  // --- an alternative game before its start: each player picks the start move on their own slider
  const src = () => g().source ?? [];
  const [pick, setPick] = createSignal<number | null>(null); // null: the last move
  const [swap, setSwap] = createSignal(false);
  createEffect(on(() => g().id, () => { setPick(null); setSwap(false); }));
  const proposal = () => (g().phase === 'setup' ? g().proposal ?? null : null);
  const mine = () => proposal()?.from === me();
  const theirs = () => !!proposal() && proposal()!.from !== me();
  const myStep = () => Math.min(pick() ?? src().length, src().length);
  const shownStep = () => proposal()?.step ?? myStep(); // an offer (own or the opponent's) is shown at once
  const turnAfter = (n: number): Side => (n === 0 ? 'b' : src()[n - 1][0] === 1 ? 'w' : 'b');
  const stepLine = (n: number) => `${n === 0 ? 'Начало партии' : `Ход ${n} из ${src().length}`} · ходят ${turnAfter(n) === 'b' ? 'чёрные' : 'белые'}`;
  const view = createMemo<BoardView | null>(() => {
    if (g().phase !== 'setup') return null;
    const n = shownStep();
    return { ...replay(g().size, src(), n), key: `setup-${n}` };
  });
  const stoneOf = (s: Side) => (s === 'b' ? 'чёрные 火' : 'белые 冰');

  // requests to the opponent
  const REQ_MINE = {
    alt: 'Вы предложили альтернативный розыгрыш', adjourn: 'Вы предложили отложить партию', close: 'Вы предложили закрыть партию без подсчёта',
    back: 'Вы предложили вернуться в основную партию', promote: 'Вы предложили сделать эту партию основной',
  };
  const REQ_LABEL = { alt: 'Альтернативный розыгрыш', adjourn: 'Отложить партию', close: 'Закрыть партию', back: 'Вернуться в основную партию', promote: 'Сделать основной партией' };
  const REQ_ASK = { alt: 'альтернативный розыгрыш', adjourn: 'отложить партию', close: 'закрыть партию без подсчёта', back: 'вернуться в основную партию', promote: 'сделать эту партию основной' };
  const myReq = () => (g().req && g().req!.from === me() ? g().req! : null);
  const theirReq = () => (g().req && g().req!.from !== me() ? g().req! : null);
  const ongoing = () => ['play', 'paused', 'scoring', 'setup'].includes(g().phase);

  const rematchPending = () => app.lobby?.outgoing && (app.lobby.outgoing.to === g().black.id || app.lobby.outgoing.to === g().white.id);

  return (
    <div>
      <Topbar
        info={`${g().size}×${g().size} · коми ${komiLabel(g().komi)} · ${timeLabel(g().moveTime)} на ход`}
        who={
          <span class="h2h-head" role="img" aria-label={`Счёт встреч: ${nickOf('b')} ${g().h2h?.b ?? 0}, ${nickOf('w')} ${g().h2h?.w ?? 0}`}>
            <span class="h2h-nick">{nickOf('b')}</span>
            <span class="mini b" aria-hidden="true">火</span>
            <strong>{g().h2h?.b ?? 0} : {g().h2h?.w ?? 0}</strong>
            <span class="mini w" aria-hidden="true">冰</span>
            <span class="h2h-nick">{nickOf('w')}</span>
          </span>
        }
      />
      <main class="game">
        <div class="board-col">
          <Board blurred={g().phase === 'paused'} view={view()} offer={!!proposal()} />
          <Show when={startNote()}>
            <div class="pass-note" role="status">Игра началась</div>
          </Show>
          <Show when={passNote()}>
            {(n) => (
              <div class="pass-note" role="status">
                <span class={`mini ${n().s}`} style={{ width: '30px', height: '30px', 'font-size': '17px' }}>{n().s === 'b' ? '火' : '冰'}</span>
                <span>Пас{n().timeout ? ' · время вышло' : ''}</span>
              </div>
            )}
          </Show>
          <Show when={g().phase === 'paused'}>
            <div class="pause-overlay">
              <h2>Игра на паузе</h2>
              <p style={{ margin: 0, 'max-width': '360px', 'line-height': 1.5, 'text-shadow': '0 1px 8px rgba(0,0,0,.6)' }}>Время на ход остановлено. Игра продолжится, когда оба игрока будут готовы.</p>
            </div>
          </Show>
        </div>

        <aside class="side">
          <Show when={g().parent && g().phase !== 'setup'}>
            <div class="altline"><span class="chip alt">Альтернативный розыгрыш</span><span class="muted">с хода {g().from}</span></div>
          </Show>
          <Show when={g().phase !== 'setup'}>
          <div class="gameclock" aria-label="Время партии без пауз">
            <span class="muted">Время партии</span>
            <strong>{fmtLong(played())}</strong>
          </div>
          {/* the player cards (move clocks, prisoners) are hidden while counting and on the result */}
          <For each={g().phase === 'scoring' || g().phase === 'result' ? [] : (['b', 'w'] as Side[])}>
            {(s) => (
              <section class="player" classList={{ active: active(s) }} aria-label={`${nickOf(s)}, ${s === 'b' ? 'чёрные' : 'белые'}`}>
                <div style={{ display: 'flex', 'align-items': 'center', gap: '14px' }}>
                  <div style={{ 'text-align': 'left', 'flex': '0 0 auto' }}>
                    <div style={{ 'font-size': '26px', 'font-weight': 800, 'font-variant-numeric': 'tabular-nums', color: active(s) ? (timeLeft(s) <= 30000 ? 'var(--danger)' : 'var(--text)') : '#8f9397' }}>{fmt(timeLeft(s))}</div>
                    <div class="muted" style={{ 'font-size': '12px' }}>{g().phase === 'paused' && g().turn === s ? 'остановлено' : 'на ход'}</div>
                  </div>
                  <div style={{ flex: '1 1 auto', 'min-width': 0, 'text-align': 'right' }}>
                    <div style={{ 'font-size': '17px', 'font-weight': 800, overflow: 'hidden', 'text-overflow': 'ellipsis', 'white-space': 'nowrap' }}>{nickOf(s)}</div>
                    <div class="muted" style={{ 'font-size': '13px' }}>{s === 'b' ? 'Чёрные' : 'Белые'}{s === me() ? ' · вы' : ''}</div>
                  </div>
                  <span class={`mini ${s}`} style={{ width: '34px', height: '34px', 'font-size': '19px', 'flex': '0 0 auto' }}>{s === 'b' ? '火' : '冰'}</span>
                </div>
                <div class="timerbar" classList={{ low: active(s) && timeLeft(s) <= 30000 }}><div style={{ width: `${Math.max(0, Math.min(100, (timeLeft(s) / moveMs()) * 100))}%` }} /></div>
                <div style={{ display: 'flex', 'justify-content': 'space-between', 'font-size': '14px' }} class="muted">
                  <span>Пленные: <strong style={{ color: 'var(--text)', 'font-size': '16px' }}>{g().caps[s]}</strong></span>
                  <Show when={g().phase === 'paused'}>
                    <span style={{ color: g().ready[s] ? 'var(--ok)' : '#c9c3ba', 'font-weight': 700 }}>{g().ready[s] ? 'Готов продолжить' : 'Не готов'}</span>
                  </Show>
                </div>
              </section>
            )}
          </For>
          </Show>

          <Show when={g().phase === 'play'}>
            {/* whose move it is shows on the player cards; only the warning after a pass stays */}
            <Show when={g().passes === 1}>
              <div style={{ padding: '12px 14px', 'border-radius': '12px', background: '#1f2225', border: '1px solid #33383d', color: '#c9c3ba' }}>
                Пас {sideName(g().turn === 'b' ? 'w' : 'b')}. Второй пас подряд завершит партию
              </div>
            </Show>
            <Show when={g().pauseReq === me()}>
              <div class="banner">
                <div style={{ display: 'flex', 'align-items': 'center', gap: '10px' }}><div class="spinner" /><strong>Ждём ответа на паузу</strong></div>
                <span style={{ color: '#c9c3ba', 'font-size': '14px' }}>Пока соперник не согласился, время на ход идёт.</span>
                <button class="btn" onClick={() => send({ t: 'pauseCancel' })}>Отменить запрос</button>
              </div>
            </Show>
            <div class="grid3">
              <button class="btn" disabled={g().turn !== me()} onClick={() => send({ t: 'pass' })}>Пас</button>
              <button class="btn" disabled={!!g().pauseReq} onClick={() => send({ t: 'pauseRequest' })}>Пауза</button>
              <button class="btn danger" onClick={() => { if (confirm('Сдаться? Партия закончится вашим поражением.')) send({ t: 'resign' }); }}>Сдаться</button>
            </div>
          </Show>

          <Show when={g().phase === 'paused'}>
            <div class="banner">
              <strong style={{ 'font-size': '17px' }}>Пауза</strong>
              <span style={{ color: '#c9c3ba', 'font-size': '14px', 'line-height': 1.5 }}>Отметьте, когда будете готовы. Статус можно вернуть обратно в любой момент.</span>
              <button class="toggle" classList={{ on: g().ready[me()] }} aria-pressed={g().ready[me()]} onClick={() => send({ t: 'ready', ready: !g().ready[me()] })}>
                <span class="knob" />{g().ready[me()] ? 'Готов продолжить' : 'Не готов'}
              </button>
            </div>
          </Show>

          <Show when={g().phase === 'scoring'}>
            <div class="banner">
              <strong style={{ 'font-size': '17px' }}>Подсчёт очков</strong>
              <span style={{ color: '#c9c3ba', 'font-size': '14px', 'line-height': 1.5 }}>Сервер отметил мёртвые камни (притушены) и территорию (кружки). Нажмите на камень, чтобы отметить его группу мёртвой или живой, — территория пересчитается. Нажмите на пустой пункт, чтобы сделать его область нейтральной (крестики) или вернуть как посчитано. После любого изменения оба подтверждают заново. Очки покажем, когда согласятся оба.</span>
              <div class="grid2">
                <button class="btn primary" disabled={g().agree[me()]} onClick={() => send({ t: 'scoreAgree' })}>{g().agree[me()] ? 'Вы согласны' : 'Согласен'}</button>
                <button class="btn" onClick={() => send({ t: 'playOn' })}>Доиграть</button>
              </div>
              <span class="muted" style={{ 'font-size': '14px' }}>{g().agree[opp()] ? `${nickOf(opp())} согласен` : `${nickOf(opp())} ещё не выбрал`} · осталось {fmt(g().deadline - now())}</span>
            </div>
          </Show>

          <Show when={g().phase === 'setup'}>
            <Show
              when={theirs()}
              fallback={
                <div class="box">
                  <div class="altline"><span class="chip alt">Альтернативный розыгрыш</span><span class="muted">от партии из {movesWord(src().length)}</span></div>
                  <div class="between"><span class="label">Точка старта</span><strong class="stepline">{stepLine(shownStep())}</strong></div>
                  <input class="slider" type="range" min="0" max={src().length} value={shownStep()} disabled={mine()} aria-label="Ход, с которого начать" onInput={(e) => setPick(Number(e.currentTarget.value))} />
                  <div class="grid3">
                    <button class="btn small" disabled={mine() || myStep() === 0} onClick={() => setPick(myStep() - 1)}>◀ Ход</button>
                    <button class="btn small" disabled={mine() || myStep() >= src().length} onClick={() => setPick(myStep() + 1)}>Ход ▶</button>
                    <button class="btn small" disabled={mine() || myStep() === src().length} onClick={() => setPick(null)}>Последний</button>
                  </div>
                  <label class="check"><input type="checkbox" checked={mine() ? proposal()!.swap : swap()} disabled={mine()} onChange={(e) => setSwap(e.currentTarget.checked)} /> Предложить поменяться цветами</label>
                  <Show
                    when={mine()}
                    fallback={<button class="btn primary" onClick={() => send({ t: 'propose', step: myStep(), swap: swap() })}>{myStep() === 0 ? 'Предложить начать с начала' : `Предложить начать с хода ${myStep()}`}</button>}
                  >
                    <div class="waiting"><div class="spinner" />Ждём ответа {nickOf(opp())}…</div>
                    <button class="btn" onClick={() => send({ t: 'proposeCancel' })}>Отозвать предложение</button>
                  </Show>
                </div>
              }
            >
              <div class="box proposal">
                <span class="label">Предложение соперника</span>
                <strong style={{ 'font-size': '17px' }}>{stepLine(proposal()!.step)}</strong>
                <span class="note">
                  {nickOf(opp())} предлагает начать отсюда{proposal()!.swap ? ' и поменяться цветами' : ''}: вы — {stoneOf(proposal()!.swap ? opp() : me())}, {nickOf(opp())} — {stoneOf(proposal()!.swap ? me() : opp())}.
                </span>
                <div class="grid2">
                  <button class="btn primary" onClick={() => send({ t: 'proposeAnswer', accept: true })}>Принять</button>
                  <button class="btn" onClick={() => send({ t: 'proposeAnswer', accept: false })}>Отказаться</button>
                </div>
              </div>
            </Show>
          </Show>

          <Show when={ongoing()}>
            <Show
              when={myReq()}
              fallback={
                <div style={{ display: 'flex', 'flex-direction': 'column', gap: '8px' }}>
                  <Show
                    when={g().parent}
                    fallback={
                      <Show when={g().moves > 0}>
                        <button class="btn small" disabled={!!g().req || g().alts >= 5} onClick={() => send({ t: 'ask', kind: 'alt' })}>Альтернативный розыгрыш</button>
                        <Show when={g().alts >= 5}><span class="muted" style={{ 'font-size': '13px' }}>У партии уже 5 альтернативных розыгрышей: закройте один из них, чтобы начать новый.</span></Show>
                      </Show>
                    }
                  >
                    <Show when={g().mainOpen}>
                      <button class="btn small" disabled={!!g().req} onClick={() => send({ t: 'ask', kind: 'back' })}>Вернуться в основную партию</button>
                    </Show>
                    <Show when={g().phase !== 'setup'}>
                      <button class="btn small" disabled={!!g().req} onClick={() => send({ t: 'ask', kind: 'promote' })}>Сделать основной партией</button>
                    </Show>
                  </Show>
                  <div class="grid2">
                    <button class="btn small" disabled={!!g().req} onClick={() => send({ t: 'ask', kind: 'adjourn' })}>Отложить</button>
                    <button class="btn small" disabled={!!g().req} onClick={() => send({ t: 'ask', kind: 'close' })}>Закрыть</button>
                  </div>
                </div>
              }
            >
              {(r) => (
                <div class="banner">
                  <div style={{ display: 'flex', 'align-items': 'center', gap: '10px' }}><div class="spinner" /><strong>{REQ_MINE[r().kind]}</strong></div>
                  <span style={{ color: '#c9c3ba', 'font-size': '14px' }}>Ждём ответа {nickOf(opp())}.</span>
                  <button class="btn" onClick={() => send({ t: 'askCancel' })}>Отозвать предложение</button>
                </div>
              )}
            </Show>
          </Show>

          <Show when={g().phase === 'result' && g().result}>
            <div class="card" style={{ padding: '20px', display: 'flex', 'flex-direction': 'column', gap: '16px' }}>
              <div>
                <span class="label" style={{ color: 'var(--text)' }}>Победил</span>
                <div class="winner-line">
                  <span style={{ 'font-family': "'Unbounded', 'Manrope', sans-serif", 'font-size': '22px', 'font-weight': 700, 'min-width': 0 }}><Who s={g().result!.winner} size={30} /></span>
                  <span class="winner-by">
                    <Show when={g().result!.reason === 'score'} fallback="соперник сдался">на <strong>{num(g().result!.diff)}</strong> очка</Show>
                  </span>
                </div>
              </div>
              <Show when={g().result!.score}>
                {(sc) => (
                  <table class="score">
                    <thead><tr class="muted"><th scope="col"></th><th scope="col"><Who s="b" /></th><th scope="col"><Who s="w" /></th></tr></thead>
                    <tbody>
                      <tr><th scope="row">Территория</th><td>{sc().bTerritory}</td><td>{sc().wTerritory}</td></tr>
                      <tr><th scope="row">Пленные</th><td>{sc().bPrisoners}</td><td>{sc().wPrisoners}</td></tr>
                      <tr><th scope="row">Коми</th><td>—</td><td>{num(sc().komi)}</td></tr>
                      <tr style={{ 'font-weight': 800 }}><th scope="row">Итого</th><td>{num(sc().b)}</td><td>{num(sc().w)}</td></tr>
                      <tr style={{ 'font-weight': 800 }}><th scope="row">Разница</th><td>{g().result!.winner === 'b' ? '+' + num(g().result!.diff) : ''}</td><td>{g().result!.winner === 'w' ? '+' + num(g().result!.diff) : ''}</td></tr>
                    </tbody>
                  </table>
                )}
              </Show>
              <div class="grid2">
                <button class="btn primary" disabled={!!rematchPending()} onClick={() => send({ t: 'rematch' })}>{rematchPending() ? 'Ждём ответа…' : 'Реванш'}</button>
                <button class="btn" onClick={downloadSgf}>Скачать SGF</button>
              </div>
              <Show when={rematchPending()}><button class="btn ghost small" onClick={() => send({ t: 'cancelInvite' })}>Отменить реванш</button></Show>
              <button class="btn ghost" onClick={() => send({ t: 'leave' })}>В комнату</button>
            </div>
          </Show>
        </aside>
      </main>

      <Show when={theirReq()}>
        {(r) => (
          <div class="modal-back" role="alertdialog" aria-modal="true" aria-labelledby="rq-title">
            <div class="card modal">
              <div>
                <span class="label">{REQ_LABEL[r().kind]}</span>
                <h2 id="rq-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>{nickOf(opp())} предлагает {REQ_ASK[r().kind]}</h2>
              </div>
              <ul class="reqlist">
                <Show when={r().kind === 'alt'}>
                  <li>Эта партия будет отложена: камни остаются, часы стоят.</li>
                  <li>Вы оба перейдёте в её копию и выберете ход, с которого начать.</li>
                  <li>Из копии можно вернуться в эту партию или сделать копию основной.</li>
                </Show>
                <Show when={r().kind === 'back'}>
                  <li>Этот альтернативный розыгрыш закроется без подсчёта и в счёт встреч не пойдёт.</li>
                  <li>Основная партия продолжится с того места, где её отложили, часы — с остатка.</li>
                </Show>
                <Show when={r().kind === 'promote'}>
                  <li>Основная партия закроется без подсчёта и в счёт встреч не пойдёт.</li>
                  <li>Эта партия станет основной: у неё могут быть свои альтернативные розыгрыши.</li>
                  <Show when={g().alts > 1}><li>Другие альтернативные розыгрыши основной партии ({g().alts - 1}) перейдут к этой.</li></Show>
                </Show>
                <Show when={r().kind === 'adjourn'}>
                  <li>Часы остановятся, вы оба вернётесь в комнату.</li>
                  <li>Продолжить партию можно из «Моих партий», когда оба будете в сети.</li>
                </Show>
                <Show when={r().kind === 'close'}>
                  <li>Партия закончится без подсчёта и без победителя.</li>
                  <li>В счёт встреч она не пойдёт.</li>
                  <Show when={!g().parent && g().alts > 0}><li>Её альтернативные розыгрыши ({g().alts}) станут отдельными партиями.</li></Show>
                </Show>
              </ul>
              <div class="grid2">
                <button class="btn primary" onClick={() => send({ t: 'askAnswer', accept: true })}>Принять</button>
                <button class="btn" onClick={() => send({ t: 'askAnswer', accept: false })}>Отказаться</button>
              </div>
            </div>
          </div>
        )}
      </Show>

      <Show when={g().away && g().away === opp()}>
        <div class="modal-back" role="alertdialog" aria-modal="true" aria-labelledby="away-title">
          <div class="card modal">
            <div>
              <span class="label">Соперник не в сети</span>
              <h2 id="away-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>{nickOf(opp())} отключился, и его время на ход вышло</h2>
            </div>
            <p class="muted" style={{ margin: 0, 'font-size': '14px', 'line-height': 1.5 }}>
              Подсчёт — партия закончится по позиции сейчас (мёртвые камни определит сервер) и пойдёт в счёт встреч. Отложить — партия заморозится и останется в «Моих партиях». Если соперник вернётся раньше, игра продолжится.
            </p>
            <div class="grid2">
              <button class="btn primary" onClick={() => send({ t: 'awayScore' })}>Подсчитать очки</button>
              <button class="btn" onClick={() => send({ t: 'awayAdjourn' })}>Отложить</button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={g().phase === 'play' && g().pauseReq && g().pauseReq !== me()}>
        <div class="modal-back" role="alertdialog" aria-modal="true" aria-labelledby="pr-title">
          <div class="card modal">
            <div>
              <span class="label">Запрос паузы</span>
              <h2 id="pr-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>Соперник {nickOf(opp())} просит поставить игру на паузу</h2>
            </div>
            <ul style={{ margin: 0, padding: '16px 16px 16px 34px', 'border-radius': '12px', background: 'var(--bg)', border: '1px solid #33383d', display: 'flex', 'flex-direction': 'column', gap: '8px', 'line-height': 1.5, color: '#d6d2ca' }}>
              <li>Время на ход остановится у обоих.</li>
              <li>Доска размоется до конца паузы.</li>
              <li>Игра продолжится, когда вы оба нажмёте «Готов продолжить».</li>
            </ul>
            <div style={{ display: 'flex', 'justify-content': 'space-between', padding: '12px 16px', 'border-radius': '12px', background: '#2c241d', border: '1px solid #5a3d24' }}>
              <span>{g().turn === me() ? 'Ваше время на ход пока идёт' : 'Время соперника на ход пока идёт'}</span>
              <strong style={{ 'font-variant-numeric': 'tabular-nums' }}>{fmt(timeLeft(g().turn))}</strong>
            </div>
            <div class="grid2">
              <button class="btn primary" onClick={() => send({ t: 'pauseAnswer', accept: true })}>Согласиться на паузу</button>
              <button class="btn" onClick={() => send({ t: 'pauseAnswer', accept: false })}>Отказаться</button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  );
}

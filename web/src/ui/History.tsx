import { For, onMount, Show } from 'solid-js';
import { app, setApp, type HistoryGame, type Side } from '../store';
import { send } from '../net';
import { Topbar } from './Lobby';
import { MiniBoard, movesWord } from './MiniBoard';

const num = (v: number) => String(v).replace('.', ',');
const two = (n: number) => String(n).padStart(2, '0');
/** "11.11.2026, 19:31" in the viewer's time zone. */
export const dateTime = (iso: string) => {
  const d = new Date(iso);
  return `${two(d.getDate())}.${two(d.getMonth() + 1)}.${d.getFullYear()}, ${two(d.getHours())}:${two(d.getMinutes())}`;
};
export const stone = (s: Side) => <span class={`mini ${s}`} aria-hidden="true">{s === 'b' ? '火' : '冰'}</span>;

/** The side this player played in a history game. */
export const mySide = (h: HistoryGame): Side => (h.black.toLowerCase() === app.me?.nick.toLowerCase() ? 'b' : 'w');
const other = (s: Side): Side => (s === 'b' ? 'w' : 'b');

/** "rombl 火 4 : 2 冰 Romanos": you first, each nickname with the stone it played; the head-to-head score when the game ended. */
export function PairLine(props: { h: HistoryGame }) {
  const me = () => mySide(props.h);
  const nick = (s: Side) => (s === 'b' ? props.h.black : props.h.white);
  const wins = (s: Side) => props.h.h2h[s === 'b' ? 0 : 1];
  return (
    <span class="vsline">
      <span class="nk me">{nick(me())}</span>{stone(me())}
      <strong>{wins(me())} : {wins(other(me()))}</strong>
      {stone(other(me()))}<span class="nk">{nick(other(me()))}</span>
    </span>
  );
}

const STATUS = { finished: ['Закончена', 'done'], adjourned: ['Отложена', 'adj'], closed: ['Закрыта', 'closed'] } as const;

function Card(props: { h: HistoryGame }) {
  const h = () => props.h;
  const me = () => mySide(h());
  const opp = () => other(me());
  const nick = (s: Side) => (s === 'b' ? h().black : h().white);
  const won = () => h().result?.winner === me();
  const sc = () => h().result?.score;
  // known values only; the rest is a dash
  const cell = (s: Side, v: (s: Side) => number | null) => {
    const x = v(s);
    return <td classList={{ unk: x === null }}>{x === null ? '—' : num(x)}</td>;
  };
  const territory = (s: Side) => (sc() ? (s === 'b' ? sc()!.bTerritory : sc()!.wTerritory) : null);
  const prisoners = (s: Side) => (sc() ? (s === 'b' ? sc()!.bPrisoners : sc()!.wPrisoners) : h().caps[s === 'b' ? 0 : 1]);
  const komi = (s: Side) => (s === 'w' ? h().komi : null);
  const total = (s: Side) => (sc() ? (s === 'b' ? sc()!.b : sc()!.w) : null);
  const moves = () => (h().alt ? `${h().from} + ${movesWord(h().moves - h().from)}` : movesWord(h().moves));
  const view = () => {
    setApp('review', { id: h().id, moves: null, sgf: '' });
    setApp('view', 'review');
    send({ t: 'historyGame', game: h().id });
  };
  return (
    <article class="hcard">
      <div class="hmini"><MiniBoard size={h().size} board={h().board} last={h().last} px={150} /></div>
      <div class="hhead">
        <div class="toprow"><PairLine h={h()} /><span class="when">{dateTime(h().started)}</span></div>
        <div class="meta2">{h().size}×{h().size} · {moves()}</div>
        <div class="row-gap" style={{ 'flex-wrap': 'wrap', gap: '8px' }}>
          <span class={`chip ${STATUS[h().status][1]}`}>{STATUS[h().status][0]}</span>
          <Show when={h().alt}><span class="chip alt">Альтернативный розыгрыш</span></Show>
        </div>
        <Show when={h().result}>
          <div class="verdict" classList={{ win: won(), loss: !won() }}>
            {won() ? 'Победа' : 'Поражение'}
            <span>
              <Show when={h().result!.reason === 'score'} fallback={won() ? 'соперник сдался' : 'вы сдались'}>
                на <b>{num(h().result!.diff)}</b> очка
              </Show>
            </span>
          </div>
        </Show>
      </div>
      <div>
        <table class="score compact">
          <thead><tr class="muted"><th scope="col"></th><th scope="col"><span class="who">{stone(me())}вы</span></th><th scope="col"><span class="who">{stone(opp())}<span class="who-nick">{nick(opp())}</span></span></th></tr></thead>
          <tbody>
            <tr><th scope="row">Территория</th>{cell(me(), territory)}{cell(opp(), territory)}</tr>
            <tr><th scope="row">Пленные</th>{cell(me(), prisoners)}{cell(opp(), prisoners)}</tr>
            <tr><th scope="row">Коми</th>{cell(me(), komi)}{cell(opp(), komi)}</tr>
            <tr style={{ 'font-weight': 800 }}><th scope="row">Итого</th>{cell(me(), total)}{cell(opp(), total)}</tr>
          </tbody>
        </table>
        {/* only a finished game can be looked through */}
        <Show when={h().status === 'finished'}>
          <div class="hfoot"><button class="btn small" onClick={view}>Просмотреть игру</button></div>
        </Show>
      </div>
    </article>
  );
}

/** "История игр": the player's games, the latest first. */
export function History() {
  onMount(() => send({ t: 'history' }));
  const list = () => app.history ?? [];
  return (
    <div>
      <Topbar />
      <main class="mygames">
        <div class="between">
          <h2 style={{ 'font-size': '22px' }}>История игр</h2>
          <Show when={app.history}><span class="muted">{list().length ? `партий: ${list().length}` : ''}</span></Show>
        </div>
        <Show when={app.history} fallback={<p class="muted" style={{ margin: 0 }}>Загружаем…</p>}>
          <Show when={list().length > 0} fallback={<p class="muted" style={{ margin: 0 }}>Сыгранных партий пока нет.</p>}>
            <div class="hist"><For each={list()}>{(h) => <Card h={h} />}</For></div>
          </Show>
        </Show>
      </main>
    </div>
  );
}

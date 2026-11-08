import { For, Show } from 'solid-js';
import { app, setApp, timeLabel, type MyGame } from '../store';
import { send } from '../net';
import { Topbar } from './Lobby';
import { MiniBoard, movesWord } from './MiniBoard';

const dateLabel = (ms: number) => new Date(ms).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
const sideName = (s: string) => (s === 'b' ? 'чёрных' : 'белых');

/** "My games": unfinished games in rows, each row is a main game followed by its alternatives. */
export function MyGames() {
  // a player in a game first finishes it or adjourns it with the opponent's consent
  const playing = () => !!app.game && app.game.phase !== 'result';
  const waiting = () => !!app.lobby?.outgoing;

  const rows = () => {
    const by = new Map<string, MyGame[]>();
    for (const g of app.lobby?.games ?? []) {
      const row = by.get(g.main) ?? [];
      row.push(g);
      by.set(g.main, row);
    }
    const list = [...by.values()].map((row) => row.sort((a, b) => Number(a.alt) - Number(b.alt))); // main first, alternatives by age
    const key = (row: MyGame[]) => (row.some((g) => g.current) ? Infinity : Math.max(...row.map((g) => g.adjournedAt)));
    return list.sort((a, b) => key(b) - key(a));
  };

  const Card = (p: { g: MyGame }) => (
    <article class="gcard" classList={{ off: !p.g.available && !p.g.current, now: p.g.current }}>
      <MiniBoard size={p.g.size} board={p.g.board} last={p.g.last} px={222} />
      <div class="vs">
        <span class={`mini ${p.g.you}`} aria-hidden="true">{p.g.you === 'b' ? '火' : '冰'}</span>
        <span class="who-nick">вы — {p.g.opp}</span>
        <span class={`mini ${p.g.you === 'b' ? 'w' : 'b'}`} aria-hidden="true">{p.g.you === 'b' ? '冰' : '火'}</span>
      </div>
      <div class="meta">
        <Show when={p.g.alt} fallback={<span class="chip main">Основная</span>}>
          <span class="chip alt">Альтернативный розыгрыш</span>
        </Show>
        <br />
        {p.g.size}×{p.g.size} · {timeLabel(p.g.moveTime)} на ход · {movesWord(p.g.moves)}
        {p.g.alt && !p.g.setup ? ` · с хода ${p.g.from}` : ''}
        <br />
        {p.g.setup ? 'Выбор точки старта' : p.g.turn === p.g.you ? 'Ваш ход' : `Ход ${sideName(p.g.turn)}`}
        {p.g.current ? '' : ` · отложена ${dateLabel(p.g.adjournedAt)}`}
      </div>
      <div class="gcard-foot">
        <Show
          when={p.g.current}
          fallback={
            <>
              <Show when={p.g.available} fallback={<span class="chip off">Соперник не в сети или играет</span>}>
                <span class="chip ok">● Доступна</span>
              </Show>
              <button class="btn small primary" disabled={!p.g.available || playing() || waiting()} onClick={() => send({ t: 'resume', game: p.g.id })}>Продолжить</button>
            </>
          }
        >
          <span class="chip ok">● Идёт сейчас</span>
          <button class="btn small" onClick={() => setApp('view', 'main')}>К партии</button>
        </Show>
      </div>
    </article>
  );

  return (
    <div>
      <Topbar />
      <main class="mygames">
        <div>
          <h2 style={{ 'font-size': '22px' }}>Мои партии</h2>
          <p class="muted" style={{ margin: '6px 0 0', 'line-height': 1.5 }}>
            Каждый ряд — основная партия и её альтернативные розыгрыши (не больше 5). Отложенные партии хранятся, пока вы их не доиграете или не закроете. «Продолжить» отправляет сопернику предложение, партия продолжится, когда он согласится.
          </p>
        </div>
        <Show when={playing()}>
          <div class="banner"><span>Чтобы продолжить другую партию, сначала доиграйте текущую или отложите её с согласия соперника.</span></div>
        </Show>
        <Show when={rows().length > 0} fallback={<p class="muted" style={{ margin: 0 }}>Незаконченных партий нет.</p>}>
          <For each={rows()}>
            {(row) => (
              <section class="gamerow" aria-label={`Партия с ${row[0].opp}`}>
                <div class="gamerow-head">
                  <strong>Партия с {row[0].opp}</strong>
                  <span class="muted">альтернативных розыгрышей: {row.filter((g) => g.alt).length} из 5</span>
                </div>
                <div class="gamerow-cards"><For each={row}>{(g) => <Card g={g} />}</For></div>
              </section>
            )}
          </For>
        </Show>
      </main>
    </div>
  );
}

import { For, Show } from 'solid-js';
import { app, timeLabel, type MyGame } from '../store';
import { send } from '../net';
import { Topbar } from './Lobby';
import { MiniBoard, movesWord } from './MiniBoard';

const dateLabel = (ms: number) => new Date(ms).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
const sideName = (s: string) => (s === 'b' ? 'чёрных' : 'белых');

/** "My games": the adjourned games, each can be continued when the opponent is free. */
export function MyGames() {
  const games = () => app.lobby?.games ?? [];
  // a player in a game first finishes it or adjourns it with the opponent's consent
  const playing = () => !!app.game && app.game.phase !== 'result';
  const waiting = () => !!app.lobby?.outgoing;

  const Card = (p: { g: MyGame }) => (
    <article class="gcard" classList={{ off: !p.g.available }}>
      <MiniBoard size={p.g.size} board={p.g.board} last={p.g.last} px={222} />
      <div class="vs">
        <span class={`mini ${p.g.you}`} aria-hidden="true">{p.g.you === 'b' ? '火' : '冰'}</span>
        <span class="who-nick">вы — {p.g.opp}</span>
        <span class={`mini ${p.g.you === 'b' ? 'w' : 'b'}`} aria-hidden="true">{p.g.you === 'b' ? '冰' : '火'}</span>
      </div>
      <div class="meta">
        <Show when={p.g.alt} fallback={<span>Основная</span>}>
          <span class="chip alt">Альтернативный розыгрыш</span>
        </Show>
        {' '}· {p.g.size}×{p.g.size} · {timeLabel(p.g.moveTime)} на ход · {movesWord(p.g.moves)}
        <br />
        {p.g.setup ? 'Выбор точки старта' : p.g.alt ? `С хода ${p.g.from} · ` : ''}
        {p.g.setup ? '' : p.g.turn === p.g.you ? 'Ваш ход' : `Ход ${sideName(p.g.turn)}`} · отложена {dateLabel(p.g.adjournedAt)}
      </div>
      <div class="gcard-foot">
        <Show when={p.g.available} fallback={<span class="chip off">Соперник не в сети или играет</span>}>
          <span class="chip ok">● Доступна</span>
        </Show>
        <button class="btn small primary" disabled={!p.g.available || playing() || waiting()} onClick={() => send({ t: 'resume', game: p.g.id })}>Продолжить</button>
      </div>
    </article>
  );

  return (
    <div>
      <Topbar />
      <main class="mygames">
        <div>
          <h2 style={{ 'font-size': '22px' }}>Мои партии</h2>
          <p class="muted" style={{ margin: '6px 0 0', 'line-height': 1.5 }}>Отложенные партии хранятся, пока вы их не доиграете или не закроете. «Продолжить» отправляет сопернику предложение, партия продолжится, когда он согласится.</p>
        </div>
        <Show when={playing()}>
          <div class="banner"><span>Чтобы продолжить другую партию, сначала доиграйте текущую или отложите её с согласия соперника.</span></div>
        </Show>
        <Show when={games().length > 0} fallback={<p class="muted" style={{ margin: 0 }}>Отложенных партий нет.</p>}>
          <div class="games"><For each={games()}>{(g) => <Card g={g} />}</For></div>
        </Show>
      </main>
    </div>
  );
}

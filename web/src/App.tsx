import { Match, onMount, Show, Switch } from 'solid-js';
import { app, h2hLine, setApp, settingsLabel } from './store';
import { connect, fetchMe, send } from './net';
import { Login } from './ui/Login';
import { Lobby } from './ui/Lobby';
import { Game } from './ui/Game';
import { MyGames } from './ui/MyGames';
import { History } from './ui/History';
import { Review } from './ui/Review';
import { OutgoingModal } from './ui/Outgoing';
import { MiniBoard, movesWord } from './ui/MiniBoard';

export function App() {
  onMount(async () => {
    try {
      const me = await fetchMe();
      if (me) { setApp('me', me); connect(); }
    } finally {
      setApp('checked', true);
    }
  });

  // an invitation can arrive in the room or on the result screen (a rematch)
  const incoming = () => {
    const inv = app.lobby?.incoming?.[0];
    const g = app.game;
    if (!inv || (g && g.phase !== 'result')) return null;
    return inv;
  };

  return (
    <>
      <Show when={app.me && !app.connected && app.checked}>
        <div class="offline">Нет связи с сервером, переподключаемся…</div>
      </Show>
      <Switch>
        <Match when={!app.checked}><div /></Match>
        <Match when={!app.me || app.renaming}><Login /></Match>
        <Match when={app.view === 'games'}><MyGames /></Match>
        <Match when={app.view === 'history'}><History /></Match>
        <Match when={app.view === 'review'}><Review /></Match>
        <Match when={app.game}><Game /></Match>
        <Match when={true}><Lobby /></Match>
      </Switch>
      <Show when={!app.game || app.view === 'games'}><OutgoingModal /></Show>
      <Show when={incoming()?.game}>
        {(gm) => (
          <div class="modal-back" role="dialog" aria-modal="true" aria-labelledby="cont-title">
            <div class="card modal">
              <div>
                <span class="label">Продолжить партию</span>
                <h2 id="cont-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>{incoming()!.nick} предлагает продолжить отложенную партию</h2>
              </div>
              <div class="row-gap">
                <MiniBoard size={gm().size} board={gm().board} last={gm().last} px={120} />
                <span class="muted" style={{ 'font-size': '14px', 'line-height': 1.5 }}>
                  {gm().alt ? 'Альтернативный розыгрыш' : 'Основная'} · {gm().size}×{gm().size} · {movesWord(gm().moves)}<br />
                  {gm().setup ? 'Продолжится выбор точки старта.' : `${gm().turn === gm().you ? 'Ваш ход' : gm().turn === 'b' ? 'Ход чёрных' : 'Ход белых'}, часы продолжат с остатка.`}
                </span>
              </div>
              <div class="grid2">
                <button class="btn primary" onClick={() => { setApp('view', 'main'); send({ t: 'acceptInvite', from: incoming()!.from }); }}>Принять</button>
                <button class="btn" onClick={() => send({ t: 'declineInvite', from: incoming()!.from })}>Отказаться</button>
              </div>
            </div>
          </div>
        )}
      </Show>
      <Show when={incoming() && !incoming()!.game ? incoming() : null}>
        {(inv) => (
          <div class="modal-back" role="dialog" aria-modal="true" aria-labelledby="inv-title">
            <div class="card modal">
              <div>
                <span class="label">{app.game ? 'Реванш' : 'Приглашение'}</span>
                <h2 id="inv-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>{inv().nick} приглашает вас сыграть</h2>
              </div>
              <dl class="params">
                <div><dt class="label">Доска</dt><dd>{inv().settings.size}×{inv().settings.size}</dd></div>
                <div><dt class="label">Коми</dt><dd>{String(inv().settings.komi).replace('.', ',')}</dd></div>
                <div><dt class="label">Время на ход</dt><dd>{settingsLabel(inv().settings).split(' · ')[2].replace(' на ход', '')}</dd></div>
                <div><dt class="label">Ваш цвет</dt><dd>Случайно</dd></div>
              </dl>
              <div class="h2h-line">{h2hLine(inv().h2h)}</div>
              <p class="muted" style={{ margin: 0, 'font-size': '14px' }}>Настройки задал пригласивший игрок.</p>
              <div class="grid2">
                <button class="btn primary" onClick={() => { setApp('view', 'main'); send({ t: 'acceptInvite', from: inv().from }); }}>Принять</button>
                <button class="btn" onClick={() => send({ t: 'declineInvite', from: inv().from })}>Отклонить</button>
              </div>
            </div>
          </div>
        )}
      </Show>
    </>
  );
}

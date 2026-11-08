import { Match, onMount, Show, Switch } from 'solid-js';
import { app, h2hLine, setApp, settingsLabel } from './store';
import { connect, fetchMe, send } from './net';
import { Login } from './ui/Login';
import { Lobby } from './ui/Lobby';
import { Game } from './ui/Game';

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
        <Match when={app.game}><Game /></Match>
        <Match when={true}><Lobby /></Match>
      </Switch>
      <Show when={incoming()}>
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
                <button class="btn primary" onClick={() => send({ t: 'acceptInvite', from: inv().from })}>Принять</button>
                <button class="btn" onClick={() => send({ t: 'declineInvite', from: inv().from })}>Отклонить</button>
              </div>
            </div>
          </div>
        )}
      </Show>
      <Show when={app.notice}><div class="notice" role="status">{app.notice}</div></Show>
    </>
  );
}

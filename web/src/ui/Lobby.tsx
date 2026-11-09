import { For, Show, type JSX } from 'solid-js';
import { app, setApp, komiLabel, timeLabel, type Settings } from '../store';
import { send } from '../net';

const SIZES = [9, 13, 15, 19];
const KOMI = [4.5, 5.5, 6.5, 7.5];
const TIMES = [30, 60, 120, 180, 300, 600];

/** How many games wait in "My games" to be continued. */
const adjourned = () => (app.lobby?.games ?? []).filter((g) => !g.current).length;

/** The header; in a game `who` (the head-to-head score) takes the place of «Вы: nick». */
export function Topbar(props: { info?: string; who?: JSX.Element; corner?: JSX.Element }) {
  return (
    <header class="topbar">
      <div class="brand">
        <span aria-hidden="true" style={{ display: 'flex' }}><span class="mini b">火</span><span class="mini w" style={{ 'margin-left': '-6px' }}>冰</span></span>
        Го онлайн
        <Show when={props.info !== ''}>
          <span class="muted" style={{ 'font-family': 'Manrope', 'font-weight': 500, 'font-size': '15px', 'margin-left': '8px' }}>{props.info ?? 'Комната без рейтинга'}</span>
        </Show>
      </div>
      <nav style={{ display: 'flex', 'align-items': 'center', gap: '10px' }} aria-label="Разделы">
        <Show when={props.who} fallback={<span class="muted" style={{ 'font-size': '15px', 'margin-right': '4px' }}>Вы: <strong style={{ color: 'var(--text)' }}>{app.me?.nick}</strong></span>}>
          {props.who}
        </Show>
        <Show when={app.view === 'games'}>
          <button class="btn small" onClick={() => setApp('view', 'main')}>{app.game ? 'К партии' : 'Комната'}</button>
        </Show>
        <button class="btn small" classList={{ on: app.view === 'games' }} aria-pressed={app.view === 'games'} onClick={() => setApp('view', 'games')}>
          Мои партии<Show when={adjourned() > 0}><span class="count">{adjourned()}</span></Show>
        </button>
        {props.corner}
      </nav>
    </header>
  );
}

function Seg<T extends number>(props: { label: string; cols: number; options: T[]; value: T; text: (v: T) => string; onPick: (v: T) => void; disabled: boolean }) {
  return (
    <div style={{ display: 'flex', 'flex-direction': 'column', gap: '10px' }}>
      <span class="label">{props.label}</span>
      <div class="seg" style={{ 'grid-template-columns': `repeat(${props.cols}, minmax(0, 1fr))` }}>
        <For each={props.options}>
          {(o) => (
            <button type="button" classList={{ on: o === props.value }} aria-pressed={o === props.value} disabled={props.disabled} onClick={() => props.onPick(o)}>
              {props.text(o)}
            </button>
          )}
        </For>
      </div>
    </div>
  );
}

export function Lobby() {
  const settings = (): Settings => app.lobby?.settings ?? { size: 19, komi: 6.5, moveTime: 300 };
  const update = (patch: Partial<Settings>) => send({ t: 'settings', settings: { ...settings(), ...patch } });
  const outgoing = () => app.lobby?.outgoing ?? null;

  return (
    <div>
      <Topbar />
      <main class="lobby">
        <section class="card" aria-labelledby="settings-title" style={{ flex: '1 1 320px', 'max-width': '400px', padding: '24px', display: 'flex', 'flex-direction': 'column', gap: '22px' }}>
          <div>
            <h2 id="settings-title" style={{ 'font-size': '20px' }}>Настройки ваших приглашений</h2>
            <p class="muted" style={{ margin: '6px 0 0', 'font-size': '14px' }}>Применяются к партии, когда приглашение отправляете вы</p>
          </div>
          <Seg label="Размер доски" cols={4} options={SIZES} value={settings().size} text={(n) => `${n}×${n}`} onPick={(size) => update({ size })} disabled={!!outgoing()} />
          <Seg label="Коми" cols={4} options={KOMI} value={settings().komi} text={komiLabel} onPick={(komi) => update({ komi })} disabled={!!outgoing()} />
          <Seg label="Время на ход" cols={3} options={TIMES} value={settings().moveTime} text={timeLabel} onPick={(moveTime) => update({ moveTime })} disabled={!!outgoing()} />
          <div class="muted" style={{ 'border-top': '1px solid #33383d', 'padding-top': '16px', 'font-size': '14px', 'line-height': 1.5 }}>
            Общего времени нет. Если время на ход вышло, делается автоматический пас.
          </div>
        </section>

        <section class="card" aria-labelledby="players-title" style={{ flex: '999 1 560px', 'min-width': 0, overflow: 'hidden' }}>
          <div style={{ display: 'flex', 'justify-content': 'space-between', 'align-items': 'center', gap: '8px', padding: '20px 24px', 'border-bottom': '1px solid #33383d' }}>
            <h2 id="players-title" style={{ 'font-size': '20px' }}>Игроки в комнате</h2>
            <span class="muted">Онлайн: {app.lobby?.online ?? 0}</span>
          </div>
          <Show when={(app.lobby?.players.length ?? 0) > 0} fallback={<p class="muted" style={{ padding: '24px', margin: 0 }}>Пока никого нет. Откройте игру во второй вкладке или пришлите ссылку сопернику.</p>}>
            <ul class="players">
              <For each={app.lobby?.players ?? []}>
                {(p) => (
                  <li>
                    <span class={`dot ${p.status}`} aria-hidden="true" />
                    <div style={{ flex: '1 1 200px', 'min-width': 0, display: 'flex', 'align-items': 'baseline', gap: '8px' }}>
                      <span style={{ 'font-size': '16px', 'font-weight': 700, overflow: 'hidden', 'text-overflow': 'ellipsis', 'white-space': 'nowrap' }}>{p.nick}</span>
                    </div>
                    <span class="h2h" classList={{ ahead: p.h2h?.[0] > p.h2h?.[1], behind: p.h2h?.[0] < p.h2h?.[1] }} title="Ваш счёт встреч с этим игроком">
                      {p.h2h && p.h2h[0] + p.h2h[1] > 0 ? `${p.h2h[0]} : ${p.h2h[1]}` : '—'}
                    </span>
                    <span class="muted" style={{ 'font-size': '14px', 'min-width': '110px' }}>{p.status === 'playing' ? 'Играет' : 'Свободен'}</span>
                    <div style={{ 'min-width': '130px', display: 'flex', 'justify-content': 'flex-end' }}>
                      <Show when={p.status === 'free'}>
                        <button class="btn small" disabled={!!outgoing()} onClick={() => send({ t: 'invite', to: p.id })}>Пригласить</button>
                      </Show>
                    </div>
                  </li>
                )}
              </For>
            </ul>
          </Show>
        </section>
      </main>

    </div>
  );
}

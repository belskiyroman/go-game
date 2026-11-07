import { For, Show } from 'solid-js';
import { app, setApp, komiLabel, timeLabel, type Settings } from '../store';
import { send } from '../net';

const SIZES = [9, 13, 15, 19];
const KOMI = [4.5, 5.5, 6.5, 7.5];
const TIMES = [30, 60, 120, 180, 300, 600];

export function Topbar(props: { info?: string }) {
  return (
    <header class="topbar">
      <div class="brand">
        <span aria-hidden="true" style={{ display: 'flex' }}><span class="mini b">火</span><span class="mini w" style={{ 'margin-left': '-6px' }}>冰</span></span>
        Го онлайн
        <span class="muted" style={{ 'font-family': 'Manrope', 'font-weight': 500, 'font-size': '15px', 'margin-left': '8px' }}>{props.info ?? 'Комната без рейтинга'}</span>
      </div>
      <Show when={!props.info}>
        <div style={{ display: 'flex', 'align-items': 'center', gap: '12px' }}>
          <span class="muted" style={{ 'font-size': '15px' }}>Вы: <strong style={{ color: 'var(--text)' }}>{app.me?.nick}</strong> <span style={{ 'font-size': '13px' }}>{app.me?.tag}</span></span>
          <button class="btn ghost small" onClick={() => setApp('renaming', true)}>Сменить ник</button>
        </div>
      </Show>
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
                      <span style={{ 'font-size': '13px', color: '#8f9397' }}>{p.tag}</span>
                    </div>
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

      <Show when={outgoing()}>
        {(o) => (
          <div class="modal-back" role="dialog" aria-modal="true" aria-labelledby="out-title">
            <div class="card modal">
              <div>
                <span class="label">Приглашение отправлено</span>
                <h2 id="out-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>Ждём ответа от {o().nick}</h2>
              </div>
              <div style={{ display: 'flex', 'align-items': 'center', gap: '12px', color: '#c9c3ba' }}>
                <div class="spinner" /><span>Игрок ещё не принял и не отклонил приглашение</span>
              </div>
              <dl class="params">
                <div><dt class="label">Доска</dt><dd>{o().settings.size}×{o().settings.size}</dd></div>
                <div><dt class="label">Коми</dt><dd>{komiLabel(o().settings.komi)}</dd></div>
                <div><dt class="label">Время на ход</dt><dd>{timeLabel(o().settings.moveTime)}</dd></div>
                <div><dt class="label">Ваш цвет</dt><dd>Случайно</dd></div>
              </dl>
              <p class="muted" style={{ margin: 0, 'font-size': '14px', 'line-height': 1.5 }}>Пока приглашение активно, настройки менять нельзя. Чтобы изменить их или пригласить другого игрока, отмените это приглашение.</p>
              <button class="btn" onClick={() => send({ t: 'cancelInvite' })}>Отменить приглашение</button>
            </div>
          </div>
        )}
      </Show>
    </div>
  );
}

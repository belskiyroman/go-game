import { Show } from 'solid-js';
import { app, h2hLine, komiLabel, timeLabel } from '../store';
import { send } from '../net';
import { MiniBoard, movesWord } from './MiniBoard';

/** The player's own invitation waiting for an answer: a new game or continuing an adjourned one. */
export function OutgoingModal() {
  const o = () => app.lobby?.outgoing ?? null;
  return (
    <Show when={o()}>
      {(o) => (
        <div class="modal-back" role="dialog" aria-modal="true" aria-labelledby="out-title">
          <div class="card modal">
            <div>
              <span class="label">{o().game ? 'Продолжить партию' : 'Приглашение отправлено'}</span>
              <h2 id="out-title" style={{ 'font-size': '22px', 'margin-top': '6px' }}>Ждём ответа от {o().nick}</h2>
            </div>
            <div style={{ display: 'flex', 'align-items': 'center', gap: '12px', color: '#c9c3ba' }}>
              <div class="spinner" /><span>{o().game ? 'Вы предложили продолжить отложенную партию' : 'Игрок ещё не принял и не отклонил приглашение'}</span>
            </div>
            <Show
              when={o().game}
              fallback={
                <>
                  <div class="h2h-line">{h2hLine(o().h2h)}</div>
                  <dl class="params">
                    <div><dt class="label">Доска</dt><dd>{o().settings.size}×{o().settings.size}</dd></div>
                    <div><dt class="label">Коми</dt><dd>{komiLabel(o().settings.komi)}</dd></div>
                    <div><dt class="label">Время на ход</dt><dd>{timeLabel(o().settings.moveTime)}</dd></div>
                    <div><dt class="label">Ваш цвет</dt><dd>Случайно</dd></div>
                  </dl>
                  <p class="muted" style={{ margin: 0, 'font-size': '14px', 'line-height': 1.5 }}>Пока приглашение активно, настройки менять нельзя. Чтобы изменить их или пригласить другого игрока, отмените это приглашение.</p>
                </>
              }
            >
              {(gm) => (
                <div class="row-gap">
                  <MiniBoard size={gm().size} board={gm().board} last={gm().last} px={120} />
                  <span class="muted" style={{ 'font-size': '14px', 'line-height': 1.5 }}>{gm().alt ? 'Альтернативный розыгрыш' : 'Основная'} · {gm().size}×{gm().size} · {movesWord(gm().moves)}</span>
                </div>
              )}
            </Show>
            <button class="btn" onClick={() => send({ t: 'cancelInvite' })}>{o().game ? 'Отозвать предложение' : 'Отменить приглашение'}</button>
          </div>
        </div>
      )}
    </Show>
  );
}

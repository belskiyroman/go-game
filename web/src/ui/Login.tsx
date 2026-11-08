import { createSignal } from 'solid-js';
import { app, setApp } from '../store';
import { connect, login, reconnect, send } from '../net';

export function Login() {
  const [nick, setNick] = createSignal(app.me?.nick ?? '');
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');

  const submit = async (e: Event) => {
    e.preventDefault();
    if (!nick().trim()) { setError('Введите ник'); return; }
    setBusy(true); setError('');
    try {
      const prev = app.me?.id;
      const me = await login(nick());
      setApp('me', me);
      setApp('renaming', false);
      if (prev && prev !== me.id) reconnect(); // signed in as another player
      else { connect(); send({ t: 'refresh' }); }
    } catch {
      setError('Не удалось войти. Попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main style={{ 'min-height': '100vh', display: 'flex', 'align-items': 'center', 'justify-content': 'center', padding: '32px 16px', background: 'radial-gradient(ellipse at 50% 0%, #2a2118 0%, #141618 60%)' }}>
      <div class="card" style={{ width: '100%', 'max-width': '440px', padding: '40px 32px', display: 'flex', 'flex-direction': 'column', gap: '28px', 'box-shadow': '0 30px 60px rgba(0,0,0,.45)' }}>
        <div style={{ display: 'flex', 'flex-direction': 'column', 'align-items': 'center', gap: '14px', 'text-align': 'center' }}>
          <div aria-hidden="true" style={{ display: 'flex' }}>
            <span class="mini b" style={{ width: '44px', height: '44px', 'font-size': '24px' }}>火</span>
            <span class="mini w" style={{ width: '44px', height: '44px', 'font-size': '24px', 'margin-left': '-10px' }}>冰</span>
          </div>
          <h1 style={{ 'font-family': "'Unbounded', 'Manrope', sans-serif", 'font-size': '34px' }}>Го онлайн</h1>
          <p class="muted" style={{ margin: 0, 'font-size': '16px', 'line-height': 1.5 }}>Введите ник, чтобы войти в комнату и найти соперника</p>
        </div>
        <form style={{ display: 'flex', 'flex-direction': 'column', gap: '14px' }} onSubmit={submit}>
          <label for="nick" class="label">Ваш ник</label>
          <input id="nick" class="field" type="text" maxlength="20" placeholder="Например, Странник" autocomplete="off"
            value={nick()} onInput={(e) => setNick(e.currentTarget.value)} />
          <button class="btn primary" style={{ 'min-height': '52px', 'font-size': '16px' }} disabled={busy()}>Войти</button>
          <span role="alert" style={{ color: 'var(--danger)', 'min-height': '20px' }}>{error()}</span>
        </form>
        <p class="muted" style={{ margin: 0, 'font-size': '14px', 'line-height': 1.55, 'text-align': 'center' }}>
          Ник — это ваш логин, пароля пока нет. Новый ник создаёт нового игрока, существующий — входит под ним. Игрок хранится 2 дня с последнего захода.
        </p>
      </div>
    </main>
  );
}

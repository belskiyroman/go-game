import { reconcile } from 'solid-js/store';
import { app, setApp, emitGameUpdate, type GameEvent, type GameState, type Me } from './store';

// ngrok's free plan shows a warning page to browsers; this header lets API calls through it.
const headers = { 'Content-Type': 'application/json', 'ngrok-skip-browser-warning': '1' };

export async function fetchMe(): Promise<Me | null> {
  const r = await fetch('/api/me', { headers, credentials: 'same-origin' });
  return r.ok ? r.json() : null; // null: no session yet
}

export async function login(nick: string): Promise<Me> {
  const r = await fetch('/api/login', { method: 'POST', headers, credentials: 'same-origin', body: JSON.stringify({ nick }) });
  if (!r.ok) throw new Error('login failed');
  return r.json();
}

let ws: WebSocket | null = null;
let retry = 0;
let wanted = false;

export function connect() {
  wanted = true;
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const sock = new WebSocket(`${proto}://${location.host}/ws`);
  ws = sock;
  sock.onopen = () => { retry = 0; setApp('connected', true); };
  sock.onclose = () => {
    if (ws !== sock && ws !== null) return; // an old connection replaced by reconnect()
    ws = null;
    setApp('connected', false);
    if (wanted) setTimeout(connect, Math.min(5000, 500 * 2 ** retry++));
  };
  sock.onmessage = (ev) => {
    let m: any;
    try { m = JSON.parse(ev.data); } catch { return; }
    switch (m.t) {
      case 'me': setApp('me', { id: m.id, nick: m.nick, tag: m.tag }); break;
      case 'lobby': setApp('lobby', { players: m.players, online: m.online, settings: m.settings, outgoing: m.outgoing, incoming: m.incoming, games: m.games ?? [] }); break;
      case 'game': {
        const g = m.game as GameState;
        setApp('clockOffset', g.serverNow - Date.now());
        if (app.game?.id !== g.id) setApp('view', 'main'); // a new game (or a continued one) opens at once
        setApp('game', reconcile(g)); // replace, not merge: fields that are gone (a request, a proposal) must go
        emitGameUpdate(g, (m.event as GameEvent) ?? null);
        break;
      }
      case 'noGame': setApp('game', null); break;
      case 'error': break;
    }
  };
}

export function disconnect() { wanted = false; ws?.close(); }

/** After signing in as another player: drop the old connection and the old player's state. */
export function reconnect() {
  const old = ws;
  ws = null;
  if (old) { old.onmessage = null; old.onclose = null; old.close(); }
  setApp('game', null);
  setApp('lobby', null);
  connect();
}

export function send(msg: Record<string, unknown>) {
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
}


export const isMyGameTurn = () => {
  const g = app.game;
  return !!g && g.phase === 'play' && g.you === g.turn;
};

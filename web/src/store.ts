import { createStore } from 'solid-js/store';

export type Settings = { size: number; komi: number; moveTime: number };
export type Me = { id: string; nick: string; tag: string };
// h2h: head-to-head wins [yours against this player, theirs against you]
export type Player = { id: string; nick: string; tag: string; status: 'free' | 'playing'; h2h: [number, number] };
export type Side = 'b' | 'w';
/** An adjourned game in "My games" (also attached to an invitation to continue it). */
export type MyGame = {
  id: string; you: Side; opp: string; size: number; komi: number; moveTime: number;
  board: string; last: number; moves: number; turn: Side;
  alt: boolean; from: number; setup: boolean; // an alternative game, its start move; adjourned while choosing the start
  adjournedAt: number; available: boolean;     // the opponent is online and not in a game
};
export type Outgoing = { to: string; nick: string; tag: string; settings: Settings; h2h: [number, number]; game?: MyGame | null };
export type Incoming = { from: string; nick: string; tag: string; settings: Settings; h2h: [number, number]; game?: MyGame | null };
export type Lobby = { players: Player[]; online: number; settings: Settings; outgoing: Outgoing | null; incoming: Incoming[]; games: MyGame[] };

export type Score = { bTerritory: number; wTerritory: number; bPrisoners: number; wPrisoners: number; komi: number; b: number; w: number };
export type GameState = {
  id: string; seq: number; size: number; komi: number; moveTime: number;
  you: Side | '';
  black: { id: string; nick: string; tag: string };
  white: { id: string; nick: string; tag: string };
  board: string; turn: Side; last: number; passes: number;
  caps: { b: number; w: number };
  phase: 'play' | 'paused' | 'scoring' | 'result' | 'setup';
  pauseReq: Side | '';
  ready: { b: boolean; w: boolean };
  agree: { b: boolean; w: boolean };
  serverNow: number; deadline: number; remaining: number; moves: number;
  played: number; playedRunning: boolean; // game time without pauses, ms at serverNow
  h2h?: { b: number; w: number }; // head-to-head wins of the two players
  dead?: number[]; territory?: { b: number[]; w: number[] };
  result?: { winner: Side; reason: 'score' | 'resign'; score?: Score; diff: number };
  sgf?: string;
  away: Side | '';               // this player went offline and ran out of time: the other one decides
  parent: string; from: number;  // an alternative game: the game it was copied from and the start move
  req?: { kind: 'alt' | 'adjourn' | 'close'; from: Side }; // a request waiting for an answer
  source?: [number, number, number][]; // setup: the moves to choose from, [color 1|2, x, y] (a pass is x = -1)
  proposal?: { from: Side; step: number; swap: boolean }; // setup: the offered start move
};
export type GameEvent =
  | { kind: 'move'; color: Side; x: number; y: number; captured: number[] }
  | { kind: 'pass'; color: Side; timeout?: boolean }
  | { kind: 'resign'; color: Side }
  | { kind: 'started' };

type AppState = {
  me: Me | null;
  checked: boolean;          // the session check finished
  renaming: boolean;         // the player opened "change nickname"
  connected: boolean;
  lobby: Lobby | null;
  game: GameState | null;
  clockOffset: number;       // server time minus local time, ms
  notice: string;
  view: 'main' | 'games';    // "My games" opened over the room or the game
};

export const [app, setApp] = createStore<AppState>({
  me: null, checked: false, renaming: false, connected: false, lobby: null, game: null, clockOffset: 0, notice: '', view: 'main',
});

// Game events go to the board renderer in order; it consumes them by sequence.
type Listener = (g: GameState, e: GameEvent | null) => void;
const listeners = new Set<Listener>();
export function onGameUpdate(fn: Listener) { listeners.add(fn); return () => listeners.delete(fn); }
export function emitGameUpdate(g: GameState, e: GameEvent | null) { listeners.forEach((fn) => fn(g, e)); }

export const serverNow = () => Date.now() + app.clockOffset;

export const komiLabel = (k: number) => String(k).replace('.', ',');
export const timeLabel = (s: number) => (s < 60 ? `${s} с` : `${s / 60} мин`);
/** Head-to-head line for invitations: "Ваши встречи: 7 : 4". */
export const h2hLine = (s?: [number, number]) => (!s || s[0] + s[1] === 0 ? 'Вы ещё не играли друг с другом' : `Ваши встречи: ${s[0]} : ${s[1]}`);
export const settingsLabel = (s: Settings) => `${s.size}×${s.size} · коми ${komiLabel(s.komi)} · ${timeLabel(s.moveTime)} на ход`;

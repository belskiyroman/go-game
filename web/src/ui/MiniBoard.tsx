import { For, Show } from 'solid-js';

/** A small static board for "My games" and invitations. */
export function MiniBoard(props: { size: number; board: string; last: number; px: number }) {
  const m = () => props.px * 0.06;
  const cell = () => (props.px - 2 * m()) / (props.size - 1);
  const at = (i: number) => [m() + (i % props.size) * cell(), m() + Math.floor(i / props.size) * cell()];
  const stones = () => [...props.board].map((c, i) => ({ c, i })).filter((s) => s.c !== '.');
  return (
    <svg class="miniboard" width={props.px} height={props.px} viewBox={`0 0 ${props.px} ${props.px}`} role="img" aria-label="Позиция на доске">
      <rect width={props.px} height={props.px} rx="8" fill="url(#mb-wood)" />
      <defs>
        <linearGradient id="mb-wood" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#d6a35c" /><stop offset="1" stop-color="#b9813f" /></linearGradient>
        <radialGradient id="mb-b" cx="34%" cy="28%"><stop offset="0" stop-color="#7a7f86" /><stop offset=".25" stop-color="#2a2c30" /><stop offset="1" stop-color="#050506" /></radialGradient>
        <radialGradient id="mb-w" cx="34%" cy="28%"><stop offset="0" stop-color="#fff" /><stop offset=".35" stop-color="#f1efe9" /><stop offset="1" stop-color="#bfbaae" /></radialGradient>
      </defs>
      <For each={Array.from({ length: props.size }, (_, i) => i)}>
        {(i) => {
          const o = () => m() + i * cell();
          const w = i === 0 || i === props.size - 1 ? 1.2 : 0.7;
          return (
            <>
              <line x1={m()} y1={o()} x2={props.px - m()} y2={o()} stroke="rgba(58,40,18,.85)" stroke-width={w} />
              <line x1={o()} y1={m()} x2={o()} y2={props.px - m()} stroke="rgba(58,40,18,.85)" stroke-width={w} />
            </>
          );
        }}
      </For>
      <For each={stones()}>
        {(s) => <circle cx={at(s.i)[0]} cy={at(s.i)[1]} r={cell() * 0.47} fill={s.c === 'b' ? 'url(#mb-b)' : 'url(#mb-w)'} />}
      </For>
      <Show when={props.last >= 0 && props.board[props.last] !== '.'}>
        <circle cx={at(props.last)[0]} cy={at(props.last)[1]} r={cell() * 0.2} fill="none" stroke={props.board[props.last] === 'b' ? '#ff9a4a' : '#3d7fd6'} stroke-width={Math.max(1, cell() * 0.12)} />
      </Show>
    </svg>
  );
}

/** "Основная · 19×19 · 57 ходов" and so on. */
export const movesWord = (n: number) => {
  const a = n % 10, b = n % 100;
  if (a === 1 && b !== 11) return `${n} ход`;
  if (a >= 2 && a <= 4 && (b < 12 || b > 14)) return `${n} хода`;
  return `${n} ходов`;
};

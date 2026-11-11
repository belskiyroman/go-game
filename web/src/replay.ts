import type { Move3 } from './store';

/** A position that plays moves by the rules: captures, no suicide (ko is checked by the caller). */
export class Goban {
  pts: number[];
  caps = [0, 0, 0]; // stones captured by black [1] and white [2]
  last = -1;
  constructor(readonly size: number) { this.pts = new Array<number>(size * size).fill(0); }

  private near(i: number) {
    const n = this.size, x = i % n, y = Math.floor(i / n), out: number[] = [];
    if (x > 0) out.push(i - 1);
    if (x < n - 1) out.push(i + 1);
    if (y > 0) out.push(i - n);
    if (y < n - 1) out.push(i + n);
    return out;
  }

  /** The group at i, or null when it has a liberty. */
  private dead(i: number) {
    const c = this.pts[i], group = [i], seen = new Set([i]);
    for (let k = 0; k < group.length; k++) {
      for (const j of this.near(group[k])) {
        if (this.pts[j] === 0) return null;
        if (this.pts[j] === c && !seen.has(j)) { seen.add(j); group.push(j); }
      }
    }
    return group;
  }

  /** Plays a stone; returns the captured points, or null for an illegal move (occupied, suicide). */
  play(c: number, x: number, y: number): number[] | null {
    if (x < 0) { this.last = -1; return []; } // a pass
    const i = y * this.size + x;
    if (this.pts[i] !== 0) return null;
    this.pts[i] = c;
    const captured: number[] = [];
    for (const j of this.near(i)) {
      if (this.pts[j] && this.pts[j] !== c) {
        const g = this.dead(j);
        if (g) for (const k of g) { this.pts[k] = 0; captured.push(k); }
      }
    }
    if (!captured.length && this.dead(i)) { this.pts[i] = 0; return null; }
    this.caps[c] += captured.length;
    this.last = i;
    return captured;
  }

  board() { return this.pts.map((c) => (c === 1 ? 'b' : c === 2 ? 'w' : '.')).join(''); }
}

/** Replays the first n moves on an empty board. */
export function replay(size: number, moves: Move3[], n: number) {
  const b = new Goban(size);
  for (const [c, x, y] of moves.slice(0, n)) b.play(c, x, y);
  return { board: b.board(), last: b.last, caps: { b: b.caps[1], w: b.caps[2] } };
}

/** The moves of an SGF record: ";B[dd]" → [1, 3, 3], ";W[]" (a pass) → [2, -1, -1]. */
export function parseSgf(sgf: string): Move3[] {
  const out: Move3[] = [];
  for (const m of sgf.matchAll(/;([BW])\[([a-s]{2})?\]/g)) {
    const c = m[1] === 'B' ? 1 : 2;
    out.push(m[2] ? [c, m[2].charCodeAt(0) - 97, m[2].charCodeAt(1) - 97] : [c, -1, -1]);
  }
  return out;
}

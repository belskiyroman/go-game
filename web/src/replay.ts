/** Replays moves on an empty board: for choosing the start move of an alternative game. */
export function replay(size: number, moves: [number, number, number][], n: number) {
  const pts = new Array<number>(size * size).fill(0);
  let last = -1;
  const near = (i: number) => {
    const x = i % size, y = Math.floor(i / size), out: number[] = [];
    if (x > 0) out.push(i - 1);
    if (x < size - 1) out.push(i + 1);
    if (y > 0) out.push(i - size);
    if (y < size - 1) out.push(i + size);
    return out;
  };
  // removes the group at i if it has no liberties
  const take = (i: number) => {
    const c = pts[i], group = [i], seen = new Set([i]);
    for (let k = 0; k < group.length; k++) {
      for (const j of near(group[k])) {
        if (pts[j] === 0) return;
        if (pts[j] === c && !seen.has(j)) { seen.add(j); group.push(j); }
      }
    }
    for (const j of group) pts[j] = 0;
  };
  for (const [c, x, y] of moves.slice(0, n)) {
    if (x < 0) { last = -1; continue; }
    const i = y * size + x;
    pts[i] = c;
    for (const j of near(i)) if (pts[j] && pts[j] !== c) take(j);
    last = i;
  }
  return { board: pts.map((c) => (c === 1 ? 'b' : c === 2 ? 'w' : '.')).join(''), last };
}

// The timeline's reading of the log: the events grouped by turn and by
// whose half of the turn they fall in, where each turn's review starts, and
// the prizes left after each turn. Shared by the timeline, its turn list
// and the playback controls' scrubber.

// groupTurns groups log lines into turns ({turn, lines, halves}), each
// half ({turn, seat, lines}) one player's part of the turn ('' in setup).
// A turn counts both players: turn 3 is each player's third turn.
export function groupTurns(log) {
  const out = [];
  for (const l of log) {
    let t = out.at(-1);
    if (!t || t.turn !== l.turn) out.push((t = { turn: l.turn, lines: [], halves: [] }));
    t.lines.push(l);
    let h = t.halves.at(-1);
    const seat = l.turn_seat || '';
    if (!h || h.seat !== seat) t.halves.push((h = { turn: l.turn, seat, lines: [] }));
    h.lines.push(l);
  }
  return out;
}

// startOf is where a turn's, or one player's half of it, review begins:
// the turn has started and the opening draw is in hand, before the
// player's first choice. For setup, the end of setup. It takes anything
// with {turn, lines}, a whole turn or a half.
export function startOf(h) {
  const lines = h.lines;
  if (h.turn === 0) return lines.at(-1).id;
  let i = lines.findIndex((l) => /^turn \d+ start/.test(l.text));
  if (i < 0) return lines[0].id;
  const through = (action) => {
    while (i + 1 < lines.length && lines[i + 1].action === action) i++;
  };
  through(lines[i].action);
  // Imported games log the opening draw as its own action.
  if (lines[i + 1]?.text.includes(' drew ')) {
    i++;
    through(lines[i].action);
  }
  return lines[i].id;
}

// prizeScores maps each turn to the prizes each player has left at its
// end ({p1, p2}), counting the prize cards dealt at setup down by every
// prize taken.
export function prizeScores(turns) {
  const left = { p1: 6, p2: 6 };
  const out = new Map();
  for (const t of turns) {
    for (const l of t.lines) {
      const setup = /^setup: .*prizes (\d+)/.exec(l.text);
      if (setup) left.p1 = left.p2 = Number(setup[1]);
      const took = /^(p1|p2) took prize /.exec(l.text);
      if (took) left[took[1]] = Math.max(0, left[took[1]] - 1);
    }
    out.set(t.turn, { ...left });
  }
  return out;
}

// replayFrom is where a replay of the current position starts: the start
// of the latest half that has moves after its start, before event head.
// 0 (the beginning) when there is none.
export function replayFrom(turns, head) {
  for (let i = turns.length - 1; i >= 0; i--) {
    const halves = turns[i].halves;
    for (let j = halves.length - 1; j >= 0; j--) {
      const s = startOf(halves[j]);
      if (s < head) return s;
    }
  }
  return 0;
}

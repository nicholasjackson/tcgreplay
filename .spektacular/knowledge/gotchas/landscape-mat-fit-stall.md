---
tags: [board, layout, webui, playwright, wide]
---

# The landscape mat can stall when its height and width card sizes nearly match

On the landscape mat (board area 1600px or wider, `wide-board/design.md`) the card size is the largest that lets all four rows fit the board area's height and the widest row fit its width, measured live in `web/src/App.svelte`. When the size allowed by the height and the size allowed by the width come out almost equal (seen at 2560×1440: 191.0px against 191.85px), the measurement can go back and forth and the page stalls for several seconds after a resize; `wide.spec.js` "nothing on the board is cut off" then fails every run.

Anything that changes the space around the mat, or the height of its rows, can tip it into that state: the header's height, the gutter or padding around the board panel, the timeline's width, or anything that hangs below a card. In the Studio restyle, letting attached Energy and Tools hang below the cards on touch screens stalled the main thread for about 7.5s after a resize, which failed `desktop.spec.js` "follows resizing live" on the tablet profile; the fix was to scope that rule to today's mat only. If a frame or row change makes the wide or resize specs slow or flaky, look at how close the two limits are and move the frame a few pixels so one clearly wins (the Studio restyle settled on a 4px gutter around the landscape mat for this reason), rather than loosening the tests.

There is almost no height to spare: at 2560×1440 the card width sits within about 1px of the "1.6× today's width" threshold that `wide.spec.js` checks, so anything that takes extra height from the landscape mat (a taller strip, a readout under the bench, a caption row) fails that test. Take the space from the width or the frame instead.


**An 8-second freeze on load was fitBoard re-running itself, not the fit oscillating.** `fitBoard` (`App.svelte`) runs its first measure inside the `{@attach}` effect. While that measure read `fit` tracked, every write to `fit` tore the attachment down (resetting `fit` to 0) and attached it again. On a board that overflows as soon as it loads (the landscape mat with two tucked strips over a Bench Pokémon), that loop ran about 1000 forced layouts until Svelte threw `effect_update_depth_exceeded`, about 8 seconds later. The fix is that the first measure runs in `untrack(watch)`. If a measure attachment ever reads state it writes, wrap it the same way. To find a freeze like this, catch `pageerror` and count `getBoundingClientRect` callers in an init script; a CPU profile shows the work as Layout and UpdateLayoutTree inside one FunctionCall.

**Rows on the landscape mat must not be squeezable.** Your hand used to be a row with `min-height: 0` in that fixed-height flex column. When the rows ran over, the hand's box shrank, fitBoard saw nothing to fix, and its cards spilled over your Bench. Both hands are now strips in the board's top and bottom margins (absolutely placed, so fitBoard ignores them), and mat.spec checks at 2560×1440 and 3840×2160, with and without tucked strips, that neither hand covers its Bench.

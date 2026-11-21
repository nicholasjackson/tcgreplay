---
tags: [design, board, site, webui, testing, studio]
---

# The signed-off design canvas wins over the shipped code and over old tests

Once the maintainer signs off a design (the Studio canvas, https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ, and the design documents under the `design` source), it defines structure, layout and copy:

- **Build to it.** When an existing browser test pins the old layout (a fanned hand, a "Timeline" heading, a 12px line size), update the test to the design. Don't keep the old look to keep the test passing. The Studio restyle broke this rule. The board shipped fanned hands, an opponent hand fan instead of a "Hand 6" chip, and a timeline with an extra heading, split control rows and turn-level headings. The header was also missing the breadcrumb, warnings pill and theme button. The maintainer caught it against the signed-off StudioBoard.
- **The design system follows the canvas, not the code.** The "TCG Replay Studio" design system (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK) documents the design as signed off. The shipped code supplies only token names and values, and parts the canvas never drew.
- **Deviate only with sign-off.** Any departure from the design needs the maintainer's explicit agreement, not just a note in a changelog's Deviations. Show the difference side by side and ask.

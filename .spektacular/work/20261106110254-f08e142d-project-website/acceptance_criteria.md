- [ ] **Pitch, animation and start link above the fold**
  On a 1366×768 browser window, the home page shows the one-sentence description, the animation and a "Get started" link without scrolling; following the link opens the connecting instructions.
- [ ] **Animation shows the loop offline**
  With the hosted playmat unreachable (blocked in the browser), the home page animation still plays, loops, and shows in order: a battle log arriving, the board filling with the game, a coaching message, a rewind to an earlier turn, and a different line being played.
- [ ] **Reduced motion shows a still**
  With the operating system's reduced-motion setting on, the home page shows a still image of the playmat where the animation would be, and nothing on the page moves.
- [ ] **Each assistant's instructions lead to a working connection**
  A person who has never used tcgreplay, following only the site's instructions for claude.ai, for ChatGPT and for Claude Code in turn, gets the assistant to confirm it can reach the playmat in each (for example by listing what it can do), and the instructions say how to check this.
- [ ] **Desktop instructions open the board**
  Following only the site's desktop instructions on a machine with the prerequisites the site lists, a person installs the playmat, connects the Claude desktop app, starts a game, and opens the board at the address the site gives.
- [ ] **Sign-in and board link explained**
  The site has text, findable from the connecting instructions, describing the Google sign-in prompt and how to open the board link from the assistant's reply.
- [ ] **Import instructions produce a complete import**
  Following the site's import instructions with the site's example game, the assistant reports an import with no problems, and the site says what a complete import looks like and what to do if lines are missing.
- [ ] **Every coaching workflow is covered**
  For each coaching workflow the playmat offers at the time of publishing, the site has a section describing it with at least one example prompt.
- [ ] **Rewinding is explained**
  The site has a section explaining how to rewind an imported game to a chosen turn and play a different line, with an example prompt.
- [ ] **Example game is complete and anonymised**
  The example battle log on the site has every line of the game from setup to the result, contains no real opponent's player name, and imports with its decklist without problems.
- [ ] **Limits match the service**
  The site's stated idle time, users served at once and requests per minute match the hosted playmat's configured limits at the time of publishing. The site also says what the hosted playmat keeps, that games are lost when the service is updated, that a game's record can be kept and imported again, and that the desktop option is there when the hosted playmat is full.
- [ ] **Privacy page is linkable and complete**
  The privacy page has its own stable address, and states what is collected at sign-in, what is kept while playing, how long each is kept, and what is never collected.
- [ ] **Notice on every page**
  Every page of the site shows the unofficial fan project notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit.
- [ ] **Developer section links out**
  The site's developer section links to the source repository and contains no build or test instructions of its own beyond a pointer.
- [ ] **README links to the site**
  The repository README contains a link to the site's home page and to its connecting instructions, and no longer contains the step-by-step connecting instructions itself.
- [ ] **No horizontal scroll on a phone**
  At a 375-pixel-wide viewport, no page scrolls horizontally, and long code, addresses and prompts wrap or scroll within their own box.
- [ ] **One-action copy**
  Each example prompt, the hosted playmat's address and the example game have a copy control; activating it puts exactly that text on the clipboard.
- [ ] **No cookies or third-party requests**
  Loading every page in a fresh browser profile sets no cookies and makes no requests to any host other than the site's own.
- [ ] **Merged changes go live**
  After a change to the site's content is accepted into the project, the live site shows the change within 15 minutes with no manual step.

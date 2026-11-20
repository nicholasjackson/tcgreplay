# Demo video script

The demo video OpenAI's review asks for. It walks the five positive test
cases in `listing.json` in order, then one negative case, so recording the
video and running the test cases are the same rehearsal.

Before recording:

- Use the reviewer's Google account, the one set as `TCGREPLAY_REVIEWER`
  on the hosted playmat. It must have no two-step verification, or OpenAI's
  reviewer cannot sign in.
- Open the example game at https://tcgreplay.app/docs/example in another
  tab. Its decklist and battle log are what the prompts paste.
- Start from a new ChatGPT chat with TCG Replay not yet connected.

## Shots

1. **Connect and sign in.** In the new chat, add TCG Replay from the
   plugins menu. ChatGPT opens the Google sign-in; sign in with the
   reviewer account and return to the chat.
2. **Import the example game** (positive case 1). Copy the decklist and the
   battle log from the example page and send them with:
   "Here's a game from TCG Live and my decklist. Import it and review it
   with me." Show the import summary and the board link.
3. **Open the board** (positive case 2). Click the board link: it opens
   signed in, showing the game. Back in the chat: "Open the board and tell
   me what the position was at the start of my turn 3."
4. **Review a turn** (positive case 3). "Review my turn 2 with me: what
   should I have done differently? Put your notes on the board." Switch to
   the board and show the coaching notes on the timeline.
5. **Replay a turn** (positive case 4). "Rewind to the start of my turn 3 so
   I can play it differently on the board." Play a card or two on the board,
   end the turn, and show the assistant answering in the same reply.
6. **Keep and restore the game** (positive case 5). "Give me the record of
   this game so I can keep it, then restore the game from it." Show the
   record and the rebuilt game.
7. **A request it hands back** (negative case 1). In a new chat with TCG
   Replay connected, paste only the battle log: "Here's my TCG Live battle
   log, can you review it?" Show the assistant asking for the decklist and
   importing nothing.

Upload the video somewhere OpenAI's reviewers can watch it without signing
in, then follow step 6 of `SUBMITTING.md`.

// Copy controls: every element marked data-copy gets a Copy button that
// puts exactly that element's text on the clipboard, and says so to
// screen readers. Loaded once by the shared layout.
//
// The button carries the copy icon. In a terminal box or beside a prompt
// bubble it is icon-only (data-copy-icon), named by data-copy-name ("Copy
// the command", "Copy the prompt"), and its word shows only to confirm
// "Copied". Elsewhere (an address, a copy panel) it reads "Copy" beside the
// icon and is named "Copy <label>". If the clipboard is refused, the text
// is selected instead, so it can be copied by hand.
const live = document.createElement('div');
live.className = 'sr-only';
live.setAttribute('aria-live', 'polite');
document.body.append(live);

const icon =
  '<svg class="copy-icon" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><rect x="9" y="9" width="11" height="11" rx="2"></rect><path d="M5 15V5a1 1 0 0 1 1-1h9"></path></svg>';

// Code blocks in the docs' Markdown are copyable too: each gets the same
// box as a CopyBlock, copying its text without the final newline. A
// ```prompt fence is an example prompt to say to the assistant: it wraps.
// Other code sits in a dark box, as a CopyBlock's code does.
for (const pre of document.querySelectorAll<HTMLElement>('.prose pre:not([data-copy])')) {
  const prompt = pre.querySelector('code.language-prompt') !== null;
  const block = document.createElement('div');
  const text = (pre.textContent ?? '').replace(/\n$/, '');
  // A one-line command wraps in its box; longer code keeps its lines.
  block.className = prompt ? 'copy-block wrap prompt' : text.includes('\n') ? 'copy-block' : 'copy-block command';
  if (prompt) {
    const label = document.createElement('span');
    label.className = 'label';
    label.textContent = 'Example prompt';
    block.append(label);
  }
  const box = document.createElement('div');
  box.className = 'copy-box';
  if (!prompt) box.dataset.theme = 'dark';
  pre.replaceWith(block);
  box.append(pre);
  block.append(box);
  pre.dataset.copy = '';
  pre.dataset.copyText = text;
  pre.dataset.copyIcon = '';
  pre.dataset.copyName = prompt ? 'Copy the prompt' : text.includes('\n') ? 'Copy the code' : 'Copy the command';
}

for (const el of document.querySelectorAll<HTMLElement>('[data-copy]')) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy';
  const iconOnly = el.dataset.copyIcon !== undefined;
  if (iconOnly) button.classList.add('icon');
  button.innerHTML = `${icon}<span class="copy-text">Copy</span>`;
  const word = button.querySelector<HTMLElement>('.copy-text')!;
  const what = el.dataset.copyLabel;
  const name = el.dataset.copyName ?? (what ? `Copy ${what}` : 'Copy');
  button.setAttribute('aria-label', name);
  let timer: ReturnType<typeof setTimeout> | undefined;
  button.addEventListener('click', async () => {
    const text = el.dataset.copyText ?? el.textContent ?? '';
    try {
      await navigator.clipboard.writeText(text);
      word.textContent = 'Copied';
      button.dataset.copied = '';
      live.textContent = what ? `Copied ${what}` : 'Copied';
    } catch {
      const range = document.createRange();
      range.selectNodeContents(el);
      const selection = getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
      word.textContent = 'Selected';
      button.dataset.copied = '';
      live.textContent = 'Copy failed: the text is selected, copy it yourself';
    }
    clearTimeout(timer);
    timer = setTimeout(() => {
      word.textContent = 'Copy';
      delete button.dataset.copied;
    }, 2000);
  });
  (el.closest('.copy-box') ?? el.parentElement ?? el).append(button);
}

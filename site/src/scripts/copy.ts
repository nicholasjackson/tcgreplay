// Copy controls: every element marked data-copy gets a Copy button that
// puts exactly that element's text on the clipboard, and says so to
// screen readers. Loaded once by the shared layout.
const live = document.createElement('div');
live.className = 'sr-only';
live.setAttribute('aria-live', 'polite');
document.body.append(live);

// Code blocks in the docs' Markdown are copyable too: each gets the same
// box as a CopyBlock, copying its text without the final newline. A
// ```prompt fence is an example prompt to say to the assistant: it wraps.
for (const pre of document.querySelectorAll<HTMLElement>('.prose pre:not([data-copy])')) {
  const prompt = pre.querySelector('code.language-prompt') !== null;
  const block = document.createElement('div');
  block.className = prompt ? 'copy-block wrap prompt' : 'copy-block';
  if (prompt) {
    const label = document.createElement('span');
    label.className = 'label';
    label.textContent = 'Example prompt';
    block.append(label);
  }
  const box = document.createElement('div');
  box.className = 'copy-box';
  pre.replaceWith(block);
  box.append(pre);
  block.append(box);
  pre.dataset.copy = '';
  pre.dataset.copyText = (pre.textContent ?? '').replace(/\n$/, '');
  if (prompt) pre.dataset.copyLabel = 'Example prompt';
}

for (const el of document.querySelectorAll<HTMLElement>('[data-copy]')) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy';
  button.textContent = 'Copy';
  const what = el.dataset.copyLabel;
  button.setAttribute('aria-label', what ? `Copy ${what}` : 'Copy');
  let timer: ReturnType<typeof setTimeout> | undefined;
  button.addEventListener('click', async () => {
    const text = el.dataset.copyText ?? el.textContent ?? '';
    try {
      await navigator.clipboard.writeText(text);
      button.textContent = 'Copied';
      button.dataset.copied = '';
      live.textContent = what ? `Copied ${what}` : 'Copied';
    } catch {
      button.textContent = 'Copy failed';
      live.textContent = 'Copy failed: select the text and copy it yourself';
    }
    clearTimeout(timer);
    timer = setTimeout(() => {
      button.textContent = 'Copy';
      delete button.dataset.copied;
    }, 2000);
  });
  (el.closest('.copy-box') ?? el.parentElement ?? el).append(button);
}

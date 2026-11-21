// On this page: marks the link to the section in view with
// aria-current="true", in the docs rail and in the Privacy and Terms card
// (any nav marked data-toc). The section in view is the last one whose
// heading has reached the top quarter of the window, or the first while
// none has. Loaded once by the shared layout; it changes nothing on a page
// without such a list.
for (const nav of document.querySelectorAll<HTMLElement>('nav[data-toc]')) {
  const links = [...nav.querySelectorAll<HTMLAnchorElement>('a[href^="#"]')];
  const targets = links.map((a) => document.getElementById(decodeURIComponent(a.hash.slice(1))));
  if (links.length === 0 || targets.some((t) => t === null)) continue;
  let current = -1;
  const mark = () => {
    const line = innerHeight / 4;
    let i = 0;
    targets.forEach((t, n) => {
      if (t!.getBoundingClientRect().top <= line) i = n;
    });
    // At the foot of the page the last sections can never reach the line.
    if (innerHeight + scrollY >= document.documentElement.scrollHeight - 2 && scrollY > 0) i = targets.length - 1;
    if (i === current) return;
    current = i;
    links.forEach((a, n) => (n === i ? a.setAttribute('aria-current', 'true') : a.removeAttribute('aria-current')));
  };
  mark();
  addEventListener('scroll', mark, { passive: true });
}

// The mounted half of CopyLink (CopyButton.test.ts covers the shared
// copyText mechanics; these assert the component's own behaviour): clicking
// copies the page's URL, confirms inline, and degrades honestly through the
// fallback when the Clipboard API rejects.
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import CopyLink from './CopyLink';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderCopyLink() {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root!.render(<CopyLink />);
  });
}

function button(): HTMLButtonElement {
  const btn = container!.querySelector('[data-testid="copy-link"]');
  if (!(btn instanceof HTMLButtonElement)) {
    throw new Error('copy-link button not rendered');
  }
  return btn;
}

afterEach(() => {
  vi.unstubAllGlobals();
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  delete (document as any).execCommand;
  const r = root;
  if (r !== null && container !== null) {
    act(() => {
      r.unmount();
    });
  }
  container?.remove();
  container = null;
  root = null;
});

describe('CopyLink (mounted)', () => {
  it('copies the page URL and confirms inline', async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    await renderCopyLink();

    const btn = button();
    expect(btn.textContent).toContain('Copy link');
    await act(async () => {
      btn.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith(window.location.href);
    expect(button().textContent).toContain('Copied');
  });

  // Insecure/embedded contexts: the Clipboard API rejects and the textarea
  // + execCommand fallback carries the copy. jsdom has no execCommand, so
  // both are stubbed, exactly as CopyButton.test.ts does.
  it('falls back to execCommand when the Clipboard API rejects', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => { throw new Error('denied'); }) } });
    const execCommand = vi.fn(() => true);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = execCommand;
    await renderCopyLink();

    await act(async () => {
      button().dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(button().textContent).toContain('Copied');
  });

  it('says Copy failed when neither path can copy', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => { throw new Error('denied'); }) } });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = vi.fn(() => false);
    await renderCopyLink();

    await act(async () => {
      button().dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(button().textContent).toContain('Copy failed');
  });
});

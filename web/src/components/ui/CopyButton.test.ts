import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyReducer, copyText } from './CopyButton';

describe('copyReducer', () => {
  it('confirms on succeeded', () => {
    expect(copyReducer('idle', { type: 'succeeded' })).toBe('copied');
  });

  it('marks failure on failed', () => {
    expect(copyReducer('idle', { type: 'failed' })).toBe('failed');
  });

  it('resets to idle from both terminal states', () => {
    expect(copyReducer('copied', { type: 'reset' })).toBe('idle');
    expect(copyReducer('failed', { type: 'reset' })).toBe('idle');
  });

  // While a copy is in flight the button goes back to idle so a second
  // click mid-flight doesn't show a stale "Copied".
  it('goes idle on a fresh copy action', () => {
    expect(copyReducer('copied', { type: 'copy' })).toBe('idle');
  });
});

describe('copyText', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    delete (document as any).execCommand;
  });

  it('uses the Clipboard API when available, copying the exact raw value', async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    await expect(copyText('trace-abc')).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText).toHaveBeenCalledWith('trace-abc');
  });

  // Insecure/embedded contexts: navigator.clipboard is missing or rejects,
  // and the fallback selects the value into an off-screen textarea and
  // execCommand("copy")es it. jsdom has no execCommand, so both are stubbed.
  it('falls back to textarea + execCommand when the Clipboard API rejects', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn(async () => { throw new Error('denied'); }) } });
    const execCommand = vi.fn(() => true);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = execCommand;

    await expect(copyText('trace-abc')).resolves.toBe(true);

    expect(execCommand).toHaveBeenCalledTimes(1);
    expect(execCommand).toHaveBeenCalledWith('copy');
  });

  it('resolves false when the Clipboard API is absent and execCommand fails', async () => {
    vi.stubGlobal('navigator', {});
    const execCommand = vi.fn(() => false);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = execCommand;

    await expect(copyText('trace-abc')).resolves.toBe(false);
  });
});

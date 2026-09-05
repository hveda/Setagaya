// StageEditor's mounted tests (Execution.live.test.tsx's createRoot + act
// pattern; no testing-library in the house deps). A tiny harness owns the
// state object the same way NewTest will, recording every onStateChange
// emission and validity transition so the controlled-component contract is
// what gets asserted, not incidental DOM.
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { useState } from 'react';
import StageEditor, { type StageEditorState } from './StageEditor';
import { stagesToConfig, type StageRow } from '../lib/stagesConfig';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

const rowA: StageRow = { name: 'checkout-smoke', scenarioId: 0, concurrency: 50, engines: 2, rampup: 30, duration: 300 };
const rowB: StageRow = { name: 'checkout-smoke', scenarioId: 0, concurrency: 10, engines: 1, rampup: 0, duration: 60, throughput: 125 };

const tableState: StageEditorState = { mode: 'table', rows: [rowA, rowB], rawJson: '' };

/** Records every emission; the last entry is the current value. */
let states: StageEditorState[] = [];
let validity: boolean[] = [];

function Harness({ initial }: { initial: StageEditorState }) {
  const [state, setState] = useState<StageEditorState>(initial);
  return (
    <StageEditor
      state={state}
      onStateChange={(s) => {
        states.push(s);
        setState(s);
      }}
      configName="checkout-smoke-load"
      onValidityChange={(v) => validity.push(v)}
    />
  );
}

let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderEditor(initial: StageEditorState = tableState) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root!.render(<Harness initial={initial} />);
  });
}

afterEach(() => {
  const r = root;
  if (r !== null && container !== null) {
    act(() => {
      r.unmount();
    });
  }
  container?.remove();
  container = null;
  root = null;
  states = [];
  validity = [];
});

/** jsdom input driving: the native value setter + input event (React's value tracker ignores plain .value writes). */
async function type(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

async function click(el: Element) {
  await act(async () => {
    el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
}

const lastState = () => states[states.length - 1];
const q = (sel: string) => container!.querySelector(sel) as HTMLInputElement | null;

describe('StageEditor table mode', () => {
  it('renders one input row per stage with the five editable fields', async () => {
    await renderEditor();
    expect(q('[aria-label="stage 1 concurrency"]')?.value).toBe('50');
    expect(q('[aria-label="stage 1 ramp-up"]')?.value).toBe('30');
    expect(q('[aria-label="stage 1 engines"]')?.value).toBe('2');
    expect(q('[aria-label="stage 1 throughput"]')?.value).toBe('');
    expect(q('[aria-label="stage 2 throughput"]')?.value).toBe('125');
    expect(q('[aria-label="stage 2 duration"]')?.value).toBe('60');
    expect(q('[aria-label="remove stage 2"]')).not.toBeNull();
  });

  it('routes edits through onStateChange without touching other rows', async () => {
    await renderEditor();
    await type(q('[aria-label="stage 1 concurrency"]')!, '80');
    expect(lastState().rows[0].concurrency).toBe(80);
    expect(lastState().rows[1].concurrency).toBe(10);
  });

  it('treats an emptied throughput as unlimited (undefined)', async () => {
    await renderEditor();
    await type(q('[aria-label="stage 2 throughput"]')!, '');
    expect(lastState().rows[1].throughput).toBeUndefined();
    await type(q('[aria-label="stage 2 throughput"]')!, '120');
    expect(lastState().rows[1].throughput).toBe(120);
  });

  it('adds a copy of the last row, removes rows, and keeps at least one', async () => {
    await renderEditor({ mode: 'table', rows: [rowA], rawJson: '' });
    await click(q('[aria-label="add stage"]')!);
    expect(lastState().rows).toHaveLength(2);
    expect(lastState().rows[1]).toEqual(rowA);
    await click(q('[aria-label="remove stage 2"]')!);
    expect(lastState().rows).toHaveLength(1);
    // The last remaining row cannot be removed.
    expect(q('[aria-label="remove stage 1"]')! as unknown as HTMLButtonElement).toHaveProperty('disabled', true);
  });

  it('shows Entry.Validate-mirrored copy inline under the offending inputs', async () => {
    await renderEditor({
      mode: 'table',
      rows: [{ ...rowA, concurrency: 0, engines: 0, duration: 0, throughput: -5 }],
      rawJson: '',
    });
    expect(q('[data-testid="stage-1-concurrency-error"]')?.textContent).toBe('concurrency must be positive');
    expect(q('[data-testid="stage-1-engines-error"]')?.textContent).toBe('engines must be positive');
    expect(q('[data-testid="stage-1-duration-error"]')?.textContent).toBe('duration must be positive');
    expect(q('[data-testid="stage-1-throughput-error"]')?.textContent).toBe('throughput cannot be negative');
  });

  it('surfaces validity upward: invalid rows -> false, fixed -> true; a pending scenario does not block the table', async () => {
    await renderEditor({ mode: 'table', rows: [{ ...rowA, concurrency: 0 }], rawJson: '' });
    expect(validity[validity.length - 1]).toBe(false);
    await type(q('[aria-label="stage 1 concurrency"]')!, '25');
    expect(validity[validity.length - 1]).toBe(true);
    // scenarioId is 0 here -- the flow assigns it at submit, so table
    // mode stays valid (raw mode owns the scenario check).
    expect(lastState().rows[0].scenarioId).toBe(0);
  });
});

describe('StageEditor raw mode', () => {
  it('serializes the current rows pretty-printed (2-space) on the switch to JSON', async () => {
    await renderEditor();
    await click(q('[aria-label="switch to JSON"]')!);
    const area = q('[aria-label="stage config JSON"]')!;
    expect(area.value).toBe(JSON.stringify(stagesToConfig([rowA, rowB], 'checkout-smoke-load', 0, 0), null, 2));
    expect(area.value).toContain('\n  "name": "checkout-smoke-load",');
    expect(lastState().mode).toBe('raw');
  });

  it('parses edited JSON back into rows on the switch to Table', async () => {
    await renderEditor();
    await click(q('[aria-label="switch to JSON"]')!);
    const area = q('[aria-label="stage config JSON"]')!;
    await type(area, area.value.replace('"concurrency": 50', '"concurrency": 80'));
    await click(q('[aria-label="switch to table"]')!);
    expect(lastState().mode).toBe('table');
    expect(lastState().rows[0].concurrency).toBe(80);
    expect(q('[aria-label="stage 1 concurrency"]')?.value).toBe('80');
  });

  it('shows a parse error and does NOT destroy the previous table state', async () => {
    await renderEditor();
    await click(q('[aria-label="switch to JSON"]')!);
    const area = q('[aria-label="stage config JSON"]')!;
    await type(area, '{oops');
    expect(q('[data-testid="raw-json-error"]')?.textContent).toContain('not valid JSON');
    expect(validity[validity.length - 1]).toBe(false);
    // Attempting to switch back stays in raw mode with rows untouched.
    await click(q('[aria-label="switch to table"]')!);
    expect(lastState().mode).toBe('raw');
    expect(lastState().rows).toEqual([rowA, rowB]);
    // Repairing the JSON lets the switch through.
    await type(area, JSON.stringify(stagesToConfig([rowA], 'checkout-smoke-load', 0, 0)));
    await click(q('[aria-label="switch to table"]')!);
    expect(lastState().mode).toBe('table');
    expect(lastState().rows).toEqual([rowA]);
  });

  it('rejects parseable JSON without a tests array', async () => {
    await renderEditor();
    await click(q('[aria-label="switch to JSON"]')!);
    await type(q('[aria-label="stage config JSON"]')!, '{"name": "x"}');
    expect(q('[data-testid="raw-json-error"]')?.textContent).toContain('tests');
    expect(validity[validity.length - 1]).toBe(false);
  });

  it('validates the full config in raw mode, including the scenario id', async () => {
    await renderEditor();
    await click(q('[aria-label="switch to JSON"]')!);
    const area = q('[aria-label="stage config JSON"]')!;
    // scenario_id 0 is editable here, so the Entry.Validate mirror fires.
    expect(q('[data-testid="raw-row-errors"]')?.textContent).toContain('stage 1: scenario required');
    expect(validity[validity.length - 1]).toBe(false);
    await type(area, area.value.replaceAll('"scenario_id": 0', '"scenario_id": 42'));
    expect(q('[data-testid="raw-row-errors"]')).toBeNull();
    expect(validity[validity.length - 1]).toBe(true);
  });
});

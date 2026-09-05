// The visual stage editor for a load config (phase 22, G2): a table of
// stage rows (concurrency / ramp-up / engines / throughput / duration)
// with a raw-JSON escape hatch. Fully controlled through a single state
// object {mode, rows, rawJson} -- the host (NewTest) owns it exactly the
// way it owns any other form field. The wire contract is
// lib/stagesConfig's, which is byte-equal to newTestFlow.buildConfig for
// the single-row case.
//
// Validation mirrors loadprofile.Entry.Validate (validateStageRow), shown
// inline under the offending input; nothing is blocked at typing time.
// In table mode only the editable fields count toward validity -- the
// scenario id is assigned by the host flow at submit -- while raw mode
// owns the full check (scenario_id is user-editable there).
import { useEffect } from 'react';
import Button from './ui/Button';
import {
  configToStages,
  editableRowsValid,
  stageRowsValid,
  stagesToConfig,
  validateStageRow,
  type StageRow,
  type StagesConfigJSON,
} from '../lib/stagesConfig';

/** The whole editor's state, owned by the host; rawJson persists across mode switches. */
export interface StageEditorState {
  mode: 'table' | 'raw';
  rows: StageRow[];
  rawJson: string;
}

export interface StageEditorProps {
  state: StageEditorState;
  onStateChange: (next: StageEditorState) => void;
  /** Wrapper name used when serializing rows into raw JSON. */
  configName: string;
  /** Wrapper ids for the raw JSON; 0 placeholders are patched by the flow at submit. */
  projectId?: number;
  executionId?: number;
  /** Fires whenever this mode's submittability changes (see file comment). */
  onValidityChange?: (valid: boolean) => void;
}

type RawParse = { ok: true; cfg: StagesConfigJSON } | { ok: false; error: string };

/** Parse + shape-check the raw textarea; never throws. */
function parseRawConfig(raw: string): RawParse {
  if (raw.trim() === '') {
    return { ok: false, error: 'config JSON is empty' };
  }
  try {
    const cfg = JSON.parse(raw) as StagesConfigJSON;
    if (!Array.isArray(cfg?.tests)) {
      return { ok: false, error: 'config JSON must have a "tests" array' };
    }
    // Entries must be objects: a primitives-filled tests[] would slip
    // past validateStageRow (undefined-field comparisons are all false)
    // and report the editor valid for garbage.
    if (!cfg.tests.every((t) => typeof t === 'object' && t !== null)) {
      return { ok: false, error: 'each tests[] entry must be an object' };
    }
    return { ok: true, cfg };
  } catch (err: unknown) {
    return { ok: false, error: `not valid JSON: ${err instanceof Error ? err.message : String(err)}` };
  }
}

const inputCls =
  'rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm text-slate-900 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100';

/** Segmented-toggle classes; active state readable via aria-pressed. */
const tabCls = (active: boolean) =>
  [
    'px-3 py-1.5 text-sm font-medium transition-colors',
    active
      ? 'bg-slate-200 text-slate-900 dark:bg-slate-700 dark:text-white'
      : 'text-slate-600 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800',
  ].join(' ');

export default function StageEditor({
  state,
  onStateChange,
  configName,
  projectId = 0,
  executionId = 0,
  onValidityChange,
}: StageEditorProps) {
  const rows = state.rows;

  const patchRow = (i: number, patch: Partial<StageRow>) =>
    onStateChange({ ...state, rows: rows.map((r, j) => (j === i ? { ...r, ...patch } : r)) });
  // A new stage usually steps from the previous one, so add duplicates
  // the last row (the user then tunes it).
  const addRow = () => onStateChange({ ...state, rows: [...rows, { ...rows[rows.length - 1] }] });
  const removeRow = (i: number) => {
    if (rows.length <= 1) {
      return;
    }
    onStateChange({ ...state, rows: rows.filter((_, j) => j !== i) });
  };

  const setMode = (mode: 'table' | 'raw') => {
    if (mode === state.mode) {
      return;
    }
    if (mode === 'raw') {
      onStateChange({
        mode: 'raw',
        rows, // kept: a later parse error must not destroy the table state
        rawJson: JSON.stringify(stagesToConfig(rows, configName, projectId, executionId), null, 2),
      });
      return;
    }
    const parsed = parseRawConfig(state.rawJson);
    if (!parsed.ok) {
      // Stay in raw mode; the live parse error under the textarea names
      // the problem, and the untouched rows survive for when it's fixed.
      return;
    }
    onStateChange({ mode: 'table', rows: configToStages(parsed.cfg), rawJson: state.rawJson });
  };

  // Validity per mode (file comment): table checks the editable fields,
  // raw checks the parsed config in full including scenario ids.
  const rawParse = state.mode === 'raw' ? parseRawConfig(state.rawJson) : null;
  const rawRows = rawParse?.ok ? configToStages(rawParse.cfg) : [];
  const valid = state.mode === 'table' ? editableRowsValid(rows) : rawParse?.ok === true && stageRowsValid(rawRows);

  useEffect(() => {
    // Deps are [valid] on purpose: re-notifying on every parent render
    // would spam; the boolean only changes when submittability does.
    onValidityChange?.(valid);
  }, [valid]);

  const numberField = (
    i: number,
    field: 'concurrency' | 'rampup' | 'engines' | 'duration',
    label: string,
  ) => {
    const row = rows[i];
    // rampup carries no Entry.Validate rule, hence the string lookup.
    const err = (validateStageRow(row) as Record<string, string | undefined>)[field];
    return (
      <td className="px-3 py-2 align-top">
        <input
          type="number"
          className={`${inputCls} w-24`}
          aria-label={`stage ${i + 1} ${label}`}
          value={row[field]}
          onChange={(e) =>
            patchRow(i, { [field]: e.target.value === '' ? 0 : Number(e.target.value) } as Partial<StageRow>)
          }
        />
        {err && (
          <p className="mt-1 text-xs text-red-600 dark:text-red-400" role="alert" data-testid={`stage-${i + 1}-${field}-error`}>
            {err}
          </p>
        )}
      </td>
    );
  };

  return (
    <div className="space-y-3" data-testid="stage-editor">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div
          role="group"
          aria-label="stage editor mode"
          className="inline-flex overflow-hidden rounded-lg border border-slate-300 dark:border-slate-700"
        >
          <button type="button" aria-pressed={state.mode === 'table'} aria-label="switch to table" onClick={() => setMode('table')} className={tabCls(state.mode === 'table')}>
            Table
          </button>
          <button type="button" aria-pressed={state.mode === 'raw'} aria-label="switch to JSON" onClick={() => setMode('raw')} className={tabCls(state.mode === 'raw')}>
            JSON
          </button>
        </div>
        {state.mode === 'table' && (
          <Button variant="ghost" aria-label="add stage" onClick={addRow}>
            + Add stage
          </Button>
        )}
      </div>

      {state.mode === 'table' ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-body-sm" data-testid="stage-table">
            <thead>
              <tr className="text-caption border-b border-slate-200 text-slate-500 dark:border-slate-700 dark:text-slate-400">
                {['Concurrency', 'Ramp-up (s)', 'Engines', 'Throughput (req/s)', 'Duration (s)', ''].map((h) => (
                  <th key={h} scope="col" className="px-3 py-2 font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {rows.map((row, i) => {
                const errs = validateStageRow(row);
                return (
                  <tr key={i}>
                    {numberField(i, 'concurrency', 'concurrency')}
                    {numberField(i, 'rampup', 'ramp-up')}
                    {numberField(i, 'engines', 'engines')}
                    <td className="px-3 py-2 align-top">
                      {/* Empty throughput = unlimited: the key is omitted
                          from the JSON (loadprofile.Throughput omitempty). */}
                      <input
                        type="number"
                        className={`${inputCls} w-24`}
                        aria-label={`stage ${i + 1} throughput`}
                        placeholder="unlimited"
                        value={row.throughput ?? ''}
                        onChange={(e) => {
                          const v = e.target.value;
                          if (v === '') {
                            patchRow(i, { throughput: undefined });
                            return;
                          }
                          const n = Number(v);
                          if (Number.isFinite(n)) {
                            patchRow(i, { throughput: n });
                          }
                        }}
                      />
                      {errs.throughput && (
                        <p className="mt-1 text-xs text-red-600 dark:text-red-400" role="alert" data-testid={`stage-${i + 1}-throughput-error`}>
                          {errs.throughput}
                        </p>
                      )}
                    </td>
                    {numberField(i, 'duration', 'duration')}
                    <td className="px-3 py-2 align-top">
                      <Button
                        variant="ghost"
                        aria-label={`remove stage ${i + 1}`}
                        disabled={rows.length <= 1}
                        onClick={() => removeRow(i)}
                      >
                        ✕
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="text-caption mt-2 text-slate-500 dark:text-slate-400">
            Empty throughput = unlimited (omitted from the JSON). Scenario and test name are assigned by the flow that creates them.
          </p>
        </div>
      ) : (
        <div>
          <textarea
            aria-label="stage config JSON"
            data-testid="stage-json"
            className={`${inputCls} h-64 w-full font-mono text-xs`}
            spellCheck={false}
            value={state.rawJson}
            onChange={(e) => onStateChange({ ...state, rawJson: e.target.value })}
          />
          {rawParse && !rawParse.ok && (
            <p className="mt-2 text-sm text-red-600 dark:text-red-400" role="alert" data-testid="raw-json-error">
              {rawParse.error}
            </p>
          )}
          {rawParse?.ok && !stageRowsValid(rawRows) && (
            <ul className="mt-2 space-y-1 text-sm text-red-600 dark:text-red-400" data-testid="raw-row-errors">
              {rawRows.flatMap((row, i) =>
                Object.values(validateStageRow(row)).map((msg) => (
                  <li key={`${i}-${msg}`} role="alert">
                    stage {i + 1}: {msg}
                  </li>
                )),
              )}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

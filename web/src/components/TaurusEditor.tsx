import { useEffect, useRef, useState } from 'react';
import { EditorView, basicSetup } from 'codemirror';
import { yaml } from '@codemirror/lang-yaml';
import Button from './ui/Button';
import { ApiError } from '../api/client';
import { getScenarioRequests, setScenarioRequests, validateScenarioRequests, type Diagnostic } from '../api/scenarios';
import { parseDocument } from 'yaml';
import { applyFields, readFields, type FragmentFields } from '../lib/taurusDoc';

interface TaurusEditorProps {
  scenarioId: number;
}

/**
 * The Taurus fragment editor (R4 shell): a CodeMirror 6 surface over the
 * scenario's YAML fragment, loaded verbatim via G2 and saved verbatim via
 * G3. Byte-exactness is the contract -- the editor edits TEXT, and every
 * later feature (R5's form lens, R6's validation) reads and patches the
 * same text rather than a reserialized model.
 */
export default function TaurusEditor({ scenarioId }: TaurusEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);
  const [formFields, setFormFields] = useState<FragmentFields | null>(null);
  // Two validation layers (R6): parseError is client-side (no network),
  // serverDiags carry G4 errors and G6 info notes from the debounced G5
  // call. info notes never block saving; errors surface and Save disables.
  const [parseError, setParseError] = useState<Diagnostic | null>(null);
  const [serverDiags, setServerDiags] = useState<Diagnostic[]>([]);
  const [validating, setValidating] = useState(false);
  const validateTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // One EditorView per mount; re-pointed at each scenario via dispatch.
  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError(null);
    setSavedAt(null);
    setDirty(false);
    getScenarioRequests(scenarioId)
      .then((doc) => {
        if (!alive || !hostRef.current) {
          return;
        }
        viewRef.current?.destroy();
        const view = new EditorView({
          doc,
          extensions: [basicSetup, yaml(), EditorView.lineWrapping],
          parent: hostRef.current,
          dispatch: (tr) => {
            const v = viewRef.current;
            if (v) {
              v.update([tr]);
              if (tr.docChanged) {
                setDirty(true);
                runValidation(v.state.doc.toString());
              }
            }
          },
        });
        viewRef.current = view;
      })
      .catch((err: unknown) => {
        if (alive) setError(err instanceof ApiError ? err.message : 'Failed to load fragment.');
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
      if (validateTimerRef.current) {
        clearTimeout(validateTimerRef.current);
      }
      viewRef.current?.destroy();
      viewRef.current = null;
    };
  }, [scenarioId]);

  // Open the form lens: reads the CURRENT editor text (not the saved
  // copy) so the form always starts from what is on screen.
  const openForm = () => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    setFormFields(readFields(view.state.doc.toString()));
    setFormOpen(true);
  };

  // Apply the form back through the AST lens and swap the editor doc.
  // The lens rewrites only mapped nodes; everything else survives.
  const applyForm = (fields: FragmentFields) => {
    const view = viewRef.current;
    if (!view) {
      return;
    }
    try {
      const next = applyFields(view.state.doc.toString(), fields);
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: next } });
      setDirty(true);
      setFormOpen(false);
      setError(null);
      runValidation(next);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Cannot apply form to this YAML.');
    }
  };

  /**
   * Layer 1 -- parse check, runs synchronously on every change with NO
   * network call. yaml.parseDocument collects errors instead of throwing;
   * the first one carries a line-anchored message (prettyErrors on by
   * default embeds "line N" in the text).
   */
  const checkParse = (text: string): Diagnostic | null => {
    const doc = parseDocument(text);
    if (doc.errors.length === 0) {
      return null;
    }
    const e = doc.errors[0];
    const m = /line\s+(\d+)/i.exec(e.message);
    return {
      severity: 'error',
      message: e.message.replace(/line \d+:\s*/i, ''),
      line: m ? parseInt(m[1], 10) : 1,
    };
  };

  /** Layer 2 -- G5 on a debounce. 800ms quiet-time; one in-flight timer. */
  const validate = (text: string) => {
    if (validateTimerRef.current) {
      clearTimeout(validateTimerRef.current);
    }
    validateTimerRef.current = setTimeout(() => {
      setValidating(true);
      validateScenarioRequests(scenarioId, text)
        .then((res) => {
          setServerDiags(res.diagnostics);
          if (res.valid) {
            setError(null);
          }
        })
        .catch((err: unknown) => {
          // Transport-level failures (404 scenario, network): surfaced in
          // the banner, never silently swallowed.
          setError(err instanceof ApiError ? err.message : 'Validation failed.');
        })
        .finally(() => setValidating(false));
    }, 800);
  };

  /** Runs both layers on the current doc; called from every mutation path. */
  const runValidation = (text: string) => {
    const pe = checkParse(text);
    setParseError(pe);
    if (pe) {
      // Broken YAML: layer 2 is pointless -- clear stale server notes and
      // cancel any pending debounce (no network call, per the contract).
      setServerDiags([]);
      if (validateTimerRef.current) {
        clearTimeout(validateTimerRef.current);
      }
      return;
    }
    validate(text);
  };

  const save = () => {
    const view = viewRef.current;
    if (!view || saving || parseError) {
      return;
    }
    setSaving(true);
    setError(null);
    // The doc's string form is what goes on the wire -- no normalization,
    // no pretty-print. The backend validates and stores byte-for-byte.
    setScenarioRequests(scenarioId, view.state.doc.toString())
      .then(() => {
        setDirty(false);
        setSavedAt(new Date().toLocaleTimeString());
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Save failed.');
      })
      .finally(() => setSaving(false));
  };

  const reload = () => {
    setDirty(false);
    // Re-point the effect by keying off a state nudge: simplest honest
    // reload is re-running the load effect via scenarioId change -- but
    // since it hasn't changed, fetch and swap the doc directly.
    getScenarioRequests(scenarioId)
      .then((doc) => {
        const view = viewRef.current;
        if (view) {
          view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: doc } });
        }
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof ApiError ? err.message : 'Reload failed.');
      });
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={save} disabled={!dirty || saving || loading || parseError !== null}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
        <Button variant="outline" onClick={reload} disabled={loading}>
          Reload
        </Button>
        <Button variant="ghost" onClick={formOpen ? () => setFormOpen(false) : openForm} disabled={loading}>
          {formOpen ? 'Hide form' : 'Edit fields'}
        </Button>
        <span className="text-caption text-slate-500 dark:text-slate-400">
          {loading ? 'Loading…' : savedAt ? `Saved at ${savedAt}` : dirty ? 'Unsaved changes' : 'Up to date'}
        </span>
      </div>
      {error && (
        <p className="text-sm text-red-600 dark:text-red-400" role="alert">
          {error}
        </p>
      )}
      {parseError && (
        <p className="rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-300" role="alert">
          Parse error at line {parseError.line}: {parseError.message}
        </p>
      )}
      {(serverDiags.length > 0 || validating) && !parseError && (
        <div className="rounded-md bg-slate-50 p-3 text-sm dark:bg-slate-800" role="status">
          <p className="mb-1 font-medium text-slate-700 dark:text-slate-300">
            Validation {validating ? 'checking…' : 'results'}
          </p>
          <ul className="space-y-1">
            {serverDiags.map((d, i) => (
              <li
                key={i}
                className={
                  d.severity === 'error'
                    ? 'text-red-600 dark:text-red-400'
                    : 'text-sky-700 dark:text-sky-400'
                }
              >
                <span className="font-mono">{d.line > 0 ? `line ${d.line}` : 'doc'}</span>{' '}
                {d.message}
                {d.severity === 'info' && (
                  <span className="text-slate-500 dark:text-slate-400">
                    {' '}(stored but not compiled — the key rides along; the engine ignores it)
                  </span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
      {formOpen && formFields && (
        <FormLens
          fields={formFields}
          onChange={setFormFields}
          onApply={() => applyForm(formFields)}
          onCancel={() => setFormOpen(false)}
        />
      )}
      <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700" ref={hostRef} />
    </div>
  );
}


/** The form lens panel: plain inputs bound to FragmentFields. */
function FormLens({
  fields,
  onChange,
  onApply,
  onCancel,
}: {
  fields: FragmentFields;
  onChange: (f: FragmentFields) => void;
  onApply: () => void;
  onCancel: () => void;
}) {
  const input =
    'rounded-md border border-slate-300 bg-white px-2 py-1.5 text-sm text-slate-900 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100';
  const setHeader = (i: number, patch: Partial<{ name: string; value: string }>) => {
    onChange({ ...fields, headers: fields.headers.map((h, j) => (j === i ? { ...h, ...patch } : h)) });
  };
  return (
    <div className="space-y-3 rounded-lg border border-slate-200 p-4 dark:border-slate-700">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label className="text-caption text-slate-600 dark:text-slate-300">
          Target URL (default-address)
          <input className={`${input} mt-1 w-full`} value={fields.defaultAddress} onChange={(e) => onChange({ ...fields, defaultAddress: e.target.value })} placeholder="http://checkout.svc" />
        </label>
        <label className="text-caption text-slate-600 dark:text-slate-300">
          Method
          <select className={`${input} mt-1 w-full`} value={fields.method} onChange={(e) => onChange({ ...fields, method: e.target.value })}>
            <option value="">(unchanged)</option>
            <option value="GET">GET</option>
            <option value="POST">POST</option>
            <option value="PUT">PUT</option>
            <option value="DELETE">DELETE</option>
            <option value="PATCH">PATCH</option>
            <option value="HEAD">HEAD</option>
          </select>
        </label>
      </div>
      <label className="text-caption text-slate-600 dark:text-slate-300 block">
        Path (appended to target)
        <input className={`${input} mt-1 w-full`} value={fields.path} onChange={(e) => onChange({ ...fields, path: e.target.value })} placeholder="/api/orders" />
      </label>
      <div>
        <p className="text-caption mb-1 text-slate-600 dark:text-slate-300">Headers (a cookie is a header — no separate field)</p>
        {fields.headers.length === 0 && (
          <p className="text-caption text-slate-400">No headers.</p>
        )}
        <div className="space-y-2">
          {fields.headers.map((h, i) => (
            <div key={i} className="flex gap-2">
              <input className={`${input} flex-1`} value={h.name} onChange={(e) => setHeader(i, { name: e.target.value })} placeholder="X-Auth" aria-label="header name" />
              <input className={`${input} flex-1`} value={h.value} onChange={(e) => setHeader(i, { value: e.target.value })} placeholder="token" aria-label="header value" />
              <Button variant="ghost" onClick={() => onChange({ ...fields, headers: fields.headers.filter((_, j) => j !== i) })}>
                ✕
              </Button>
            </div>
          ))}
        </div>
        <Button variant="ghost" className="mt-2" onClick={() => onChange({ ...fields, headers: [...fields.headers, { name: '', value: '' }] })}>
          + Add header
        </Button>
      </div>
      <div className="flex gap-2">
        <Button onClick={onApply}>Apply to YAML</Button>
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  );
}

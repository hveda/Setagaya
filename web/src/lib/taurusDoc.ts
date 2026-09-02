// The form lens (R5). Parses the fragment with yaml.parseDocument and
// edits fields IN the AST -- comments, key order, and uncompiled keys
// (think-time etc.) are untouched because the library only rewrites the
// nodes a field maps to. The edited doc's toString() is what the editor
// saves; byte-equality tests pin the contract.
import { parseDocument } from 'yaml';
import type { Document, YAMLMap } from 'yaml';

/**
 * The fragment shape this lens edits. A Taurus config's scenarios map is
 * keyed by scenario name and may hold several; the operator UI edits the
 * FIRST entry (the deploy path compiles exactly that one for portable
 * scenarios -- see lifecycleapp's fragment handling).
 */
export interface FragmentFields {
  defaultAddress: string;
  method: string;
  /** Path appended to default-address; empty = hit the base URL. */
  path: string;
  headers: Array<{ name: string; value: string }>;
}

/**
 * Locates the first scenario's map, or null when the fragment does not
 * have one (empty doc, or only an execution block).
 */
/** True when n is a YAML collection (has the items array). */
function isMap(n: unknown): n is YAMLMap {
  return (
    !!n &&
    typeof n === 'object' &&
    Array.isArray((n as { items?: unknown }).items)
  );
}

function firstScenarioMap(doc: Document): YAMLMap | null {
  const scenarios = doc.get('scenarios', true);
  if (!isMap(scenarios)) {
    return null;
  }
  const first = scenarios.items[0];
  const value = first?.value;
  return isMap(value) ? value : null;
}

/** Reads the editable fields out of the fragment. Missing keys read as empty. */
export function readFields(yamlText: string): FragmentFields {
  const doc = parseDocument(yamlText);
  const fields: FragmentFields = { defaultAddress: '', method: '', path: '', headers: [] };
  if (doc.errors.length > 0) {
    // Malformed YAML: the lens has nothing sane to read; the editor's
    // validation layer (R6) owns surfacing the parse error.
    return fields;
  }
  const scenario = firstScenarioMap(doc);
  if (!scenario) {
    return fields;
  }
  const addr = scenario.get('default-address', true);
  if (typeof addr?.value === 'string') {
    fields.defaultAddress = addr.value;
  }
  const req: unknown = scenario.get('requests', true);
  if (isMap(req)) {
    const firstReq: unknown = req.items[0];
    if (isMap(firstReq)) {
      const method = firstReq.get('method', true);
      if (typeof method?.value === 'string') {
        fields.method = method.value;
      }
      const url = firstReq.get('url', true);
      if (typeof url?.value === 'string') {
        // Split the stored URL against default-address so the form shows
        // the path part; a URL that does not start with the base is shown
        // whole in the path field.
        const base = fields.defaultAddress;
        fields.path = base && url.value.startsWith(base) ? url.value.slice(base.length) : url.value;
      }
    }
  }
  const headers = scenario.get('headers', true);
  if (isMap(headers)) {
    for (const item of headers.items) {
      const pair: unknown = item;
      const k = (pair as { key?: { value?: unknown } }).key?.value;
      const v = (pair as { value?: { value?: unknown } }).value?.value;
      if (typeof k === 'string') {
        fields.headers.push({ name: k, value: typeof v === 'string' ? v : '' });
      }
    }
  }
  return fields;
}

/**
 * Writes the form's fields back into the AST and returns the serialized
 * text. Only the mapped nodes change: comments, key order, and unknown
 * keys are preserved because parseDocument keeps them in the AST and
 * toString re-renders them as-is.
 */
/**
 * Detects the fragment's serialization style from the original text: the
 * block indent width and whether sequence dashes are indented under their
 * parent key. toString() re-indents to these options -- defaults (2 spaces,
 * sequences indented) would reformat a 4-space/indented-seq fragment and
 * break the byte-equality contract on untouched nodes.
 */
export function detectSeqIndent(yamlText: string): { indent: number; indented: boolean } {
  let indent = 2;
  let indented = true;
  let prevKeyIndent = -1;
  for (const line of yamlText.split('\n')) {
    if (!line.trim() || line.trimStart().startsWith('#')) {
      continue;
    }
    const pad = line.length - line.trimStart().length;
    const isSeq = line.trimStart().startsWith('- ');
    if (isSeq && prevKeyIndent >= 0) {
      indented = pad > prevKeyIndent;
      if (pad > prevKeyIndent) {
        indent = pad - prevKeyIndent;
      }
      break;
    }
    if (!isSeq) {
      prevKeyIndent = pad;
    }
  }
  // indent measured as the child step; fall back to scanning any indented
  // content line for the map indent when no sequence precedes it.
  if (indent === 2) {
    for (const line of yamlText.split('\n')) {
      if (!line.trim() || line.trimStart().startsWith('#') || line.trimStart().startsWith('- ')) {
        continue;
      }
      const pad = line.length - line.trimStart().length;
      if (pad > 0) {
        indent = pad;
        break;
      }
    }
  }
  return { indent, indented };
}

export function applyFields(yamlText: string, fields: FragmentFields): string {
  const doc = parseDocument(yamlText);
  const seq = detectSeqIndent(yamlText);
  if (doc.errors.length > 0) {
    // Refuse to write into a broken doc -- the caller keeps the text and
    // shows the parse error instead (editing a broken doc via the form
    // would silently drop whatever the parser could not model).
    throw new Error('cannot apply form fields to invalid YAML');
  }
  const scenario = firstScenarioMap(doc);
  if (!scenario) {
    throw new Error('fragment has no scenarios block to edit');
  }

  // setIn on missing parents is avoided deliberately: default-address and
  // headers are created at the top level of the scenario map via in-place
  // set, which preserves the map's existing key order and flow style.
  scenario.set('default-address', fields.defaultAddress);

  if (fields.headers.length === 0) {
    scenario.delete('headers');
  } else {
    const headers = new Map(fields.headers.map((h) => [h.name, h.value]));
    scenario.set('headers', headers);
  }

  // requests[0]: patch method and url in place; create the block only
  // when there is something to write (a fragment may be header/addr-only).
  const needsRequest = fields.method !== '' || fields.path !== '';
  if (needsRequest) {
    const existing: unknown = scenario.get('requests', true);
    if (!isMap(existing)) {
      const seqItems = (existing as { items?: unknown[] } | null | undefined)?.items;
      if (Array.isArray(seqItems) && seqItems.length === 0) {
        throw new Error('fragment has an empty requests list');
      }
      scenario.set('requests', [{ method: fields.method || 'GET', url: fields.path || '/' }]);
    } else {
      const req0: unknown = existing.items[0];
      if (isMap(req0)) {
        if (fields.method !== '') {
          req0.set('method', fields.method);
        }
        if (fields.path !== '') {
          // Re-join against the base: the stored url is the absolute form.
          const url = fields.defaultAddress && !fields.path.startsWith('http')
            ? `${fields.defaultAddress.replace(/\/$/, '')}${fields.path.startsWith('/') ? '' : '/'}${fields.path}`
            : fields.path;
          req0.set('url', url);
        }
      }
    }
  }

  // toString takes the serialization options: the DETECTED indent and
  // sequence-indent flag keep the output byte-identical to the original
  // formatting for untouched nodes (the default 2-space indent would
  // reformat a 4-space fragment).
  return doc.toString({ indent: seq.indent, indentSeq: seq.indented });
}

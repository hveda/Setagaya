import { afterEach, describe, expect, it } from 'vitest';
import { formatApmLink, loadApmTemplate, saveApmTemplate, APM_TEMPLATE_PLACEHOLDER } from './apm';

describe('formatApmLink', () => {
  // The link renders only when every ingredient is present: a template, a
  // correlation id, and the placeholder connecting them. Anything less
  // returns null so callers render no link at all (the raw id remains the
  // copy-paste affordance).
  it.each([
    ['https://apm.example.com/trace/{correlation_id}', 'abc123', 'https://apm.example.com/trace/abc123'],
    ['https://apm.example.com/trace/{correlation_id}?span={correlation_id}', 'abc', 'https://apm.example.com/trace/abc?span=abc'],
    ['  https://apm.example.com/{correlation_id}  ', 'abc', 'https://apm.example.com/abc'],
  ])('substitutes every occurrence of the placeholder (%s)', (template, id, expected) => {
    expect(formatApmLink(template, id)).toBe(expected);
  });

  it.each([
    ['no link: empty template', '', 'abc123'],
    ['no link: whitespace-only template', '   ', 'abc123'],
    ['no link: template missing the placeholder', 'https://apm.example.com/trace/', 'abc123'],
    ['no link: empty correlation id', 'https://apm.example.com/trace/{correlation_id}', ''],
  ])('%s', (_name, template, id) => {
    expect(formatApmLink(template, id)).toBeNull();
  });

  it('exposes the placeholder it substitutes', () => {
    expect(formatApmLink(`https://x/${APM_TEMPLATE_PLACEHOLDER}`, 'abc')).toBe('https://x/abc');
  });
});

describe('apm template storage', () => {
  afterEach(() => {
    localStorage.removeItem('honryu-apm-template');
  });

  it('loadApmTemplate returns "" when nothing is stored', () => {
    expect(loadApmTemplate()).toBe('');
  });

  it('saveApmTemplate round-trips, and an empty string clears back to ""', () => {
    saveApmTemplate('https://apm.example.com/trace/{correlation_id}');
    expect(loadApmTemplate()).toBe('https://apm.example.com/trace/{correlation_id}');

    saveApmTemplate('');
    expect(loadApmTemplate()).toBe('');
  });
});

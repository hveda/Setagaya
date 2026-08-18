// APM deep-link support for correlation ids (phase 10): the raw id is
// always the copy-paste affordance; a deployment-specific URL template,
// stored client-side in localStorage (honryu-apm-template), optionally
// turns it into a link. No server involvement -- same precedent as
// DashboardLayout's honryu-theme key.

/** The placeholder formatApmLink substitutes. */
export const APM_TEMPLATE_PLACEHOLDER = '{correlation_id}';

const STORAGE_KEY = 'honryu-apm-template';

/**
 * Builds an APM deep link from a template, or returns null when no link
 * should render: empty/whitespace template, template missing the
 * {correlation_id} placeholder, or empty correlation id. Every occurrence
 * of the placeholder is substituted.
 */
export function formatApmLink(template: string, correlationId: string): string | null {
  const trimmed = template.trim();
  if (!trimmed || !correlationId || !trimmed.includes(APM_TEMPLATE_PLACEHOLDER)) {
    return null;
  }
  return trimmed.replaceAll(APM_TEMPLATE_PLACEHOLDER, correlationId);
}

/** The saved template, or '' when none is stored. */
export function loadApmTemplate(): string {
  return localStorage.getItem(STORAGE_KEY) ?? '';
}

/** Stores the template as given (empty string clears it). */
export function saveApmTemplate(template: string): void {
  localStorage.setItem(STORAGE_KEY, template);
}

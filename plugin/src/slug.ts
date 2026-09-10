/**
 * Route/slug utilities.
 *
 * A route is the URL path segment under which a note is published.
 * It must be URL-safe: lowercase letters, digits, and single interior
 * hyphens; it must start and end with an alphanumeric character.
 */

export const ROUTE_MAX_LENGTH = 64;

const ROUTE_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/**
 * Convert an arbitrary note title (or filename) into a URL-safe slug.
 *
 * - Lowercases ASCII letters; leaves non-ASCII letters (accents, CJK, etc.)
 *   in place so they can be kept by callers that want them (they are stripped
 *   by `isValidRoute`, so callers should validate the final result).
 * - Collapses any run of whitespace or `_` into a single hyphen.
 * - Strips characters that are not letters, digits, or hyphens.
 * - Collapses repeated hyphens and trims leading/trailing hyphens.
 * - Truncates to ROUTE_MAX_LENGTH without leaving a trailing hyphen.
 *
 * May return an empty string (e.g. title was all punctuation); callers are
 * expected to validate with `isValidRoute` before use.
 */
export function slugify(title: string): string {
  const normalized = title
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[\s_]+/g, "-")
    // Drop anything that is not a letter, digit, or hyphen. \p{L}\p{N} keep
    // unicode letters/digits so non-ASCII titles still produce a base slug.
    .replace(/[^\p{L}\p{N}-]+/gu, "")
    .replace(/-{2,}/g, "-")
    .replace(/^-+|-+$/g, "");

  if (normalized.length <= ROUTE_MAX_LENGTH) {
    return normalized;
  }
  return normalized.slice(0, ROUTE_MAX_LENGTH).replace(/-+$/g, "");
}

/** Whether `route` is a valid, URL-safe publish route. */
export function isValidRoute(route: string): boolean {
  return route.length > 0 && route.length <= ROUTE_MAX_LENGTH && ROUTE_PATTERN.test(route);
}

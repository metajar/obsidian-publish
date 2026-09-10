/**
 * Shared password generator.
 *
 * Single source of truth for the CSPRNG generator used by the publish modal
 * and by the per-row "generate password" action in settings. Plaintext
 * passwords exist only transiently — never logged, never persisted.
 */

/** Generate a URL-unambiguous password using the platform CSPRNG. */
export function generatePassword(length = 20): string {
  // Alphabet excludes easily-confused characters (l, I, 1, O, 0).
  const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const values = new Uint32Array(length);
  crypto.getRandomValues(values);
  let out = "";
  for (let i = 0; i < values.length; i++) {
    out += alphabet[values[i] % alphabet.length];
  }
  return out;
}

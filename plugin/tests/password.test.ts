import { describe, expect, it } from "vitest";
import { generatePassword } from "../src/password";

// The generator alphabet deliberately excludes easily-confused characters:
// lowercase l, uppercase I and O, digits 0 and 1.
const ALPHABET = /^[a-km-zA-HJ-NP-Z2-9]+$/;

describe("generatePassword", () => {
  it("generates a 20-character password by default", () => {
    expect(generatePassword()).toHaveLength(20);
  });

  it("honours a custom length", () => {
    expect(generatePassword(32)).toHaveLength(32);
    expect(generatePassword(1)).toHaveLength(1);
  });

  it("uses only unambiguous characters (no l, I, O, 0, 1)", () => {
    for (let i = 0; i < 50; i++) {
      expect(generatePassword()).toMatch(ALPHABET);
    }
  });

  it("produces different passwords across calls (CSPRNG, not a fixed value)", () => {
    const seen = new Set<string>();
    for (let i = 0; i < 20; i++) {
      seen.add(generatePassword());
    }
    // 20 draws over a ~56^20 space: all-distinct is overwhelmingly expected.
    expect(seen.size).toBe(20);
  });
});

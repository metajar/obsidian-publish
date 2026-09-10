import { describe, expect, it } from "vitest";
import { isValidRoute, slugify } from "../src/slug";

describe("slugify", () => {
  it("lowercases and hyphenates a plain title", () => {
    expect(slugify("My Great Note")).toBe("my-great-note");
  });

  it("collapses repeated whitespace into a single hyphen", () => {
    expect(slugify("My   Great    Note")).toBe("my-great-note");
  });

  it("converts underscores to hyphens", () => {
    expect(slugify("meeting_notes_2026")).toBe("meeting-notes-2026");
  });

  it("strips punctuation", () => {
    expect(slugify("What's new? (v2.1!)")).toBe("whats-new-v21");
  });

  it("collapses consecutive hyphens and trims edges", () => {
    expect(slugify(" -- A -- B -- ")).toBe("a-b");
  });

  it("folds latin accents to base letters", () => {
    expect(slugify("Café Ménu")).toBe("cafe-menu");
  });

  it("returns an empty string for punctuation-only titles", () => {
    expect(slugify("?!...")).toBe("");
  });

  it("truncates to the max route length without a trailing hyphen", () => {
    const long = slugify("a".repeat(100) + " b");
    expect(long.length).toBeLessThanOrEqual(64);
    expect(long.endsWith("-")).toBe(false);
    expect(long).toBe("a".repeat(64));
  });
});

describe("isValidRoute", () => {
  it("accepts simple slugs", () => {
    expect(isValidRoute("my-note")).toBe(true);
    expect(isValidRoute("n1")).toBe(true);
    expect(isValidRoute("a")).toBe(true);
  });

  it("rejects empty and whitespace-only routes", () => {
    expect(isValidRoute("")).toBe(false);
  });

  it("rejects uppercase, spaces, and special characters", () => {
    expect(isValidRoute("My Note")).toBe(false);
    expect(isValidRoute("my_note")).toBe(false);
    expect(isValidRoute("my.note")).toBe(false);
    expect(isValidRoute("my/note")).toBe(false);
    expect(isValidRoute("my note")).toBe(false);
  });

  it("rejects leading, trailing, or doubled hyphens", () => {
    expect(isValidRoute("-note")).toBe(false);
    expect(isValidRoute("note-")).toBe(false);
    expect(isValidRoute("my--note")).toBe(false);
  });

  it("rejects routes longer than 64 characters", () => {
    expect(isValidRoute("a".repeat(64))).toBe(true);
    expect(isValidRoute("a".repeat(65))).toBe(false);
  });

  it("accepts output of slugify for normal titles", () => {
    const slug = slugify("A Perfectly Normal Title 42");
    expect(isValidRoute(slug)).toBe(true);
  });
});

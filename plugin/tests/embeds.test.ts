import { describe, expect, it } from "vitest";
import { isImageFilename, rewriteImageEmbeds, scanEmbeds } from "../src/embeds";

describe("isImageFilename", () => {
  it("accepts the server-supported image extensions, case-insensitively", () => {
    for (const name of ["a.png", "b.JPG", "c.Jpeg", "d.gif", "e.webp", "f.PNG"]) {
      expect(isImageFilename(name)).toBe(true);
    }
  });

  it("rejects non-images, extensionless names, and svg", () => {
    for (const name of ["a.pdf", "b.md", "c.mp3", "d.svg", "noext", ".png/evil"]) {
      expect(isImageFilename(name)).toBe(false);
    }
  });
});

describe("scanEmbeds — wiki syntax", () => {
  it("finds a bare wiki embed by basename", () => {
    const scan = scanEmbeds("Look at ![[shot.png]] here.");
    expect(scan.images).toHaveLength(1);
    expect(scan.images[0]).toMatchObject({ kind: "wiki", target: "shot.png", alt: "shot" });
    expect(scan.nonImages).toEqual([]);
  });

  it("keeps the full path as the target and derives alt from the basename", () => {
    const scan = scanEmbeds("![[attachments/trip/boat.jpg]]");
    expect(scan.images[0]).toMatchObject({ kind: "wiki", target: "attachments/trip/boat.jpg", alt: "boat" });
  });

  it("parses a size suffix but does not include it in the target", () => {
    const scan = scanEmbeds("![[shot.png|400]] and ![[wide.png|800x600]]");
    expect(scan.images.map((i) => i.target)).toEqual(["shot.png", "wide.png"]);
  });

  it("records non-image attachments separately", () => {
    const scan = scanEmbeds("![[report.pdf]] ![[other note.md]]");
    expect(scan.images).toEqual([]);
    expect(scan.nonImages).toEqual(["report.pdf", "other note.md"]);
  });

  it("ignores wiki links that are not embeds", () => {
    const scan = scanEmbeds("[[shot.png]] is a link, not an embed.");
    expect(scan.images).toEqual([]);
    expect(scan.nonImages).toEqual([]);
  });
});

describe("scanEmbeds — standard ref syntax", () => {
  it("finds local relative refs with their alt text", () => {
    const scan = scanEmbeds("![the boat](attachments/boat.jpg)");
    expect(scan.images).toHaveLength(1);
    expect(scan.images[0]).toMatchObject({ kind: "ref", target: "attachments/boat.jpg", alt: "the boat" });
  });

  it("leaves remote http(s) refs untouched — not images, not non-images", () => {
    const scan = scanEmbeds("![logo](https://example.com/logo.png) and ![x](http://x.test/a.gif)");
    expect(scan.images).toEqual([]);
    expect(scan.nonImages).toEqual([]);
  });

  it("records non-image local refs as attachments", () => {
    const scan = scanEmbeds("![spec](docs/spec.pdf)");
    expect(scan.images).toEqual([]);
    expect(scan.nonImages).toEqual(["spec.pdf"]);
  });

  it("does not mistake a plain link for an embed", () => {
    const scan = scanEmbeds("see [the docs](docs/spec.pdf) for details");
    expect(scan.images).toEqual([]);
    expect(scan.nonImages).toEqual([]);
  });
});

describe("scanEmbeds — ordering and code blocks", () => {
  it("returns embeds in document order across both syntaxes", () => {
    const scan = scanEmbeds("![a](a.png)\n\n![[b.png]]\n\n![c](c.png)");
    expect(scan.images.map((i) => i.target)).toEqual(["a.png", "b.png", "c.png"]);
  });

  it("ignores embeds inside fenced code blocks and inline code", () => {
    const md = [
      "Text with `![[inline.png]]` inline code.",
      "",
      "```markdown",
      "![[fenced.png]]",
      "![fenced](fenced.jpg)",
      "```",
      "",
      "Real one: ![[real.png]]",
    ].join("\n");
    const scan = scanEmbeds(md);
    expect(scan.images.map((i) => i.target)).toEqual(["real.png"]);
    expect(scan.nonImages).toEqual([]);
  });
});

describe("rewriteImageEmbeds", () => {
  const url = "https://notes.example.com/assets/abc123.png";

  it("rewrites a wiki embed (size suffix dropped) to a standard image ref", () => {
    const scan = scanEmbeds("Before ![[shot.png|400]] after");
    const out = rewriteImageEmbeds("Before ![[shot.png|400]] after", scan.images, () => url);
    expect(out).toBe(`Before ![shot](${url}) after`);
  });

  it("rewrites a local ref, keeping the original alt text", () => {
    const scan = scanEmbeds("![the boat](attachments/boat.jpg)");
    const out = rewriteImageEmbeds("![the boat](attachments/boat.jpg)", scan.images, () => url);
    expect(out).toBe(`![the boat](${url})`);
  });

  it("leaves embeds untouched when urlFor returns null (upload failed / unresolved)", () => {
    const md = "![kept](kept.png) ![[dropped.png]]";
    const scan = scanEmbeds(md);
    const out = rewriteImageEmbeds(md, scan.images, (embed) =>
      embed.kind === "ref" ? url : null,
    );
    expect(out).toBe(`![kept](${url}) ![[dropped.png]]`);
  });

  it("rewrites multiple embeds without corrupting the surrounding text", () => {
    const md = "# Title\n\n![[one.png]] mid ![two](two.png) `code ![[not-touched.png]]` end ![[three.png|50]]";
    const scan = scanEmbeds(md);
    const out = rewriteImageEmbeds(md, scan.images, () => "https://s.test/x.png");
    expect(out).toBe(
      "# Title\n\n![one](https://s.test/x.png) mid ![two](https://s.test/x.png) `code ![[not-touched.png]]` end ![three](https://s.test/x.png)",
    );
  });

  it("strips brackets from alt text so the rewrite cannot break the markdown", () => {
    const scan = scanEmbeds("![[weird.png]]");
    const embed = { ...scan.images[0], alt: "a[b]" };
    const out = rewriteImageEmbeds("![[weird.png]]", [embed], () => url);
    expect(out).toBe(`![ab](${url})`);
  });
});

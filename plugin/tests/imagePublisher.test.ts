import { beforeEach, describe, expect, it } from "vitest";
import { PublishApiClient } from "../src/api";
import { publishNoteImages, type VaultLike } from "../src/imagePublisher";
import { __setRequestUrlHandler } from "./mocks/obsidian";

const BASE = "https://notes.example.com";
const TOKEN = "test-token-abc123";

interface FakeFile {
  path: string;
  name: string;
  bytes: Uint8Array;
}

function fakeVault(files: FakeFile[]): VaultLike & { reads: string[] } {
  const reads: string[] = [];
  return {
    reads,
    getFiles: () => files.map((f) => ({ path: f.path, name: f.name })),
    readBinary: async (file: { path: string }) => {
      const found = files.find((f) => f.path === file.path);
      if (!found) throw new Error("not found");
      reads.push(found.path);
      return found.bytes.slice().buffer;
    },
  };
}

function pngBytes(): Uint8Array {
  return new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3]);
}

let assetRequests: Array<{ filename: string | null; bodyText: string }> = [];
/** Uploads succeed and return a stable URL derived from the filename. */
function uploadsSucceed(): void {
  assetRequests = [];
  __setRequestUrlHandler(async (req) => {
    if (!req.url.endsWith("/api/assets")) {
      throw new Error(`unexpected request in test: ${req.method} ${req.url}`);
    }
    const text = typeof req.body === "string" ? req.body : new TextDecoder("latin1").decode(req.body ?? new ArrayBuffer(0));
    const match = text.match(/filename="([^"]*)"/);
    assetRequests.push({ filename: match?.[1] ?? null, bodyText: text });
    return {
      status: 201,
      json: { url: `/assets/${match?.[1]}`, filename: match?.[1] ?? "" },
    };
  });
}

/** Uploads fail with the given HTTP status (resolved, not thrown). */
function uploadsFail(status: number): void {
  __setRequestUrlHandler(async (req) => {
    if (!req.url.endsWith("/api/assets")) {
      throw new Error(`unexpected request in test: ${req.method} ${req.url}`);
    }
    return { status, json: { error: "nope" } };
  });
}

function run(markdown: string, vault: VaultLike) {
  return publishNoteImages({
    markdown,
    vault,
    client: new PublishApiClient(BASE, TOKEN),
    baseUrl: `${BASE}/`,
  });
}

beforeEach(() => {
  uploadsSucceed();
});

describe("publishNoteImages — wiki embeds", () => {
  it("uploads a vault image by basename and rewrites the embed to the server URL", async () => {
    const vault = fakeVault([{ path: "attachments/shot.png", name: "shot.png", bytes: pngBytes() }]);
    const out = await run("![[shot.png|400]]", vault);
    expect(out.uploaded).toEqual(["shot.png"]);
    expect(out.failed).toEqual([]);
    expect(out.markdown).toBe(`![shot](${BASE}/assets/shot.png)`);
    expect(out.markdown).not.toContain("|400");
  });

  it("resolves a wiki target with a path to the exact vault file", async () => {
    const vault = fakeVault([
      { path: "a/dupe.png", name: "dupe.png", bytes: pngBytes() },
      { path: "b/dupe.png", name: "dupe.png", bytes: pngBytes() },
    ]);
    const out = await run("![[b/dupe.png]]", vault);
    expect(out.uploaded).toEqual(["dupe.png"]);
    expect(out.markdown).toBe(`![dupe](${BASE}/assets/dupe.png)`);
  });

  it("leaves an unresolvable image embed unrewritten and reports nothing failed", async () => {
    const vault = fakeVault([]);
    const out = await run("![[missing.png]]", vault);
    expect(out.uploaded).toEqual([]);
    expect(out.failed).toEqual([]);
    expect(out.markdown).toBe("![[missing.png]]");
  });
});

describe("publishNoteImages — ref embeds", () => {
  it("treats a local ref as vault-relative and keeps the alt text", async () => {
    const vault = fakeVault([{ path: "pics/boat.jpg", name: "boat.jpg", bytes: pngBytes() }]);
    const out = await run("![the boat](pics/boat.jpg)", vault);
    expect(out.uploaded).toEqual(["boat.jpg"]);
    expect(out.markdown).toBe(`![the boat](${BASE}/assets/boat.jpg)`);
  });

  it("decodes percent-encoded ref paths", async () => {
    const vault = fakeVault([{ path: "pics/my boat.png", name: "my boat.png", bytes: pngBytes() }]);
    const out = await run("![](pics/my%20boat.png)", vault);
    expect(out.uploaded).toEqual(["my boat.png"]);
  });

  it("never touches remote refs", async () => {
    const vault = fakeVault([{ path: "logo.png", name: "logo.png", bytes: pngBytes() }]);
    const md = "![remote](https://cdn.example.com/logo.png)";
    const out = await run(md, vault);
    expect(out.uploaded).toEqual([]);
    expect(out.markdown).toBe(md);
  });
});

describe("publishNoteImages — dedupe and failure policy", () => {
  it("uploads the same file once when embedded twice via different syntaxes", async () => {
    const vault = fakeVault([{ path: "img.png", name: "img.png", bytes: pngBytes() }]);
    const out = await run("![[img.png]] and ![again](./img.png)", vault);
    expect(assetRequests).toHaveLength(1);
    expect(out.uploaded).toEqual(["img.png"]);
    expect(out.markdown).toBe(
      `![img](${BASE}/assets/img.png) and ![again](${BASE}/assets/img.png)`,
    );
  });

  it("fails soft on upload errors: publish markdown keeps the original embed", async () => {
    uploadsFail(500);
    const vault = fakeVault([{ path: "shot.png", name: "shot.png", bytes: pngBytes() }]);
    const out = await run("![[shot.png]]", vault);
    expect(out.uploaded).toEqual([]);
    expect(out.failed).toEqual(["shot.png"]);
    expect(out.markdown).toBe("![[shot.png]]");
  });

  it("fails soft per file: a failing upload does not block the others", async () => {
    let calls = 0;
    __setRequestUrlHandler(async (req) => {
      const text = typeof req.body === "string" ? req.body : new TextDecoder("latin1").decode(req.body ?? new ArrayBuffer(0));
      const name = text.match(/filename="([^"]*)"/)?.[1] ?? "";
      calls += 1;
      if (name === "bad.png") return { status: 413, json: { error: "too large" } };
      return { status: 201, json: { url: `/assets/${name}`, filename: name } };
    });
    const vault = fakeVault([
      { path: "good.png", name: "good.png", bytes: pngBytes() },
      { path: "bad.png", name: "bad.png", bytes: pngBytes() },
    ]);
    const out = await run("![[good.png]] ![[bad.png]]", vault);
    expect(calls).toBe(2);
    expect(out.uploaded).toEqual(["good.png"]);
    expect(out.failed).toEqual(["bad.png"]);
    expect(out.markdown).toBe(`![good](${BASE}/assets/good.png) ![[bad.png]]`);
  });

  it("uses an absolute URL from the server as-is", async () => {
    __setRequestUrlHandler(async (req) => {
      const text = typeof req.body === "string" ? req.body : new TextDecoder("latin1").decode(req.body ?? new ArrayBuffer(0));
      const name = text.match(/filename="([^"]*)"/)?.[1] ?? "";
      return { status: 201, json: { url: `https://cdn.other.test/f/${name}`, filename: name } };
    });
    const vault = fakeVault([{ path: "a.png", name: "a.png", bytes: pngBytes() }]);
    const out = await run("![[a.png]]", vault);
    expect(out.markdown).toBe("![a](https://cdn.other.test/f/a.png)");
  });
});

describe("publishNoteImages — non-image attachments", () => {
  it("counts non-image attachments and skips the rewrite", async () => {
    const vault = fakeVault([
      { path: "report.pdf", name: "report.pdf", bytes: new Uint8Array([1]) },
      { path: "shot.png", name: "shot.png", bytes: pngBytes() },
    ]);
    const out = await run("![[report.pdf]] ![[shot.png]]", vault);
    expect(out.nonImageCount).toBe(1);
    expect(out.uploaded).toEqual(["shot.png"]);
    expect(out.markdown).toBe(`![[report.pdf]] ![shot](${BASE}/assets/shot.png)`);
  });

  it("returns the markdown unchanged when there is nothing to publish", async () => {
    const vault = fakeVault([]);
    const md = "# Just text\n\nNo embeds here.";
    const out = await run(md, vault);
    expect(out).toEqual({ markdown: md, uploaded: [], failed: [], nonImageCount: 0 });
    expect(vault.reads).toEqual([]);
  });
});

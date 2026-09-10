import { beforeEach, describe, expect, it } from "vitest";
import { describeApiError, PublishApiClient } from "../src/api";
import { __setRequestUrlHandler, type MockRequest } from "./mocks/obsidian";

const BASE = "https://notes.example.com";
const TOKEN = "test-token-abc123";

function client(): PublishApiClient {
  return new PublishApiClient(BASE, TOKEN);
}

/** Install a handler that records the last request and replies per-test. */
let lastRequest: MockRequest | null = null;

function reply(status: number, json?: unknown, text?: string): void {
  __setRequestUrlHandler(async (req) => {
    lastRequest = req;
    return { status, json, text };
  });
}

function networkError(message = "net::ERR_CONNECTION_REFUSED"): void {
  __setRequestUrlHandler(async () => {
    const err = new Error(message);
    throw err;
  });
}

function httpError(status: number, message?: string): void {
  // requestUrl rejects with an Error carrying `status` for HTTP failures
  // on some Obsidian builds; the client must map these too.
  __setRequestUrlHandler(async () => {
    const err = new Error(message ?? `HTTP ${status}`);
    (err as Error & { status: number }).status = status;
    throw err;
  });
}

beforeEach(() => {
  lastRequest = null;
});

/** Parse the last JSON request body (asset uploads send ArrayBuffer instead). */
function jsonBody(): Record<string, unknown> {
  const body = lastRequest?.body;
  if (typeof body !== "string") throw new Error("expected a JSON (string) request body");
  return JSON.parse(body);
}

describe("PublishApiClient — auth and URL construction", () => {
  it("sends the bearer token on every request and normalizes the base URL", async () => {
    reply(200, { available: true });
    const result = await client().checkRouteAvailable("my-note");
    expect(result.ok).toBe(true);
    expect(lastRequest?.url).toBe(`${BASE}/api/routes/my-note/available`);
    expect(lastRequest?.method).toBe("GET");
    expect(lastRequest?.headers?.["Authorization"]).toBe(`Bearer ${TOKEN}`);
  });

  it("strips trailing slashes from the configured base URL", async () => {
    reply(200, []);
    const result = await new PublishApiClient(`${BASE}/`, TOKEN).listPages();
    expect(result.ok).toBe(true);
    expect(lastRequest?.url).toBe(`${BASE}/api/pages`);
  });

  it("fails fast with bad-config when URL or token is missing", async () => {
    const noUrl = await new PublishApiClient("", TOKEN).listPages();
    expect(noUrl).toEqual({
      ok: false,
      error: { kind: "bad-config", message: expect.any(String) },
    });
    const noToken = await new PublishApiClient(BASE, " ").listPages();
    expect(noToken.ok).toBe(false);
    if (!noToken.ok) expect(noToken.error.kind).toBe("bad-config");
  });
});

describe("PublishApiClient — checkRouteAvailable", () => {
  it("returns true when the server says available", async () => {
    reply(200, { available: true });
    const result = await client().checkRouteAvailable("free-route");
    expect(result).toEqual({ ok: true, data: true });
  });

  it("returns false when the server says taken", async () => {
    reply(200, { available: false });
    const result = await client().checkRouteAvailable("taken-route");
    expect(result).toEqual({ ok: true, data: false });
  });

  it("rejects a malformed availability payload", async () => {
    reply(200, { nope: true });
    const result = await client().checkRouteAvailable("x");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unknown");
  });

  it("maps 401 to unauthorized", async () => {
    httpError(401);
    const result = await client().checkRouteAvailable("x");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unauthorized");
  });
});

describe("PublishApiClient — createPage", () => {
  it("POSTs route/title/markdown and omits password when not protecting", async () => {
    reply(201, {
      route: "my-note",
      title: "My Note",
      created_at: "2026-09-10T00:00:00Z",
      updated_at: "2026-09-10T00:00:00Z",
      password_protected: false,
    });
    const result = await client().createPage("my-note", {
      title: "My Note",
      markdown: "# Hello",
    });
    expect(result.ok).toBe(true);
    expect(lastRequest?.method).toBe("POST");
    expect(lastRequest?.url).toBe(`${BASE}/api/pages`);
    const body = jsonBody();
    expect(body).toEqual({ route: "my-note", title: "My Note", markdown: "# Hello" });
    expect("password" in body).toBe(false);
  });

  it("includes the password in the payload when one is set", async () => {
    reply(201, { route: "secret", title: "t", created_at: "", updated_at: "", password_protected: true });
    const result = await client().createPage("secret", {
      title: "t",
      markdown: "m",
      password: "hunter2",
    });
    expect(result.ok).toBe(true);
    const body = jsonBody();
    expect(body.password).toBe("hunter2");
  });

  it("maps 409 to route-taken", async () => {
    httpError(409, "route exists");
    const result = await client().createPage("taken", { title: "t", markdown: "m" });
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error.kind).toBe("route-taken");
      if (result.error.kind === "route-taken") expect(result.error.message).toBe("route exists");
    }
  });

  it("maps 400 to invalid-route", async () => {
    reply(400, { error: "bad slug" });
    const result = await client().createPage("Bad Slug", { title: "t", markdown: "m" });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("invalid-route");
  });
});

describe("PublishApiClient — updatePage", () => {
  it("omits password entirely when it should be kept unchanged", async () => {
    reply(200, { route: "my-note", title: "t", created_at: "", updated_at: "", password_protected: true });
    const result = await client().updatePage("my-note", { title: "t", markdown: "m" });
    expect(result.ok).toBe(true);
    expect(lastRequest?.method).toBe("PUT");
    expect(lastRequest?.url).toBe(`${BASE}/api/pages/my-note`);
    const body = jsonBody();
    expect("password" in body).toBe(false);
  });

  it("sends explicit null to remove password protection", async () => {
    reply(200, { route: "my-note", title: "t", created_at: "", updated_at: "", password_protected: false });
    const result = await client().updatePage("my-note", {
      title: "t",
      markdown: "m",
      password: null,
    });
    expect(result.ok).toBe(true);
    const body = jsonBody();
    expect(body.password).toBeNull();
  });

  it("sends a string password to set or replace one", async () => {
    reply(200, { route: "my-note", title: "t", created_at: "", updated_at: "", password_protected: true });
    await client().updatePage("my-note", { title: "t", markdown: "m", password: "new-pass" });
    const body = jsonBody();
    expect(body.password).toBe("new-pass");
  });

  it("accepts a record nested under a `page` key", async () => {
    reply(200, {
      page: { route: "my-note", title: "t", created_at: "", updated_at: "", password_protected: false },
    });
    const result = await client().updatePage("my-note", { title: "t", markdown: "m" });
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.data.route).toBe("my-note");
  });

  it("maps 404 to not-found", async () => {
    httpError(404);
    const result = await client().updatePage("ghost", { title: "t", markdown: "m" });
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("not-found");
  });
});

describe("PublishApiClient — listPages", () => {
  it("parses a bare JSON array response", async () => {
    reply(200, [
      { route: "a", title: "A", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-02T00:00:00Z", password_protected: false },
    ]);
    const result = await client().listPages();
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data).toHaveLength(1);
      expect(result.data[0].route).toBe("a");
      expect(result.data[0].password_protected).toBe(false);
    }
  });

  it("parses an array wrapped under a `pages` key", async () => {
    reply(200, {
      pages: [
        { route: "b", title: "B", created_at: "", updated_at: "", password_protected: true },
      ],
    });
    const result = await client().listPages();
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.data[0].route).toBe("b");
  });

  it("maps 500 to server-error", async () => {
    reply(500, { error: "boom" });
    const result = await client().listPages();
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("server-error");
  });
});

describe("PublishApiClient — deletePage", () => {
  it("treats 204 as success", async () => {
    reply(204, undefined, "");
    const result = await client().deletePage("gone");
    expect(lastRequest?.method).toBe("DELETE");
    expect(lastRequest?.url).toBe(`${BASE}/api/pages/gone`);
    expect(result).toEqual({ ok: true, data: null });
  });

  it("maps 404 to not-found", async () => {
    reply(404, { error: "no such route" });
    const result = await client().deletePage("ghost");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("not-found");
  });
});

describe("PublishApiClient — transport failures", () => {
  it("maps connection failures to unreachable", async () => {
    networkError();
    const result = await client().listPages();
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unreachable");
  });

  it("maps thrown HTTP statuses (older requestUrl behavior) the same as resolved ones", async () => {
    httpError(401);
    const result = await client().listPages();
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unauthorized");
  });
});

describe("PublishApiClient — getTheme", () => {
  it("GETs /api/theme and parses the record", async () => {
    reply(200, { css: "body { color: red; }", name: "warm", updated_at: "2026-09-10T00:00:00Z" });
    const result = await client().getTheme();
    expect(lastRequest?.method).toBe("GET");
    expect(lastRequest?.url).toBe(`${BASE}/api/theme`);
    expect(result).toEqual({
      ok: true,
      data: { css: "body { color: red; }", name: "warm", updated_at: "2026-09-10T00:00:00Z" },
    });
  });

  it("returns the empty default theme as-is when no theme is set", async () => {
    reply(200, { css: "", name: "default", updated_at: "" });
    const result = await client().getTheme();
    expect(result).toEqual({ ok: true, data: { css: "", name: "default", updated_at: "" } });
  });

  it("maps 401 to unauthorized", async () => {
    httpError(401);
    const result = await client().getTheme();
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unauthorized");
  });
});

describe("PublishApiClient — setTheme", () => {
  it("POSTs {css, name} to /api/theme", async () => {
    reply(200, { css: "a{}", name: "custom", updated_at: "2026-09-10T00:00:00Z" });
    const result = await client().setTheme("a{}", "warm");
    expect(result.ok).toBe(true);
    expect(lastRequest?.method).toBe("POST");
    expect(lastRequest?.url).toBe(`${BASE}/api/theme`);
    expect(lastRequest?.headers?.["Content-Type"]).toBe("application/json");
    expect(jsonBody()).toEqual({ css: "a{}", name: "warm" });
  });

  it("omits name when not supplied", async () => {
    reply(200, { css: "a{}", name: "custom", updated_at: "" });
    await client().setTheme("a{}");
    const body = jsonBody();
    expect(body).toEqual({ css: "a{}" });
    expect("name" in body).toBe(false);
  });

  it("echoes the input when the server replies 2xx without a body", async () => {
    reply(204, undefined, "");
    const result = await client().setTheme("a{}", "warm");
    expect(result).toEqual({ ok: true, data: { css: "a{}", name: "warm", updated_at: "" } });
  });

  it("rejects a malformed theme body", async () => {
    reply(200, { surprise: true });
    const result = await client().setTheme("a{}");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unknown");
  });

  it("maps 401 to unauthorized (invalid token feedback for the theme section)", async () => {
    httpError(401);
    const result = await client().setTheme("a{}");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unauthorized");
  });
});

describe("PublishApiClient — uploadAsset", () => {
  const PNG_BYTES = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 1, 2, 3]);
  const input = { filename: "shot.png", mime: "image/png", data: PNG_BYTES.slice().buffer };

  function bodyText(): string {
    expect(lastRequest?.body).toBeInstanceOf(ArrayBuffer);
    return new TextDecoder("latin1").decode(lastRequest?.body as ArrayBuffer);
  }

  it("sends a multipart body with a `file` field and the image bytes", async () => {
    reply(201, { url: "/assets/abc.png", filename: "abc.png" });
    const result = await client().uploadAsset(input);
    expect(result).toEqual({ ok: true, data: { url: "/assets/abc.png", filename: "abc.png" } });
    expect(lastRequest?.method).toBe("POST");
    expect(lastRequest?.url).toBe(`${BASE}/api/assets`);
    expect(lastRequest?.headers?.["Authorization"]).toBe(`Bearer ${TOKEN}`);

    const contentType = lastRequest?.headers?.["Content-Type"] ?? "";
    expect(contentType).toMatch(/^multipart\/form-data; boundary=/);
    const boundary = contentType.split("boundary=")[1];

    const body = bodyText();
    expect(body.startsWith(`--${boundary}\r\n`)).toBe(true);
    expect(body).toContain('Content-Disposition: form-data; name="file"; filename="shot.png"');
    expect(body).toContain("Content-Type: image/png");
    // Binary payload arrives byte-exact between the blank line and the tail.
    const payloadStart = body.indexOf("\r\n\r\n") + 4;
    const payloadEnd = body.indexOf(`\r\n--${boundary}--`);
    expect(body.slice(payloadStart, payloadEnd)).toBe(new TextDecoder("latin1").decode(PNG_BYTES));
  });

  it("treats a 200 dedupe re-upload as success with the same body", async () => {
    reply(200, { url: "/assets/existing.png", filename: "existing.png" });
    const result = await client().uploadAsset(input);
    expect(result).toEqual({ ok: true, data: { url: "/assets/existing.png", filename: "existing.png" } });
  });

  it("maps 400 (non-image/svg rejection) to asset-rejected", async () => {
    reply(400, { error: "svg not allowed" });
    const result = await client().uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error.kind).toBe("asset-rejected");
      if (result.error.kind === "asset-rejected") expect(result.error.message).toBe("svg not allowed");
    }
  });

  it("maps 413 (oversize) to asset-too-large", async () => {
    httpError(413, "file too large"); // thrown-with-status variant must map too
    const result = await client().uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("asset-too-large");
  });

  it("maps 413 the same way when resolved rather than thrown", async () => {
    reply(413, { error: "too large" });
    const result = await client().uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("asset-too-large");
  });

  it("maps connection failures to unreachable", async () => {
    networkError();
    const result = await client().uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unreachable");
  });

  it("rejects a malformed asset response", async () => {
    reply(201, { nope: true });
    const result = await client().uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("unknown");
  });

  it("fails fast with bad-config when the token is missing", async () => {
    const result = await new PublishApiClient(BASE, " ").uploadAsset(input);
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.kind).toBe("bad-config");
  });
});

describe("describeApiError — Notices for the theme section", () => {
  it("labels messages with the failing action's context", () => {
    expect(describeApiError({ kind: "unauthorized", status: 401 }, "Theme")).toBe(
      "Theme: invalid API token — check it in Publish plugin settings.",
    );
    expect(describeApiError({ kind: "unreachable", message: "refused" }, "Theme")).toContain(
      "Theme: could not reach the server",
    );
    expect(describeApiError({ kind: "unauthorized", status: 401 })).toContain("Publish: invalid API token");
  });

  it("describes the asset-specific error kinds", () => {
    expect(describeApiError({ kind: "asset-rejected", status: 400 })).toContain("images only");
    expect(describeApiError({ kind: "asset-too-large", status: 413 })).toContain("max 10 MB");
  });
});

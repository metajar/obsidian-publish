import { beforeEach, describe, expect, it } from "vitest";
import { PublishApiClient } from "../src/api";
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
    const body = JSON.parse(lastRequest?.body ?? "{}");
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
    const body = JSON.parse(lastRequest?.body ?? "{}");
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
    const body = JSON.parse(lastRequest?.body ?? "{}");
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
    const body = JSON.parse(lastRequest?.body ?? "{}");
    expect(body.password).toBeNull();
  });

  it("sends a string password to set or replace one", async () => {
    reply(200, { route: "my-note", title: "t", created_at: "", updated_at: "", password_protected: true });
    await client().updatePage("my-note", { title: "t", markdown: "m", password: "new-pass" });
    const body = JSON.parse(lastRequest?.body ?? "{}");
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

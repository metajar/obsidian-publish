/**
 * Regression tests for the stale note↔route mapping bug: a note unpublished
 * elsewhere (settings, another device) left a local map entry behind, so the
 * publish modal opened in update mode and the PUT 404'd.
 *
 * openPublishModal must verify the local entry against the server and drop it
 * when the route is free again. The unpublishNote tests below cover the
 * command/context-menu path: it must confirm before deleting, clean up stale
 * mappings, and keep mappings when the delete outcome is unknown.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  TFile,
  __resetPluginData,
  __setRequestUrlHandler,
  type MockRequest,
} from "./mocks/obsidian";

type CapturedModal = { existing: unknown; opened: boolean };

const captured: CapturedModal[] = [];

vi.mock("../src/publishModal", () => ({
  PublishModal: class {
    existing: unknown;
    opened = false;
    constructor(
      _app: unknown,
      _plugin: unknown,
      _file: unknown,
      _client: unknown,
      existing: unknown,
    ) {
      this.existing = existing;
      captured.push(this satisfies CapturedModal);
    }
    open(): void {
      this.opened = true;
    }
  },
}));

type CapturedConfirm = { title: string; onConfirm: () => Promise<void> };

const confirms: CapturedConfirm[] = [];

vi.mock("../src/confirmModal", () => ({
  ConfirmModal: class {
    constructor(_app: unknown, options: { title: string; onConfirm: () => Promise<void> }) {
      confirms.push({ title: options.title, onConfirm: options.onConfirm });
    }
    open(): void {}
  },
}));

import SelfHostedPublishPlugin from "../src/main";

const FILE = new TFile("goals/sara.md", "md", "sara") as unknown as Parameters<
  InstanceType<typeof SelfHostedPublishPlugin>["openPublishModal"]
>[0];

async function newPlugin(): Promise<InstanceType<typeof SelfHostedPublishPlugin>> {
  const plugin = new SelfHostedPublishPlugin({} as never, {} as never);
  await plugin.loadSettings();
  plugin.settings.serverUrl = "http://srv";
  plugin.settings.apiToken = "tok";
  return plugin;
}

/** Availability responses keyed by route. */
function availabilityByRoute(available: Record<string, boolean>): void {
  __setRequestUrlHandler(async (req) => {
    const match = req.url.match(/\/api\/routes\/([a-z0-9-]+)\/available$/);
    if (match && req.method === "GET") {
      return { status: 200, json: { available: available[match[1]] ?? false } };
    }
    return { status: 404, json: { error: "unexpected " + req.method + " " + req.url } };
  });
}

beforeEach(() => {
  captured.length = 0;
  confirms.length = 0;
  __resetPluginData();
});

describe("openPublishModal stale-mapping recovery", () => {
  it("drops the local entry and opens in create mode when the server says the route is free", async () => {
    availabilityByRoute({ "sara-goals": true });
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: true });

    await plugin.openPublishModal(FILE);

    const modal = captured.at(-1);
    expect(modal).toBeDefined();
    expect(modal!.existing).toBeNull();
    expect(modal!.opened).toBe(true);
    // The stale mapping was forgotten, not just bypassed.
    expect(plugin.getLocalRouteEntry(FILE.path)).toBeNull();
  });

  it("keeps the entry when the server still has the page", async () => {
    availabilityByRoute({ "sara-goals": false });
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: true });

    await plugin.openPublishModal(FILE);

    const modal = captured.at(-1);
    expect(modal!.existing).toEqual({ route: "sara-goals", passwordProtected: true });
    expect(plugin.getLocalRouteEntry(FILE.path)).toEqual({ route: "sara-goals", passwordProtected: true });
  });

  it("keeps the entry when availability cannot be checked (server unreachable)", async () => {
    __setRequestUrlHandler(async () => ({ status: 0, json: {} }));
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: false });

    await plugin.openPublishModal(FILE);

    const modal = captured.at(-1);
    expect(modal!.existing).toEqual({ route: "sara-goals", passwordProtected: false });
  });

  it("opens in create mode directly when no local entry exists", async () => {
    const plugin = await newPlugin();
    await plugin.openPublishModal(FILE);
    expect(captured.at(-1)!.existing).toBeNull();
  });
});

describe("unpublishNote", () => {
  const requests: MockRequest[] = [];

  /**
   * Handler: availability for `sara-goals` per test, DELETE /api/pages
   * returning `deleteStatus`, everything else 404. All requests recorded.
   */
  function stubServer(available: boolean, deleteStatus: number): void {
    __setRequestUrlHandler(async (req) => {
      requests.push(req);
      if (req.url.endsWith("/api/routes/sara-goals/available") && req.method === "GET") {
        return { status: 200, json: { available } };
      }
      if (req.url.endsWith("/api/pages/sara-goals") && req.method === "DELETE") {
        return { status: deleteStatus, json: {} };
      }
      return { status: 404, json: { error: "unexpected " + req.method + " " + req.url } };
    });
  }

  beforeEach(() => {
    requests.length = 0;
  });

  it("confirms first, sends DELETE, and forgets the mapping on success", async () => {
    stubServer(false, 204);
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: false });

    await plugin.unpublishNote(FILE);

    // Confirmation is shown before anything is deleted.
    expect(confirms).toHaveLength(1);
    expect(confirms[0]!.title).toBe("Unpublish /sara-goals?");
    expect(requests.filter((r) => r.method === "DELETE")).toHaveLength(0);

    await confirms[0]!.onConfirm();

    expect(requests.filter((r) => r.method === "DELETE")).toHaveLength(1);
    expect(plugin.getLocalRouteEntry(FILE.path)).toBeNull();
  });

  it("drops a stale mapping without deleting when the server says the route is free", async () => {
    stubServer(true, 204);
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: false });

    await plugin.unpublishNote(FILE);

    expect(confirms).toHaveLength(0);
    expect(requests.filter((r) => r.method === "DELETE")).toHaveLength(0);
    expect(plugin.getLocalRouteEntry(FILE.path)).toBeNull();
  });

  it("treats DELETE 404 as already unpublished and cleans up the mapping", async () => {
    stubServer(false, 404);
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: false });

    await plugin.unpublishNote(FILE);
    await confirms[0]!.onConfirm();

    expect(plugin.getLocalRouteEntry(FILE.path)).toBeNull();
  });

  it("keeps the mapping when the delete fails for another reason (server error)", async () => {
    stubServer(false, 500);
    const plugin = await newPlugin();
    await plugin.rememberRoute(FILE.path, { route: "sara-goals", passwordProtected: false });

    await plugin.unpublishNote(FILE);
    await confirms[0]!.onConfirm();

    expect(requests.filter((r) => r.method === "DELETE")).toHaveLength(1);
    expect(plugin.getLocalRouteEntry(FILE.path)).toEqual({ route: "sara-goals", passwordProtected: false });
  });

  it("does nothing beyond a Notice when the note has no publish record", async () => {
    stubServer(false, 204);
    const plugin = await newPlugin();

    await plugin.unpublishNote(FILE);

    expect(confirms).toHaveLength(0);
    expect(requests).toHaveLength(0);
  });
});

/**
 * Regression tests for the stale note↔route mapping bug: a note unpublished
 * elsewhere (settings, another device) left a local map entry behind, so the
 * publish modal opened in update mode and the PUT 404'd.
 *
 * openPublishModal must verify the local entry against the server and drop it
 * when the route is free again.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  TFile,
  __resetPluginData,
  __setRequestUrlHandler,
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

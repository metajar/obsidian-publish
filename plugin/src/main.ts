import { Notice, Plugin, TFile } from "obsidian";
import { PublishApiClient } from "./api";
import { PublishModal } from "./publishModal";
import { PublishSettingTab } from "./settings";

/**
 * Local (per-vault) plugin state.
 *
 * `noteRoutes` is a convenience map only — the server is always the source of
 * truth for what is published. It exists purely so the publish modal can
 * pre-fill a note's current route and password state.
 */
export interface PublishSettings {
  serverUrl: string;
  apiToken: string;
  noteRoutes: Record<string, NoteRouteEntry>;
}

export interface NoteRouteEntry {
  route: string;
  passwordProtected: boolean;
}

const DEFAULT_SETTINGS: PublishSettings = {
  serverUrl: "",
  apiToken: "",
  noteRoutes: {},
};

export default class SelfHostedPublishPlugin extends Plugin {
  settings: PublishSettings = DEFAULT_SETTINGS;

  async onload(): Promise<void> {
    await this.loadSettings();

    this.addRibbonIcon("send", "Publish current note", () => this.publishActiveNote());

    this.addCommand({
      id: "publish-current-note",
      name: "Publish this note",
      checkCallback: (checking: boolean) => {
        const file = this.app.workspace.getActiveFile();
        if (!file || file.extension !== "md") return false;
        if (!checking) this.openPublishModal(file);
        return true;
      },
    });

    this.registerEvent(
      this.app.workspace.on("editor-menu", (menu, _editor, info) => {
        const file =
          (info as { file?: TFile | null }).file ?? this.app.workspace.getActiveFile();
        if (!file) return;
        menu.addItem((item) =>
          item
            .setTitle("Publish")
            .setIcon("cloud-upload")
            .onClick(() => this.openPublishModal(file)),
        );
      }),
    );

    this.addSettingTab(new PublishSettingTab(this.app, this));
  }

  /** Publish entry point shared by the ribbon icon and the settings button. */
  publishActiveNote(): void {
    const file = this.app.workspace.getActiveFile();
    if (!file || file.extension !== "md") {
      new Notice("Publish: open a Markdown note first.");
      return;
    }
    this.openPublishModal(file);
  }

  async openPublishModal(file: TFile): Promise<void> {
    const client = this.getClient();
    let existing = this.getLocalRouteEntry(file.path);
    if (existing) {
      // The local map is a convenience only — verify against the server
      // before offering update mode, or a stale entry (page unpublished
      // elsewhere) would send the modal into a PUT that 404s.
      const check = await client.checkRouteAvailable(existing.route);
      if (check.ok && check.data) {
        await this.forgetRoute(existing.route);
        existing = null;
      }
    }
    new PublishModal(this.app, this, file, client, existing).open();
  }

  getClient(): PublishApiClient {
    return new PublishApiClient(this.settings.serverUrl, this.settings.apiToken);
  }

  /** Best-effort local record for pre-fills; never treated as truth. */
  getLocalRouteEntry(notePath: string): NoteRouteEntry | null {
    return this.settings.noteRoutes[notePath] ?? null;
  }

  async rememberRoute(notePath: string, entry: NoteRouteEntry): Promise<void> {
    this.settings.noteRoutes[notePath] = entry;
    await this.saveSettings();
  }

  /**
   * Update the protection flag on any local mapping pointing at `route`
   * (after a settings-table password set/remove). Pre-fill convenience only.
   */
  async markRoutePasswordProtected(route: string, passwordProtected: boolean): Promise<void> {
    let changed = false;
    for (const entry of Object.values(this.settings.noteRoutes)) {
      if (entry.route === route && entry.passwordProtected !== passwordProtected) {
        entry.passwordProtected = passwordProtected;
        changed = true;
      }
    }
    if (changed) await this.saveSettings();
  }

  /** Drop any local mapping pointing at `route` (after unpublish/move). */
  async forgetRoute(route: string): Promise<void> {
    let changed = false;
    for (const [notePath, entry] of Object.entries(this.settings.noteRoutes)) {
      if (entry.route === route) {
        delete this.settings.noteRoutes[notePath];
        changed = true;
      }
    }
    if (changed) await this.saveSettings();
  }

  async loadSettings(): Promise<void> {
    const stored = (await this.loadData()) as Partial<PublishSettings> | null;
    this.settings = {
      ...DEFAULT_SETTINGS,
      ...(stored ?? {}),
      noteRoutes: stored?.noteRoutes ?? {},
    };
  }

  async saveSettings(): Promise<void> {
    await this.saveData(this.settings);
  }
}

export function showNotice(message: string, timeout = 5000): void {
  new Notice(message, timeout);
}

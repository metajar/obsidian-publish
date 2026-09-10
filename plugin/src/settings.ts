import { App, Notice, PluginSettingTab, Setting, TextComponent } from "obsidian";
import { describeApiError, PageRecord } from "./api";
import { ConfirmModal } from "./confirmModal";
import type SelfHostedPublishPlugin from "./main";

export class PublishSettingTab extends PluginSettingTab {
  private pagesContainerEl: HTMLElement | null = null;

  constructor(app: App, private readonly plugin: SelfHostedPublishPlugin) {
    super(app, plugin);
  }

  display(): void {
    const { containerEl } = this;
    containerEl.empty();
    containerEl.addClass("op-settings");

    this.renderConnectionSection(containerEl);
    this.renderPagesSection(containerEl);
  }

  // -- Server connection -----------------------------------------------------

  private renderConnectionSection(containerEl: HTMLElement): void {
    new Setting(containerEl).setName("Server connection").setHeading();

    new Setting(containerEl)
      .setName("Server URL")
      .setDesc("Base URL of your publish server, e.g. https://notes.example.com")
      .addText((text: TextComponent) =>
        text
          .setPlaceholder("https://notes.example.com")
          .setValue(this.plugin.settings.serverUrl)
          .onChange(async (value: string) => {
            this.plugin.settings.serverUrl = value.trim();
            await this.plugin.saveSettings();
          }),
      );

    new Setting(containerEl)
      .setName("API token")
      .setDesc("Bearer token generated when the server was set up.")
      .addText((text: TextComponent) => {
        text.inputEl.type = "password";
        text
          .setPlaceholder("paste your token")
          .setValue(this.plugin.settings.apiToken)
          .onChange(async (value: string) => {
            this.plugin.settings.apiToken = value.trim();
            await this.plugin.saveSettings();
          });
      });

    new Setting(containerEl).addButton((btn) =>
      btn
        .setButtonText("Test connection")
        .onClick(async () => {
          btn.setDisabled(true);
          try {
            const result = await this.plugin.getClient().listPages();
            if (result.ok) {
              new Notice(
                `Connected — ${result.data.length} page${result.data.length === 1 ? "" : "s"} published.`,
              );
            } else {
              new Notice(describeApiError(result.error), 8000);
            }
          } finally {
            btn.setDisabled(false);
          }
        }),
    );
  }

  // -- Published pages ---------------------------------------------------------

  private renderPagesSection(containerEl: HTMLElement): void {
    new Setting(containerEl)
      .setName("Published pages")
      .setDesc("Fetched live from the server.")
      .setHeading()
      .addButton((btn) =>
        btn
          .setButtonText("Refresh")
          .setIcon("refresh-cw")
          .onClick(() => void this.renderPages()),
      );

    this.pagesContainerEl = containerEl.createEl("div", {
      cls: "op-pages-container",
    });
    void this.renderPages();
  }

  private async renderPages(): Promise<void> {
    const container = this.pagesContainerEl;
    if (!container) return;
    container.empty();

    const loading = container.createEl("p", {
      cls: "op-muted",
      text: "Loading published pages…",
    });

    // Always live from the server — never from local plugin data.
    const result = await this.plugin.getClient().listPages();
    loading.remove();

    if (!result.ok) {
      container.createEl("p", {
        cls: "op-error",
        text: describeApiError(result.error),
      });
      return;
    }

    if (result.data.length === 0) {
      container.createEl("p", {
        cls: "op-muted",
        text: "No published pages yet. Run “Publish this note” from a note to get started.",
      });
      return;
    }

    this.renderPagesTable(container, result.data);
  }

  private renderPagesTable(container: HTMLElement, pages: PageRecord[]): void {
    const table = container.createEl("table", { cls: "op-pages-table" });
    const head = table.createEl("thead");
    const headRow = head.createEl("tr");
    for (const col of ["Route", "Title", "Published", "Last updated", "Password", ""]) {
      headRow.createEl("th", { text: col });
    }
    const body = table.createEl("tbody");

    for (const page of pages) {
      const row = body.createEl("tr");

      row.createEl("td", { cls: "op-route-cell" }).createEl("code", {
        text: `/${page.route}`,
      });
      row.createEl("td", { text: page.title });
      row.createEl("td", { text: formatDate(page.created_at) });
      row.createEl("td", { text: formatDate(page.updated_at) });
      row.createEl("td", {
        text: page.password_protected ? "Protected" : "—",
      });

      const actions = row.createEl("td", { cls: "op-actions-cell" });
      const copyBtn = actions.createEl("button", {
        text: "Copy link",
        cls: "op-link-button",
      });
      copyBtn.addEventListener("click", async () => {
        await navigator.clipboard.writeText(this.liveUrl(page.route));
        new Notice("Link copied to clipboard.");
      });

      const unpublishBtn = actions.createEl("button", {
        text: "Unpublish",
        cls: "op-danger-button",
      });
      unpublishBtn.addEventListener("click", () => {
        new ConfirmModal(this.app, {
          title: `Unpublish /${page.route}?`,
          body: `“${page.title}” will stop resolving immediately — readers will get a 404. This cannot be undone from the plugin, but you can publish it again at any time.`,
          confirmText: "Unpublish",
          onConfirm: async () => void this.unpublish(page),
        }).open();
      });
    }
  }

  private async unpublish(page: PageRecord): Promise<void> {
    const result = await this.plugin.getClient().deletePage(page.route);
    if (result.ok) {
      new Notice(`Unpublished /${page.route}.`);
      await this.plugin.forgetRoute(page.route);
    } else {
      new Notice(describeApiError(result.error), 8000);
    }
    await this.renderPages();
  }

  private liveUrl(route: string): string {
    const base = this.plugin.settings.serverUrl.trim().replace(/\/+$/, "");
    return `${base}/${route}`;
  }
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

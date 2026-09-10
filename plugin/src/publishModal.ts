import { App, ButtonComponent, Modal, Notice, Setting, TFile, TextComponent } from "obsidian";
import { describeApiError, PageRecord, PublishApiClient } from "./api";
import { publishNoteImages, type ImagePublishOutcome } from "./imagePublisher";
import type SelfHostedPublishPlugin from "./main";
import { isValidRoute, slugify } from "./slug";

const AVAILABILITY_DEBOUNCE_MS = 400;

/** How the password field of an existing/protected page should behave. */
type PasswordMode = "none" | "keep" | "set" | "remove";

interface PasswordUiState {
  mode: PasswordMode;
  /** Plaintext password while typing — never persisted, never echoed back from the server (it only has a hash). */
  draft: string;
}

/** Generate a URL-unambiguous password using the platform CSPRNG. */
export function generatePassword(length = 20): string {
  // Alphabet excludes easily-confused characters (l, I, 1, O, 0).
  const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const values = new Uint32Array(length);
  crypto.getRandomValues(values);
  let out = "";
  for (let i = 0; i < values.length; i++) {
    out += alphabet[values[i] % alphabet.length];
  }
  return out;
}

export class PublishModal extends Modal {
  private route = "";
  private routeStatusEl: HTMLElement | null = null;
  private availabilityTimer: number | null = null;
  private submitting = false;

  private passwordState: PasswordUiState;

  constructor(
    app: App,
    private readonly plugin: SelfHostedPublishPlugin,
    private readonly file: TFile,
    private readonly client: PublishApiClient,
    private readonly existing: { route: string; passwordProtected: boolean } | null,
  ) {
    super(app);
    this.route = existing?.route ?? slugify(file.basename);
    this.passwordState = {
      mode: existing?.passwordProtected ? "keep" : "none",
      draft: "",
    };
  }

  async onOpen(): Promise<void> {
    this.titleEl.setText(
      this.existing ? "Update published note" : "Publish note",
    );

    const { contentEl } = this;
    contentEl.empty();
    contentEl.addClass("op-publish-modal");

    new Setting(contentEl)
      .setName("Note")
      .setDesc(this.file.basename)
      .setHeading()
      .settingEl.addClass("op-note-heading");

    // --- Route ------------------------------------------------------------
    const routeSetting = new Setting(contentEl)
      .setName("Route")
      .setDesc("URL-safe slug: lowercase letters, digits, single hyphens.");
    routeSetting.addText((text: TextComponent) => {
      text.setValue(this.route).onChange((value: string) => {
        this.route = value.trim();
        this.onRouteChanged();
      });
    });
    this.routeStatusEl = contentEl.createEl("div", {
      cls: "op-route-status",
      text: "",
    });

    // --- Password ---------------------------------------------------------
    this.renderPasswordSection();

    // --- Actions ------------------------------------------------------------
    new Setting(contentEl).addButton((btn: ButtonComponent) =>
      btn
        .setButtonText(this.submitLabel())
        .setCta()
        .onClick(() => this.submit()),
    );

    // Validate + check the pre-filled route once the modal is up.
    this.onRouteChanged();
  }

  onClose(): void {
    this.clearAvailabilityTimer();
    this.contentEl.empty();
  }

  // -----------------------------------------------------------------------

  private submitLabel(): string {
    if (!this.existing) return "Publish";
    return this.route === this.existing.route ? "Update" : "Move & update";
  }

  private renderPasswordSection(): void {
    const { contentEl } = this;
    const container = contentEl.createEl("div", { cls: "op-password-section" });

    if (this.existing?.passwordProtected) {
      // Updating a protected page: the server only holds a hash, so we can
      // only offer keep / replace / remove — never display the old value.
      new Setting(container).setName("Password protection").addDropdown((drop) => {
        drop
          .addOption("keep", "Keep existing password")
          .addOption("set", "Set a new password")
          .addOption("remove", "Remove password")
          .setValue(this.passwordState.mode)
          .onChange((value: string) => {
            this.passwordState.mode = value as PasswordMode;
            this.renderDraftField(container);
          });
      });
      this.renderDraftField(container);
    } else {
      new Setting(container).setName("Password protect").addToggle((toggle) => {
        toggle
          .setValue(this.passwordState.mode === "set")
          .onChange((on: boolean) => {
            this.passwordState.mode = on ? "set" : "none";
            this.renderDraftField(container);
          });
      });
      if (this.passwordState.mode === "set") this.renderDraftField(container);
    }
  }

  private renderDraftField(container: HTMLElement): void {
    container.querySelectorAll(".op-password-draft").forEach((el) => el.remove());
    if (this.passwordState.mode !== "set") return;

    const setting = new Setting(container).setName("Password").setClass("op-password-draft");
    setting.addText((text: TextComponent) => {
      text.inputEl.type = "password";
      text.setValue(this.passwordState.draft).onChange((value: string) => {
        this.passwordState.draft = value;
      });
    });
    setting.addButton((btn: ButtonComponent) =>
      btn.setButtonText("Generate").onClick(() => {
        const generated = generatePassword();
        this.passwordState.draft = generated;
        const input = setting.settingEl.querySelector("input");
        if (input instanceof HTMLInputElement) input.value = generated;
        // Surface the generated password once so the user can share it.
        new Notice(`Generated password: ${generated}`, 15000);
      }),
    );
  }

  private onRouteChanged(): void {
    if (this.routeStatusEl) this.routeStatusEl.empty();

    if (!this.route) {
      this.setRouteStatus("Enter a route for this note.", "op-route-muted");
      return;
    }
    if (!isValidRoute(this.route)) {
      this.setRouteStatus(
        "Invalid route — use lowercase letters, digits and single hyphens (max 64 chars).",
        "op-route-bad",
      );
      return;
    }
    if (this.existing && this.route === this.existing.route) {
      this.setRouteStatus("Current route for this note.", "op-route-muted");
      this.clearAvailabilityTimer();
      return;
    }

    this.clearAvailabilityTimer();
    this.setRouteStatus("Checking availability…", "op-route-muted");
    this.availabilityTimer = window.setTimeout(() => {
      this.availabilityTimer = null;
      void this.checkAvailability();
    }, AVAILABILITY_DEBOUNCE_MS);
  }

  private clearAvailabilityTimer(): void {
    if (this.availabilityTimer !== null) {
      window.clearTimeout(this.availabilityTimer);
      this.availabilityTimer = null;
    }
  }

  private async checkAvailability(): Promise<void> {
    const route = this.route;
    if (!isValidRoute(route)) return;
    const result = await this.client.checkRouteAvailable(route);
    // The user may have kept typing; only show feedback for the current route.
    if (this.route !== route || !this.routeStatusEl) return;

    if (result.ok) {
      if (result.data) {
        this.setRouteStatus(`/${route} is available.`, "op-route-ok");
      } else {
        this.setRouteStatus(
          `/${route} is already taken by another page.`,
          "op-route-bad",
        );
      }
    } else {
      this.setRouteStatus(
        `Could not check availability: ${describeApiError(result.error)}`,
        "op-route-bad",
      );
    }
  }

  private setRouteStatus(text: string, cls: string): void {
    if (!this.routeStatusEl) return;
    this.routeStatusEl.className = `op-route-status ${cls}`;
    this.routeStatusEl.setText(text);
  }

  // -----------------------------------------------------------------------

  private async submit(): Promise<void> {
    if (this.submitting) return;

    if (!this.route || !isValidRoute(this.route)) {
      new Notice("Publish: please choose a valid route first.");
      return;
    }
    if (this.passwordState.mode === "set" && this.passwordState.draft.trim().length === 0) {
      new Notice("Publish: enter a password or generate one.");
      return;
    }
    if (this.existing && this.route !== this.existing.route) {
      // Moving a published page to a new route: create at the new route,
      // then unpublish the old one.
      if (this.passwordState.mode === "keep") {
        new Notice(
          "Publish: moving to a new route requires a new password — the old one cannot be carried over. Choose “Set a new password” or “Remove password”.",
          8000,
        );
        return;
      }
      const confirmed = await this.confirmMove();
      if (!confirmed) return;
    }

    this.submitting = true;
    try {
      let markdown: string;
      try {
        markdown = await this.plugin.app.vault.read(this.file);
      } catch (err) {
        new Notice(`Publish: could not read the note — ${err instanceof Error ? err.message : String(err)}`);
        return;
      }

      const title = this.file.basename;
      const password =
        this.passwordState.mode === "set"
          ? this.passwordState.draft
          : this.passwordState.mode === "remove"
            ? null
            : undefined; // "keep" (protected update) or "none" (unprotected)

      // Upload embedded images and rewrite the embeds to server URLs in the
      // outbound payload. The note on disk is never modified (see
      // imagePublisher.ts). Failures fail soft — publishing continues.
      const images = await publishNoteImages({
        markdown,
        vault: this.plugin.app.vault,
        client: this.client,
        baseUrl: this.plugin.settings.serverUrl,
      });

      const isUpdate = this.existing !== null && this.route === this.existing.route;
      const result = isUpdate
        ? await this.client.updatePage(this.route, { title, markdown: images.markdown, password })
        : await this.client.createPage(this.route, { title, markdown: images.markdown, password: password ?? undefined });

      if (!result.ok) {
        new Notice(describeApiError(result.error), 8000);
        this.onRouteChanged(); // re-run availability feedback for the failed route
        return;
      }

      // Move semantics: retire the old route once the new one is live.
      if (this.existing && this.route !== this.existing.route) {
        const oldRoute = this.existing.route;
        const removal = await this.client.deletePage(oldRoute);
        if (!removal.ok) {
          new Notice(
            `Published at /${this.route}, but the old route /${oldRoute} could not be removed — ${describeApiError(removal.error)}`,
            10000,
          );
        }
      }

      await this.plugin.rememberRoute(this.file.path, {
        route: this.route,
        passwordProtected: this.passwordState.mode === "set" || this.passwordState.mode === "keep",
      });

      const url = this.liveUrl(result.data);
      new Notice(`Published: /${this.route}`);
      this.showAttachmentNotices(images);
      new CopyLinkModal(this.app, url).open();
      this.close();
    } finally {
      this.submitting = false;
    }
  }

  /** Post-publish feedback for embedded attachments (both are advisory only). */
  private showAttachmentNotices(images: ImagePublishOutcome): void {
    if (images.nonImageCount > 0) {
      new Notice(
        `Publish: ${images.nonImageCount} non-image attachment${images.nonImageCount === 1 ? " was" : "s were"} not published.`,
        8000,
      );
    }
    if (images.failed.length > 0) {
      new Notice(
        `Publish: ${images.failed.length} image${images.failed.length === 1 ? "" : "s"} could not be uploaded and ${images.failed.length === 1 ? "was" : "were"} left as vault embeds — ${images.failed.join(", ")}`,
        10000,
      );
    }
  }

  private liveUrl(page: PageRecord): string {
    if (page.url) return page.url;
    const base = this.plugin.settings.serverUrl.trim().replace(/\/+$/, "");
    return `${base}/${this.route}`;
  }

  private confirmMove(): Promise<boolean> {
    return new Promise((resolve) => {
      const modal = new Modal(this.app);
      modal.titleEl.setText("Move published page?");
      modal.contentEl.createEl("p", {
        text: `This note is currently published at /${this.existing?.route}. It will be re-published at /${this.route} and the old route will stop resolving.`,
      });
      new Setting(modal.contentEl)
        .addButton((btn: ButtonComponent) =>
          btn.setButtonText("Cancel").onClick(() => {
            modal.close();
            resolve(false);
          }),
        )
        .addButton((btn: ButtonComponent) =>
          btn
            .setButtonText("Move & update")
            .setCta()
            .onClick(() => {
              modal.close();
              resolve(true);
            }),
        );
      modal.open();
    });
  }
}

/** Success dialog with the live URL and a copy-to-clipboard action. */
export class CopyLinkModal extends Modal {
  constructor(app: App, private readonly url: string) {
    super(app);
  }

  onOpen(): void {
    this.titleEl.setText("Page is live");
    const urlEl = this.contentEl.createEl("div", { cls: "op-live-url" });
    urlEl.createEl("a", { text: this.url, href: this.url });

    new Setting(this.contentEl).addButton((btn: ButtonComponent) =>
      btn
        .setButtonText("Copy link")
        .setCta()
        .onClick(async () => {
          await navigator.clipboard.writeText(this.url);
          new Notice("Link copied to clipboard.");
        }),
    ).addButton((btn: ButtonComponent) =>
      btn.setButtonText("Done").onClick(() => this.close()),
    );
  }

  onClose(): void {
    this.contentEl.empty();
  }
}

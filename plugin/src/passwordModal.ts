import { App, ButtonComponent, Modal, Notice, Setting } from "obsidian";

/** Default note for the show-once dialog — no plaintext is ever persisted. */
export const PASSWORD_NOT_STORED_NOTE =
  "Shown once — this password is not stored anywhere. Copy it now if you need to share it.";

export interface ShowPasswordModalOptions {
  /** Modal title (default "Generated password"). */
  title?: string;
  /** Note shown under the password (default: the shown-once/not-stored note). */
  note?: string;
}

/**
 * Show-once dialog for a freshly generated password.
 *
 * Shared by the publish modal's Generate button and the settings-table
 * password action. The plaintext lives only in this dialog (and, for the
 * publish modal, the transient draft field) — it is never written to plugin
 * data and never logged.
 */
export class ShowPasswordModal extends Modal {
  constructor(
    app: App,
    private readonly password: string,
    private readonly options: ShowPasswordModalOptions = {},
  ) {
    super(app);
  }

  onOpen(): void {
    this.titleEl.setText(this.options.title ?? "Generated password");
    this.contentEl.createEl("div", {
      cls: "op-password-reveal",
      text: this.password,
    });
    this.contentEl.createEl("p", {
      cls: "op-muted op-password-note",
      text: this.options.note ?? PASSWORD_NOT_STORED_NOTE,
    });

    new Setting(this.contentEl).addButton((btn: ButtonComponent) =>
      btn
        .setButtonText("Copy password")
        .setCta()
        .onClick(async () => {
          await navigator.clipboard.writeText(this.password);
          new Notice("Password copied to clipboard.");
        }),
    ).addButton((btn: ButtonComponent) =>
      btn.setButtonText("Done").onClick(() => this.close()),
    );
  }

  onClose(): void {
    this.contentEl.empty();
  }
}

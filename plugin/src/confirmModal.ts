import { App, ButtonComponent, Modal, Setting } from "obsidian";

export interface ConfirmModalOptions {
  title: string;
  body: string;
  confirmText: string;
  cancelText?: string;
  confirmClass?: string;
  onConfirm: () => void | Promise<void>;
}

/** Small generic confirmation dialog. */
export class ConfirmModal extends Modal {
  constructor(app: App, private readonly options: ConfirmModalOptions) {
    super(app);
  }

  onOpen(): void {
    this.titleEl.setText(this.options.title);
    new Setting(this.contentEl)
      .setDesc(this.options.body)
      .addButton((btn: ButtonComponent) =>
        btn
          .setButtonText(this.options.cancelText ?? "Cancel")
          .onClick(() => this.close()),
      )
      .addButton((btn: ButtonComponent) =>
        btn
          .setButtonText(this.options.confirmText)
          .setCta()
          .onClick(async () => {
            this.close();
            await this.options.onConfirm();
          }),
      );
  }
}

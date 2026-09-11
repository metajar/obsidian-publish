/**
 * Minimal mock of the `obsidian` module for unit tests.
 *
 * Only what `src/api.ts` needs is implemented. `requestUrl` dispatches to a
 * handler installed per-test via `__setRequestUrlHandler`, so tests can make
 * it resolve with an HTTP status or reject with a network error.
 */

export type MockRequest = {
  url: string;
  method: string;
  headers?: Record<string, string>;
  /** JSON payloads arrive as strings; multipart asset uploads as ArrayBuffer. */
  body?: string | ArrayBuffer;
  throw?: boolean;
};

export type MockResponse = {
  status: number;
  text?: string;
  json?: unknown;
};

export type MockHandler = (req: MockRequest) => Promise<MockResponse>;

let handler: MockHandler = async () => {
  throw new Error("no requestUrl handler installed for this test");
};

export function __setRequestUrlHandler(h: MockHandler): void {
  handler = h;
}

export async function requestUrl(req: MockRequest): Promise<MockResponse & { text: string; json?: unknown }> {
  const res = await handler(req);
  const text = res.text ?? (res.json !== undefined ? JSON.stringify(res.json) : "");
  return { ...res, text };
}

export class Notice {
  constructor(_message: string, _timeout?: number) {}
}

// -- Minimal DOM/modal mocks for modal-based components ----------------------
//
// Only the surface used by ShowPasswordModal / ConfirmModal is implemented:
// a tiny tree of FakeEl nodes plus no-op Modal/Setting/ButtonComponent
// classes that record what was rendered so tests can assert on it.

export class FakeEl {
  readonly children: FakeEl[] = [];
  text = "";
  readonly classes: string[] = [];
  readonly attrs: Record<string, string> = {};

  constructor(readonly tag: string) {}

  createEl(tag: string, opts?: { cls?: string; text?: string; attr?: Record<string, string> }): FakeEl {
    const el = new FakeEl(tag);
    if (opts?.cls) el.classes.push(...opts.cls.split(/\s+/));
    if (opts?.text) el.text = opts.text;
    if (opts?.attr) Object.assign(el.attrs, opts.attr);
    this.children.push(el);
    return el;
  }

  addClass(cls: string): void {
    this.classes.push(cls);
  }

  setText(text: string): void {
    this.text = text;
  }

  empty(): void {
    this.children.length = 0;
    this.text = "";
  }

  /** Find descendants (incl. self) that carry `cls`. */
  findAll(cls: string): FakeEl[] {
    const out: FakeEl[] = [];
    const walk = (el: FakeEl): void => {
      if (el.classes.includes(cls)) out.push(el);
      el.children.forEach(walk);
    };
    walk(this);
    return out;
  }

  /** Concatenated text of this node and all descendants. */
  fullText(): string {
    return [this.text, ...this.children.map((c) => c.fullText())].join("");
  }
}

export class Modal {
  readonly contentEl = new FakeEl("div");
  readonly titleEl = new FakeEl("h2");
  openCalls = 0;
  closeCalls = 0;

  constructor(public app: unknown) {}

  open(): void {
    this.openCalls++;
  }

  close(): void {
    this.closeCalls++;
  }
}

export class ButtonComponent {
  buttonText = "";
  isCta = false;
  clickHandler: (() => void | Promise<void>) | null = null;

  setButtonText(text: string): this {
    this.buttonText = text;
    return this;
  }

  setCta(): this {
    this.isCta = true;
    return this;
  }

  setDisabled(_disabled: boolean): this {
    return this;
  }

  onClick(cb: () => void | Promise<void>): this {
    this.clickHandler = cb;
    return this;
  }

  async click(): Promise<void> {
    await this.clickHandler?.();
  }
}

/** All Settings created since the last `__resetDom` (per-test isolation). */
export const __settings: Setting[] = [];

export function __resetDom(): void {
  __settings.length = 0;
}

export class Setting {
  readonly settingEl = new FakeEl("div");
  readonly buttons: ButtonComponent[] = [];

  constructor(public containerEl: FakeEl) {
    containerEl.children.push(this.settingEl);
    __settings.push(this);
  }

  setName(_name?: string): this {
    return this;
  }

  setDesc(_desc?: string): this {
    return this;
  }

  setClass(cls: string): this {
    this.settingEl.addClass(cls);
    return this;
  }

  addButton(cb: (button: ButtonComponent) => unknown): this {
    const btn = new ButtonComponent();
    this.buttons.push(btn);
    cb(btn);
    return this;
  }
}

/** No-op stand-in for Obsidian's global icon helper. */
export function setIcon(parent: FakeEl, _iconId: string): void {
  parent.createEl("i", { cls: "mock-icon" });
}


// -- Plugin lifecycle mocks (for main.ts tests) ------------------------------

export class TextComponent {
  constructor(_el?: unknown) {}
  setValue(_v: string): this { return this; }
  setPlaceholder(_p: string): this { return this; }
  onChange(_cb: () => void): this { return this; }
}

export class PluginSettingTab {
  containerEl = new FakeEl("div");
  constructor(public app: unknown, public plugin: unknown) {}
  display(): void {}
  hide(): void {}
}

export class TFile {
  constructor(
    public path = "note.md",
    public extension = "md",
    public basename = "note",
  ) {}
}

/** loadData/saveData round-trip through a global, so tests can seed plugin data. */
export class Plugin {
  app: unknown = {};
  manifest: unknown = {};
  async loadData(): Promise<unknown> {
    return (globalThis as Record<string, unknown>).__pluginData ?? null;
  }
  async saveData(data: unknown): Promise<void> {
    (globalThis as Record<string, unknown>).__pluginData = data;
  }
}

export function __resetPluginData(): void {
  delete (globalThis as Record<string, unknown>).__pluginData;
}

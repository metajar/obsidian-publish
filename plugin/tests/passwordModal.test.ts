import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { App } from "obsidian";
import { PASSWORD_NOT_STORED_NOTE, ShowPasswordModal } from "../src/passwordModal";
import { FakeEl, Modal, __resetDom, __settings } from "./mocks/obsidian";

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  __resetDom();
  writeText.mockClear();
  // Node has no navigator.clipboard; provide one for the copy action.
  vi.stubGlobal("navigator", { clipboard: { writeText } });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

/** Open the modal (invoking onOpen) against the mock Modal base. */
function openModal(
  password: string,
  options?: ConstructorParameters<typeof ShowPasswordModal>[2],
): { modal: ShowPasswordModal; base: Modal; content: FakeEl } {
  const modal = new ShowPasswordModal({} as App, password, options);
  modal.onOpen();
  const base = modal as unknown as Modal;
  return { modal, base, content: base.contentEl };
}

/** The button a modal's Setting created with the given text, if any. */
function findButton(text: string) {
  for (const setting of __settings) {
    const btn = setting.buttons.find((b) => b.buttonText === text);
    if (btn) return btn;
  }
  return null;
}

describe("ShowPasswordModal", () => {
  it("shows the password with the shown-once, not-stored note by default", () => {
    const { base, content } = openModal("correct-horse-battery");
    expect(base.titleEl.text).toBe("Generated password");

    const reveal = content.findAll("op-password-reveal");
    expect(reveal).toHaveLength(1);
    expect(reveal[0].text).toBe("correct-horse-battery");

    const note = content.findAll("op-password-note");
    expect(note).toHaveLength(1);
    expect(note[0].text).toContain("Shown once");
    expect(note[0].text).toContain("not stored");
    expect(PASSWORD_NOT_STORED_NOTE).toContain("not stored");
  });

  it("supports a custom title and note (publish modal context)", () => {
    const { base, content } = openModal("pw123", {
      title: "New page password",
      note: "Copy it now if you want to share it.",
    });
    expect(base.titleEl.text).toBe("New page password");
    const note = content.findAll("op-password-note");
    expect(note[0].text).toBe("Copy it now if you want to share it.");
  });

  it("copy button copies the password to the clipboard", async () => {
    const { content } = openModal("staple-horse-correct");
    const copy = findButton("Copy password");
    expect(copy).not.toBeNull();
    expect(copy?.isCta).toBe(true);
    await copy!.click();
    expect(writeText).toHaveBeenCalledWith("staple-horse-correct");
  });

  it("done button closes the modal", async () => {
    const { base } = openModal("anything");
    const done = findButton("Done");
    expect(done).not.toBeNull();
    await done!.click();
    expect(base.closeCalls).toBe(1);
  });

  it("clears the password from the DOM on close", () => {
    const { modal, base } = openModal("erase-me");
    modal.onClose();
    expect(base.contentEl.children).toHaveLength(0);
    expect(base.contentEl.fullText()).not.toContain("erase-me");
  });
});

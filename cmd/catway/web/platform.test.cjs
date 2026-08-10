"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const platform = require("./platform.js");

function key(overrides = {}) {
  return {
    type: "keydown", code: "KeyA", key: "a",
    shiftKey: false, altKey: false, ctrlKey: false, metaKey: false,
    isComposing: false, keyCode: 65,
    getModifierState: () => false,
    ...overrides,
  };
}

test("descriptor is authoritative and absent descriptors remain browser-safe", () => {
  assert.deepEqual(platform.describe({ platform: "windows", nativeClipboard: true }, "MacIntel"), {
    platform: "windows", desktop: true, nativeClipboard: true,
  });
  assert.deepEqual(platform.describe(undefined, "MacIntel"), {
    platform: "macos", desktop: false, nativeClipboard: false,
  });
  assert.deepEqual(platform.describe(undefined, "Win32"), {
    platform: "windows", desktop: false, nativeClipboard: false,
  });
});

test("platform labels retain Mac chords and use Windows-safe chords", () => {
  const mac = platform.shortcuts(platform.describe({ platform: "macos" }));
  assert.equal(mac.palette, "⌘K");
  assert.equal(mac.paste, "⌘V");
  assert.equal(mac.fontIncrease, "⌘+");
  const win = platform.shortcuts(platform.describe({ platform: "windows" }));
  assert.equal(win.palette, "Ctrl+Alt+K");
  assert.equal(win.sidebar, "Ctrl+Alt+B");
  assert.equal(win.paste, "Ctrl+Shift+V");
  assert.equal(win.fontReset, "Ctrl+0");
});

test("printable, Control, Alt, AltGr, and modifyOtherKeys stay terminal-owned", () => {
  const win = platform.describe({ platform: "windows" });
  assert.equal(platform.keyboardAction(key(), win), "terminal");
  assert.equal(platform.keyboardAction(key({ code: "KeyC", key: "c", ctrlKey: true }), win), "terminal");
  assert.equal(platform.keyboardAction(key({ altKey: true }), win), "terminal");
  assert.equal(platform.keyboardAction(key({ code: "KeyK", key: "k", ctrlKey: true, altKey: true,
    getModifierState: (name) => name === "AltGraph" }), win), "terminal");
  assert.equal(platform.keyboardAction(key({ code: "KeyU", modifyOtherKeys: 2 }), win), "terminal");
});

test("IME/dead-key control events never reach the terminal", () => {
  const win = platform.describe({ platform: "windows" });
  assert.equal(platform.keyboardAction(key({ isComposing: true }), win), "ignore");
  assert.equal(platform.keyboardAction(key({ keyCode: 229 }), win), "ignore");
  assert.equal(platform.keyboardAction(key({ key: "Process" }), win), "ignore");
  assert.equal(platform.keyboardAction(key({ key: "Dead" }), win), "ignore");
  assert.equal(platform.keyboardAction(key(), win, true), "ignore");
});

test("palette, paste, font, edit, and terminal shortcut ownership", () => {
  const mac = platform.describe({ platform: "macos" });
  const win = platform.describe({ platform: "windows" });
  assert.equal(platform.keyboardAction(key({ code: "KeyK", metaKey: true }), mac), "palette");
  assert.equal(platform.keyboardAction(key({ code: "KeyB", metaKey: true }), mac), "sidebar-toggle");
  assert.equal(platform.keyboardAction(key({ code: "KeyV", metaKey: true }), mac), "paste");
  assert.equal(platform.keyboardAction(key({ code: "Equal", metaKey: true }), mac), "font-increase");
  assert.equal(platform.keyboardAction(key({ code: "KeyC", metaKey: true }), mac), "terminal");
  assert.equal(platform.keyboardAction(key({ code: "KeyQ", metaKey: true }), mac), "browser");

  assert.equal(platform.keyboardAction(key({ code: "KeyK", ctrlKey: true, altKey: true }), win), "palette");
  assert.equal(platform.keyboardAction(key({ code: "KeyB", ctrlKey: true, altKey: true }), win), "sidebar-toggle");
  assert.equal(platform.keyboardAction(key({ code: "KeyV", ctrlKey: true, shiftKey: true }), win), "paste");
  assert.equal(platform.keyboardAction(key({ code: "Equal", ctrlKey: true, shiftKey: true }), win), "font-increase");
  assert.equal(platform.keyboardAction(key({ code: "Minus", ctrlKey: true }), win), "font-decrease");
  assert.equal(platform.keyboardAction(key({ code: "Digit0", ctrlKey: true }), win), "font-reset");
  assert.equal(platform.keyboardAction(key({ code: "KeyZ", ctrlKey: true, shiftKey: true }), win), "browser");
});

test("clipboard prefers native bridges, falls back in browsers, and surfaces failure", async () => {
  const calls = [];
  await platform.clipboardWrite("osc52", {
    nativeWrite: async (text) => calls.push(["native-write", text]),
    browserClipboard: { writeText: async (text) => calls.push(["browser-write", text]) },
  });
  assert.deepEqual(calls, [["native-write", "osc52"]]);
  assert.equal(await platform.clipboardRead({ nativeRead: async () => "paste-button" }), "paste-button");
  await platform.clipboardWrite("selection", {
    browserClipboard: { writeText: async (text) => calls.push(["browser-write", text]) },
  });
  assert.deepEqual(calls.at(-1), ["browser-write", "selection"]);
  await assert.rejects(platform.clipboardRead({ nativeRead: async () => { throw new Error("busy"); } }), /busy/);
  await assert.rejects(platform.clipboardRead({}), /unavailable/);
});

test("editable controls own paste events instead of the terminal fallback", () => {
  assert.equal(platform.pasteTargetOwnsEvent({ tagName: "INPUT" }), true);
  assert.equal(platform.pasteTargetOwnsEvent({ tagName: "textarea" }), true);
  assert.equal(platform.pasteTargetOwnsEvent({ tagName: "SPAN", isContentEditable: true }), true);
  assert.equal(platform.pasteTargetOwnsEvent({ tagName: "CANVAS" }), false);
  assert.equal(platform.pasteTargetOwnsEvent(null), false);
});

test("notification plan covers permission fallback and visible/hidden panes", () => {
  assert.deepEqual(platform.notificationPlan({ kind: "attention", visible: true, focused: true }), {
    suppress: true, toast: false, native: false,
  });
  assert.deepEqual(platform.notificationPlan({ kind: "finished", visible: false, focused: true }), {
    suppress: false, toast: true, native: false,
  });
  assert.deepEqual(platform.notificationPlan({ kind: "attention", visible: true, focused: false,
    supported: true, permission: "granted" }), { suppress: false, toast: true, native: true });
  assert.equal(platform.notificationPlan({ kind: "attention", focused: false,
    supported: true, permission: "denied" }).native, false);
  assert.deepEqual(platform.notificationPlan({ kind: "update", focused: true }), {
    suppress: false, toast: true, native: false,
  });
  const bounded = platform.boundedNotification({ message: "x".repeat(200), body: "y".repeat(700), pane: 7 });
  assert.equal(bounded.title.length, 160);
  assert.equal(bounded.body.length, 512);
  assert.equal(bounded.pane, 7);
});

test("OSC 8 URLs and top-level origins are classified narrowly", () => {
  const base = "https://cats.example/session";
  assert.equal(platform.safeExternalURL("https://docs.example/a", base), "https://docs.example/a");
  assert.equal(platform.safeExternalURL("javascript:alert(1)", base), "");
  assert.equal(platform.safeExternalURL("file:///etc/passwd", base), "");
  assert.equal(platform.safeExternalURL("https://user:pass@docs.example", base), "");
  assert.equal(platform.sameOrigin("/next", base), true);
  assert.equal(platform.sameOrigin("https://other.example", base), false);
});

test("reconnect backoff is prompt for transient localhost failure and bounded for cold start", () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5, 20].map(platform.reconnectDelay),
    [250, 500, 1000, 1500, 3000, 5000, 5000]);
});

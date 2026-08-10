(function (root, factory) {
  "use strict";
  const api = Object.freeze(factory());
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.CatsPlatform = api;
})(typeof window === "object" ? window : globalThis, function () {
  "use strict";

  const PLATFORM_WINDOWS = "windows";
  const PLATFORM_MACOS = "macos";

  function describe(raw, navigatorPlatform) {
    const descriptor = raw && typeof raw === "object" ? raw : null;
    const named = descriptor && typeof descriptor.platform === "string"
      ? descriptor.platform.toLowerCase() : "";
    let platform = named === "windows" ? PLATFORM_WINDOWS
      : (named === "macos" || named === "darwin" || named === "mac") ? PLATFORM_MACOS
      : "browser";
    if (platform === "browser") {
      const hint = String(navigatorPlatform || "").toLowerCase();
      if (hint.includes("mac")) platform = PLATFORM_MACOS;
      else if (hint.includes("win")) platform = PLATFORM_WINDOWS;
    }
    return Object.freeze({
      platform,
      desktop: named === "windows" || named === "macos" || named === "darwin" || named === "mac",
      nativeClipboard: !!(descriptor && descriptor.nativeClipboard),
    });
  }

  function shortcuts(platform) {
    const mac = platform && platform.platform === PLATFORM_MACOS;
    return Object.freeze({
      palette: mac ? "⌘K" : "Ctrl+Alt+K",
      paletteKeys: mac ? ["⌘K", "Ctrl+Alt+K"] : ["Ctrl+Alt+K"],
      sidebar: mac ? "⌘B" : "Ctrl+Alt+B",
      paste: mac ? "⌘V" : "Ctrl+Shift+V",
      fontIncrease: mac ? "⌘+" : "Ctrl++",
      fontDecrease: mac ? "⌘-" : "Ctrl+-",
      fontReset: mac ? "⌘0" : "Ctrl+0",
      help: mac ? "⌘K → keyboard shortcuts" : "F1 or Ctrl+Alt+K → keyboard shortcuts",
    });
  }

  function hasAltGraph(event) {
    return !!(event && ((event.getModifierState && event.getModifierState("AltGraph")) || event.altGraph));
  }

  function isCompositionEvent(event, composing) {
    return !!(composing || (event && (event.isComposing || event.keyCode === 229 ||
      event.key === "Process" || event.key === "Dead")));
  }

  // keyboardAction classifies only app-global ownership. "terminal" means the
  // original structured KeyboardEvent must be forwarded unchanged; encoding
  // remains server-side so modifyOtherKeys/kitty state stays authoritative.
  function keyboardAction(event, platform, composing) {
    if (!event || isCompositionEvent(event, composing)) return "ignore";
    const down = event.type === "keydown";
    const altGraph = hasAltGraph(event);
    const mac = platform && platform.platform === PLATFORM_MACOS;
    const windows = platform && platform.platform === PLATFORM_WINDOWS;

    if (down && event.code === "KeyK" &&
        ((mac && event.metaKey && !event.ctrlKey) ||
         (event.ctrlKey && event.altKey && !event.metaKey && !altGraph))) return "palette";

    if (down && event.code === "KeyB" &&
        ((mac && event.metaKey && !event.ctrlKey) ||
         (windows && event.ctrlKey && event.altKey && !event.metaKey && !altGraph))) return "sidebar-toggle";

    if (down && event.code === "KeyV" &&
        ((mac && event.metaKey && !event.ctrlKey) ||
         (windows && event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey))) return "paste";

    const fontModifier = mac
      ? event.metaKey && !event.ctrlKey && !event.altKey
      : windows && event.ctrlKey && !event.altKey && !event.metaKey;
    if (down && fontModifier) {
      if (event.code === "Equal" || event.code === "NumpadAdd") return "font-increase";
      if (event.code === "Minus" || event.code === "NumpadSubtract") return "font-decrease";
      if (event.code === "Digit0" || event.code === "Numpad0") return "font-reset";
    }

    // Cocoa owns ordinary Command shortcuts other than the two terminal chords
    // preserved by the existing app. WebView2 owns these explicit shifted edit
    // chords; if an event slips past the native accelerator, never encode it.
    if (mac && event.metaKey && !event.ctrlKey && event.code !== "KeyC" && event.code !== "KeyZ") return "browser";
    if (windows && event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey &&
        ["KeyA", "KeyC", "KeyV", "KeyX", "KeyY", "KeyZ"].includes(event.code)) return "browser";
    if (event.code === "F12") return "browser";
    return "terminal";
  }

  function clipboardWrite(text, env) {
    if (env && typeof env.nativeWrite === "function") return Promise.resolve().then(() => env.nativeWrite(text));
    const clipboard = env && env.browserClipboard;
    if (!clipboard || typeof clipboard.writeText !== "function") return Promise.reject(new Error("clipboard write unavailable"));
    return Promise.resolve().then(() => clipboard.writeText(text));
  }

  function clipboardRead(env) {
    if (env && typeof env.nativeRead === "function") return Promise.resolve().then(() => env.nativeRead());
    const clipboard = env && env.browserClipboard;
    if (!clipboard || typeof clipboard.readText !== "function") return Promise.reject(new Error("clipboard read unavailable"));
    return Promise.resolve().then(() => clipboard.readText());
  }

  // pasteTargetOwnsEvent keeps the document-level terminal paste fallback out
  // of browser-editable controls. isContentEditable includes descendants of a
  // contenteditable host; tagName covers ordinary modal/chat form controls.
  function pasteTargetOwnsEvent(target) {
    if (!target || typeof target !== "object") return false;
    const tag = String(target.tagName || "").toLowerCase();
    return tag === "input" || tag === "textarea" || target.isContentEditable === true;
  }

  function notificationPlan(input) {
    const agent = input && (input.kind === "attention" || input.kind === "finished");
    if (!agent) return Object.freeze({ suppress: false, toast: true, native: false });
    const focused = !!(input && input.focused);
    const visible = !!(input && input.visible);
    if (focused && visible) return Object.freeze({ suppress: true, toast: false, native: false });
    return Object.freeze({
      suppress: false,
      toast: true,
      native: !focused && !!(input && input.supported) && input.permission === "granted",
    });
  }

  function boundedNotification(message) {
    const msg = message && typeof message === "object" ? message : {};
    const pane = Number.isSafeInteger(msg.pane) && msg.pane > 0 ? msg.pane : 0;
    return Object.freeze({
      title: String(msg.message || "cats").slice(0, 160),
      body: String(msg.body || "").slice(0, 512),
      pane,
    });
  }

  function safeExternalURL(raw, base) {
    if (typeof raw !== "string" || raw.length === 0 || raw.length > 4096) return "";
    try {
      const parsed = new URL(raw, base);
      if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || parsed.username || parsed.password) return "";
      return parsed.href;
    } catch (_) {
      return "";
    }
  }

  function sameOrigin(raw, base) {
    try {
      const target = new URL(raw, base);
      const current = new URL(base);
      return (target.protocol === "http:" || target.protocol === "https:") && target.origin === current.origin;
    } catch (_) {
      return false;
    }
  }

  function reconnectDelay(attempt) {
    const n = Math.max(0, Math.min(5, Number.isFinite(attempt) ? Math.floor(attempt) : 0));
    return [250, 500, 1000, 1500, 3000, 5000][n];
  }

  return {
    describe,
    shortcuts,
    hasAltGraph,
    isCompositionEvent,
    keyboardAction,
    clipboardWrite,
    clipboardRead,
    pasteTargetOwnsEvent,
    notificationPlan,
    boundedNotification,
    safeExternalURL,
    sameOrigin,
    reconnectDelay,
  };
});

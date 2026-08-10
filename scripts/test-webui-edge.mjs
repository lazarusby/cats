import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import process from "node:process";

const repo = path.resolve(path.dirname(new URL(import.meta.url).pathname.replace(/^\/(.:)/, "$1")), "..");
const indexPath = path.join(repo, "cmd", "catway", "web", "index.html");
const platformPath = path.join(repo, "cmd", "catway", "web", "platform.js");
const edge = process.env.CATS_EDGE_PATH || "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe";

function reservePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close((err) => err ? reject(err) : resolve(port));
    });
  });
}

function sleep(ms) { return new Promise((resolve) => setTimeout(resolve, ms)); }

async function poll(fn, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs;
  let last;
  while (Date.now() < deadline) {
    try {
      const value = await fn();
      if (value) return value;
      last = value;
    } catch (err) { last = err; }
    await sleep(50);
  }
  throw new Error(`timed out waiting for ${what}; last=${last}`);
}

class CDP {
  constructor(url) {
    this.ws = new WebSocket(url);
    this.seq = 0;
    this.pending = new Map();
  }
  async open() {
    await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error("timed out opening Edge DevTools WebSocket")), 5000);
      this.ws.addEventListener("open", () => { clearTimeout(timer); resolve(); }, { once: true });
      this.ws.addEventListener("error", reject, { once: true });
    });
    this.ws.addEventListener("message", async (event) => {
      try {
        let raw = event.data;
        if (raw instanceof Blob) raw = await raw.text();
        else if (raw instanceof ArrayBuffer) raw = new TextDecoder().decode(raw);
        const msg = JSON.parse(raw);
        const pending = this.pending.get(msg.id);
        if (!pending) return;
        this.pending.delete(msg.id);
        clearTimeout(pending.timer);
        if (msg.error) pending.reject(new Error(msg.error.message));
        else pending.resolve(msg.result);
      } catch (err) {
        for (const [id, pending] of this.pending) {
          this.pending.delete(id);
          clearTimeout(pending.timer);
          pending.reject(err);
        }
      }
    });
  }
  send(method, params = {}) {
    const id = ++this.seq;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`timed out waiting for Edge DevTools method ${method}`));
      }, 5000);
      this.pending.set(id, { resolve, reject, timer });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const result = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
    return result.result.value;
  }
  close() { this.ws.close(); }
}

const prelude = String.raw`
window.catsDesktop = Object.freeze({ platform: "windows", nativeClipboard: true });
window.__catsTest = { sent: [], sockets: [], clipWrites: [], notifications: [], opened: [], focused: true };
Object.defineProperty(document, "hasFocus", { value: () => window.__catsTest.focused });
window.focus = () => { window.__catsTest.focused = true; window.__catsTest.windowFocused = true; };
window.open = (...args) => { window.__catsTest.opened.push(args); return null; };
window.catsClipRead = async () => window.__catsTest.clipRead || "native paste";
window.catsClipWrite = async (text) => { window.__catsTest.clipWrites.push(text); };
Object.defineProperty(navigator, "clipboard", { configurable: true, value: {
  readText: async () => "browser paste",
  writeText: async (text) => { window.__catsTest.browserWrites = (window.__catsTest.browserWrites || []).concat(text); },
} });
class CatsTestNotification {
  static permission = "granted";
  static requestPermission = async () => "granted";
  constructor(title, options) {
    this.title = title; this.options = options; this.closed = false;
    window.__catsTest.notifications.push(this);
  }
  close() { this.closed = true; }
}
window.Notification = CatsTestNotification;
class CatsTestWebSocket {
  constructor(url) {
    this.url = url; this.readyState = 0;
    this.number = window.__catsTest.sockets.length + 1;
    window.__catsTest.sockets.push(this);
    window.__catsTest.socket = this;
    setTimeout(() => {
      if (this.number === 1) { this.readyState = 3; if (this.onclose) this.onclose(); }
      else { this.readyState = 1; if (this.onopen) this.onopen(); }
    }, 10);
  }
  send(data) { window.__catsTest.sent.push(JSON.parse(data)); }
  close() { if (this.readyState === 3) return; this.readyState = 3; if (this.onclose) this.onclose(); }
}
window.WebSocket = CatsTestWebSocket;
window.__catsTest.message = (msg) => window.__catsTest.socket.onmessage({ data: JSON.stringify(msg) });
window.__catsTest.key = (init, altGraph) => {
  const event = new KeyboardEvent(init.type || "keydown", { bubbles: true, cancelable: true, ...init });
  if (altGraph) Object.defineProperty(event, "getModifierState", { value: (name) => name === "AltGraph" });
  window.dispatchEvent(event);
  return event.defaultPrevented;
};
`;

async function main() {
  const [index, policy] = await Promise.all([readFile(indexPath, "utf8"), readFile(platformPath, "utf8")]);
  const html = index.replace("</head>", `<script>${policy}</script><script>${prelude}</script></head>`);
  const server = createServer((req, res) => {
    res.writeHead(200, { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" });
    res.end(html);
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const pagePort = server.address().port;
  const debugPort = await reservePort();
  const profile = await mkdtemp(path.join(os.tmpdir(), "cats-edge-phase5-"));
  const child = spawn(edge, [
    "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
    `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`,
    `http://127.0.0.1:${pagePort}/phase5`,
  ], { stdio: ["ignore", "ignore", "pipe"], windowsHide: true });
  let edgeErr = "";
  child.stderr.on("data", (chunk) => { edgeErr += chunk; });

  let cdp;
  try {
    const target = await poll(async () => {
      const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`, { signal: AbortSignal.timeout(1000) });
      if (!response.ok) return null;
      const pages = await response.json();
      return pages.find((item) => item.type === "page" && item.url.includes(`/phase5`));
    }, 10000, "Edge DevTools page target");
    cdp = new CDP(target.webSocketDebuggerUrl);
    await cdp.open();
    await cdp.send("Runtime.enable");
    await poll(() => cdp.eval(`document.readyState === "complete" && !!window.pasteText`), 10000, "cats page initialization");

    const chrome = await cdp.eval(`({
      platform: CatsPlatform.describe(window.catsDesktop, navigator.platform),
      hint: document.querySelector("#palhint .tmk").textContent,
      title: document.querySelector("#palhint").title
    })`);
    assert.equal(chrome.platform.platform, "windows");
    assert.equal(chrome.platform.desktop, true);
    assert.equal(chrome.platform.nativeClipboard, true);
    assert.equal(chrome.hint, "Ctrl+Alt+K");
    assert.match(chrome.title, /Ctrl\+Alt\+K/);

    await poll(() => cdp.eval(`__catsTest.sockets.length >= 2 && __catsTest.sent.some(x => x.t === "init")`),
      3000, "reconnect after transient localhost failure");

    await cdp.eval(`__catsTest.sent = []; __catsTest.key({code:"KeyA",key:"a"});`);
    assert.equal(await cdp.eval(`__catsTest.sent.filter(x => x.t === "key" && x.code === "KeyA").length`), 1);
    await cdp.eval(`
      window.dispatchEvent(new CompositionEvent("compositionstart", {data:"é"}));
      __catsTest.key({code:"KeyE",key:"e",isComposing:true});
      window.dispatchEvent(new CompositionEvent("compositionend", {data:"é"}));
      __catsTest.key({type:"keyup",code:"KeyE",key:"e"});
    `);
    assert.equal(await cdp.eval(`__catsTest.sent.filter(x => x.t === "key" && x.code === "KeyE").length`), 0);
    await cdp.eval(`__catsTest.key({code:"KeyK",key:"k",ctrlKey:true,altKey:true}, true)`);
    assert.equal(await cdp.eval(`__catsTest.sent.filter(x => x.t === "key" && x.code === "KeyK").length`), 1);

    assert.equal(await cdp.eval(`__catsTest.key({code:"KeyK",key:"k",ctrlKey:true,altKey:true})`), true);
    assert.equal(await cdp.eval(`!!document.querySelector("#overlay")`), true);
    await cdp.eval(`__catsTest.key({code:"Escape",key:"Escape"})`);
    await cdp.eval(`__catsTest.key({code:"Equal",key:"+",ctrlKey:true,shiftKey:true})`);
    assert.equal(await cdp.eval(`localStorage.getItem("cats.font_px")`), "15");

    await cdp.eval(`__catsTest.sent=[]; __catsTest.clipRead="paste button"; window.pasteText();`);
    await poll(() => cdp.eval(`__catsTest.sent.some(x => x.t === "paste" && x.data === "paste button")`),
      1000, "native paste bridge");
    await cdp.eval(`window.catsClipRead=async()=>{throw new Error("clipboard busy")}; window.pasteText();`);
    await poll(() => cdp.eval(`[...document.querySelectorAll(".toast")].some(x => x.textContent.includes("paste blocked"))`),
      1000, "clipboard bridge failure toast");

    await cdp.eval(`__catsTest.message({t:"clipboard",data:btoa("osc52")})`);
    await poll(() => cdp.eval(`__catsTest.clipWrites.includes("osc52")`), 1000, "OSC52 native clipboard write");

    const layout = {
      t: "layout", workspaces: [{ id: "w1", name: "phase5", active: true }],
      tabs: [{ num: 1, name: "one", active: true, zoomed: false }],
      panes: [{ pane: 1, pub: "w1:p1", rect: [0, 0, 40, 12], inner: [0, 1, 40, 11], focused: true }],
      borders: [],
    };
    await cdp.eval(`__catsTest.message(${JSON.stringify(layout)}); __catsTest.message({
      t:"pane_frame",pane:1,w:40,h:11,cells:[{s:"x",h:1}],links:["javascript:alert(1)"],def_fg:0,def_bg:0
    })`);
    await poll(() => cdp.eval(`!!document.querySelector(".pane canvas")`), 1000, "visible pane");

    await cdp.eval(`(() => {
      __catsTest.sent=[]; window.catsClipRead=async()=>"context paste";
      const canvas=document.querySelector(".pane canvas"), r=canvas.getBoundingClientRect();
      canvas.dispatchEvent(new MouseEvent("contextmenu",{bubbles:true,cancelable:true,clientX:r.left+3,clientY:r.top+3}));
      [...document.querySelectorAll("#ctxmenu .item")].find(x=>x.textContent==="paste").click();
    })()`);
    await poll(() => cdp.eval(`__catsTest.sent.some(x => x.t === "paste" && x.data === "context paste")`),
      1000, "paste context-menu button");

    await cdp.eval(`(() => {
      __catsTest.sent=[];
      const canvas=document.querySelector(".pane canvas"), r=canvas.getBoundingClientRect();
      canvas.dispatchEvent(new MouseEvent("mousedown",{bubbles:true,cancelable:true,button:0,buttons:1,clientX:r.left+3,clientY:r.top+3}));
      canvas.dispatchEvent(new MouseEvent("mousemove",{bubbles:true,cancelable:true,button:0,buttons:1,clientX:r.left+20,clientY:r.top+3}));
      window.dispatchEvent(new MouseEvent("mouseup",{bubbles:true,cancelable:true,button:0,clientX:r.left+20,clientY:r.top+3}));
    })()`);
    const readID = await poll(async () => cdp.eval(`(__catsTest.sent.find(x => x.t==="cmd" && x.name==="read")||{}).id||""`),
      1000, "selection read command");
    await cdp.eval(`__catsTest.message({t:"cmd_result",id:${JSON.stringify(readID)},ok:true,data:{text:"selected"}})`);
    await poll(() => cdp.eval(`__catsTest.clipWrites.includes("selected")`), 1000, "selection clipboard write");

    await cdp.eval(`
      __catsTest.sent=[];
      document.querySelector('button[title^="copy mode"]').click();
      __catsTest.key({code:"KeyV",key:"v"});
      __catsTest.key({code:"ArrowRight",key:"ArrowRight"});
      __catsTest.key({code:"KeyY",key:"y"});
    `);
    const copyModeReadID = await poll(async () => cdp.eval(`(__catsTest.sent.find(x => x.t==="cmd" && x.name==="read")||{}).id||""`),
      1000, "copy-mode read command");
    await cdp.eval(`__catsTest.message({t:"cmd_result",id:${JSON.stringify(copyModeReadID)},ok:true,data:{text:"copy mode"}})`);
    await poll(() => cdp.eval(`__catsTest.clipWrites.includes("copy mode")`), 1000, "copy-mode clipboard write");

    await cdp.eval(`(() => {
      __catsTest.opened=[];
      const canvas=document.querySelector(".pane canvas"), r=canvas.getBoundingClientRect();
      canvas.dispatchEvent(new MouseEvent("mousedown",{bubbles:true,cancelable:true,button:0,buttons:1,clientX:r.left+3,clientY:r.top+3}));
      window.dispatchEvent(new MouseEvent("mouseup",{bubbles:true,cancelable:true,button:0,clientX:r.left+3,clientY:r.top+3}));
    })()`);
    assert.equal(await cdp.eval(`__catsTest.opened.length`), 0);
    assert.equal(await cdp.eval(`[...document.querySelectorAll(".toast")].some(x => x.textContent.includes("link blocked"))`), true);
    await cdp.eval(`__catsTest.message({t:"pane_frame",pane:1,w:40,h:11,cells:[{s:"x",h:1}],links:["https://example.com/docs"],def_fg:0,def_bg:0})`);
    await cdp.eval(`(() => {
      const canvas=document.querySelector(".pane canvas"), r=canvas.getBoundingClientRect();
      canvas.dispatchEvent(new MouseEvent("mousedown",{bubbles:true,cancelable:true,button:0,buttons:1,clientX:r.left+3,clientY:r.top+3}));
      window.dispatchEvent(new MouseEvent("mouseup",{bubbles:true,cancelable:true,button:0,clientX:r.left+3,clientY:r.top+3}));
    })()`);
    assert.equal(await cdp.eval(`__catsTest.opened.at(-1)[0]`), "https://example.com/docs");
    assert.equal(await cdp.eval(`location.host.startsWith("127.0.0.1:")`), true);

    await cdp.eval(`
      __catsTest.notifications=[]; __catsTest.focused=true;
      __catsTest.message({t:"notify",kind:"attention",message:"visible",body:"pane",pane:1});
    `);
    assert.equal(await cdp.eval(`__catsTest.notifications.length`), 0);
    await cdp.eval(`
      __catsTest.focused=false; Notification.permission="denied";
      __catsTest.message({t:"notify",kind:"attention",message:"hidden fallback",body:"pane",pane:2});
    `);
    assert.equal(await cdp.eval(`__catsTest.notifications.length`), 0);
    assert.equal(await cdp.eval(`[...document.querySelectorAll(".toast")].some(x=>x.textContent.includes("hidden fallback"))`), true);
    await cdp.eval(`Notification.permission="granted"; __catsTest.message({t:"notify",kind:"finished",message:"done",body:"pane",pane:1})`);
    assert.equal(await cdp.eval(`__catsTest.notifications.length`), 1);
    await cdp.eval(`__catsTest.notifications[0].onclick()`);
    assert.equal(await cdp.eval(`__catsTest.windowFocused === true && __catsTest.notifications[0].closed`), true);
    assert.equal(await cdp.eval(`__catsTest.sent.some(x => x.t==="cmd" && x.name==="agent.focus" && x.params.pane===1)`), true);

    console.log("Phase 5 Edge browser regression: PASS");
  } finally {
    if (cdp) {
      try { await cdp.send("Browser.close"); } catch (_) {}
      cdp.close();
    }
    await new Promise((resolve) => server.close(resolve));
    await Promise.race([new Promise((resolve) => child.once("exit", resolve)), sleep(2000)]);
    if (child.exitCode === null) child.kill();
    await rm(profile, { recursive: true, force: true });
  }
  if (child.exitCode && child.exitCode !== 0) throw new Error(`Edge exited ${child.exitCode}: ${edgeErr}`);
}

main().catch((err) => {
  console.error(err.stack || err);
  process.exitCode = 1;
});

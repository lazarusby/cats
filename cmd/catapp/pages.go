//go:build darwin || windows || catapp_headless

package main

import (
	"html"
	"strings"
)

// The launcher's two built-in pages are tiny, self-contained HTML (no external
// assets, no build step) rendered via webview.SetHtml — the same raw-string
// approach as the catway's login page (cmd/catway/auth.go). They share the
// catway's dark palette so the window looks of a piece before the real UI or a
// remote login page loads.

// connectPageHTML is the first-run form for the thin client. Submitting calls
// the Go-bound catsConnect(url) (see runRemote), which persists the URL and
// navigates the same window to the remote catway.
const connectPageHTML = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Cats Mux · connect</title>
<style>
  html,body{margin:0;height:100%;background:#181818;color:#d4d4d4;
    font-family:ui-monospace,"SF Mono",Menlo,Consolas,monospace;
    display:flex;align-items:center;justify-content:center;}
  form{background:#202020;border:1px solid #333;border-radius:8px;padding:28px 26px;
    width:360px;box-shadow:0 4px 20px rgba(0,0,0,.5);}
  h1{font-size:16px;margin:0 0 4px;color:#e8e8e8;}
  p.sub{font-size:12px;color:#888;margin:0 0 18px;}
  label{display:block;font-size:12px;color:#aaa;margin:0 0 6px;}
  input{width:100%;box-sizing:border-box;padding:9px 10px;font-size:14px;
    background:#141414;color:#e8e8e8;border:1px solid #3a3a3a;border-radius:5px;
    font-family:inherit;}
  input:focus{outline:none;border-color:#5b9dff;}
  button{margin-top:16px;width:100%;padding:9px;font-size:14px;cursor:pointer;
    background:#2f68c8;color:#fff;border:none;border-radius:5px;font-family:inherit;}
  button:hover{background:#3a78e0;}
  #connect-error{min-height:16px;color:#ff8a8a;font-size:12px;margin:10px 0 0;}
</style></head><body>
<form onsubmit="submitConnect(event)">
  <h1>Connect to cats</h1>
  <p class="sub">Enter your catway URL — a relay host or a direct LAN/VPN address.</p>
  <label for="url">Catway URL</label>
  <input id="url" name="url" type="url" placeholder="https://home.relay.herdr.dev"
    autofocus autocomplete="url"/>
  <p id="connect-error" role="alert"></p>
  <button type="submit">Connect</button>
</form>
<script>
  function submitConnect(e){
    e.preventDefault();
    var v = document.getElementById('url').value.trim();
    if (v) window.catsConnect(v);
  }
</script>
</body></html>`

// errorPageHTML renders a startup-failure page. title and detail are HTML-escaped
// because detail can be an arbitrary error string (paths, messages).
func errorPageHTML(title, detail string) string {
	return `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Cats Mux · error</title>
<style>
  html,body{margin:0;height:100%;background:#181818;color:#d4d4d4;
    font-family:ui-monospace,"SF Mono",Menlo,Consolas,monospace;
    display:flex;align-items:center;justify-content:center;}
  .card{background:#202020;border:1px solid #3a2a2a;border-radius:8px;
    padding:24px 26px;width:440px;box-shadow:0 4px 20px rgba(0,0,0,.5);}
  h1{font-size:15px;margin:0 0 10px;color:#ff6b6b;}
  pre{font-size:12px;color:#c9c9c9;background:#141414;border:1px solid #333;
    border-radius:5px;padding:12px;white-space:pre-wrap;word-break:break-word;
    margin:0;}
</style></head><body>
<div class="card">
  <h1>` + html.EscapeString(title) + `</h1>
  <pre>` + html.EscapeString(detail) + `</pre>
</div>
</body></html>`
}

func startingPageHTML(stage string) string {
	return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>cats · starting</title><style>` + builtInPageCSS + `</style></head><body>
<div class="card"><h1>Starting cats</h1><p>` + html.EscapeString(stage) + `…</p>
<p class="sub">You can close this window to cancel.</p></div></body></html>`
}

func windowsStartupErrorPageHTML(title, detail string) string {
	return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>cats · startup error</title><style>` + builtInPageCSS + actionButtonCSS + `</style></head><body>
<div class="card"><h1>` + html.EscapeString(title) + `</h1>
<p class="error">` + html.EscapeString(detail) + `</p><div class="actions">
<button onclick="window.catsRetry()">Retry</button>
<button onclick="window.catsRepair()">Repair</button>
<button onclick="window.catsChangeWSL()">Change distribution</button>
<button onclick="window.catsOpenLogs()">Open logs</button>
</div></div></body></html>`
}

func wslSetupPageHTML(distributions []string, detail string) string {
	var options strings.Builder
	for _, distribution := range distributions {
		escaped := html.EscapeString(distribution)
		options.WriteString(`<option value="` + escaped + `">` + escaped + `</option>`)
	}
	return `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>cats · WSL setup</title><style>` + builtInPageCSS + `
label{display:block;margin-top:12px;color:#aaa;font-size:12px}input,select{width:100%;
box-sizing:border-box;padding:8px;margin-top:4px;background:#141414;color:#eee;
border:1px solid #444;border-radius:4px}button{margin-top:16px;padding:9px 16px;
background:#2f68c8;color:#fff;border:0;border-radius:5px}` + actionButtonCSS + `</style></head><body>
<form class="card" onsubmit="event.preventDefault();window.catsSelectWSL(
document.getElementById('distribution').value,document.getElementById('user').value,
document.getElementById('payload').value)"><h1>Connect cats to WSL2</h1>
<p class="error">` + html.EscapeString(detail) + `</p>
<label>Distribution<select id="distribution" required>` + options.String() + `</select></label>
<label>Linux user<input id="user" required autocomplete="username"/></label>
<label>Payload directory<input id="payload" required value="/home/USER/.local/lib/cats/current"/></label>
<button type="submit">Verify and start</button>
<div class="actions"><button type="button" onclick="window.catsRepair()">Repair</button>
<button type="button" onclick="window.catsOpenLogs()">Open logs</button></div>
<p class="sub">CATS will not install or modify a distribution automatically. Run the installer to repair a missing payload.</p>
</form></body></html>`
}

const actionButtonCSS = `.actions{display:flex;flex-wrap:wrap;gap:8px;margin-top:16px}
.actions button{margin:0;padding:8px 11px;background:#2f68c8;color:#fff;border:0;
border-radius:5px;cursor:pointer}.actions button:hover{background:#3a78e0}`

const builtInPageCSS = `html,body{margin:0;height:100%;background:#181818;color:#d4d4d4;
font-family:ui-monospace,"SF Mono",Menlo,Consolas,monospace;display:flex;
align-items:center;justify-content:center}.card{background:#202020;border:1px solid #333;
border-radius:8px;padding:24px 26px;width:440px;box-shadow:0 4px 20px rgba(0,0,0,.5)}
h1{font-size:15px;margin:0 0 10px;color:#e8e8e8}p{font-size:12px;white-space:pre-wrap;
word-break:break-word}.sub{color:#888}.error{color:#ff8a8a}`

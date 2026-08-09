//go:build windows

// Command webview2 is a disposable Phase 1 GUI spike for the pinned webview_go
// dependency. It is not the Windows launcher.
package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func main() {
	runtime.LockOSThread()
	resultPath := filepath.Join(os.TempDir(), "cats-webview2-spike.json")
	server, addr := startServer(resultPath)
	defer server.Close()

	w := webview.New(true)
	defer w.Destroy()
	w.SetTitle("CATS WebView2 Phase 1 spike")
	w.SetSize(900, 680, webview.HintNone)
	_ = w.Bind("catsSpikeReport", func(report string) error {
		return os.WriteFile(resultPath, []byte(report+"\n"), 0o600)
	})
	_ = w.Bind("catsSpikeClose", func() { w.Terminate() })
	w.Navigate("http://" + addr + "/")

	go func() {
		time.Sleep(750 * time.Millisecond)
		w.Dispatch(func() {
			w.SetSize(940, 700, webview.HintNone)
			w.Eval(`window.catsEvalPassed = true; window.catsRender();`)
		})
	}()
	w.Run()
}

func startServer(resultPath string) (*http.Server, string) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "cats_spike", Value: "ok", Path: "/", SameSite: http.SameSiteStrictMode})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, pageHTML(resultPath))
	})
	mux.HandleFunc("/ws", websocketUpgrade)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return server, listener.Addr().String()
}

func websocketUpgrade(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Sec-WebSocket-Key")
	hijacker, ok := w.(http.Hijacker)
	if !ok || key == "" {
		http.Error(w, "websocket unavailable", http.StatusBadRequest)
		return
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	sum := sha1.Sum([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	_ = rw.Flush()
	time.Sleep(2 * time.Second)
}

func pageHTML(resultPath string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>CATS WebView2 spike</title>
<style>body{font:15px system-ui;background:#181818;color:#ddd;margin:32px}button{margin:6px;padding:8px}pre{background:#111;padding:16px}input{padding:8px;width:420px}</style></head><body>
<h1>WebView2 Phase 1 spike</h1><p>Result file: <code>` + html.EscapeString(resultPath) + `</code></p>
<pre id="result"></pre>
<button onclick="notify()">Test notification</button>
<button onclick="window.open('about:blank','_blank')">Test new window</button>
<button onclick="window.catsSpikeClose()">Test teardown</button><br>
<input autofocus placeholder="Type Control/Alt/Shift/AltGr/IME keys here">
<pre id="keys">keyboard events appear here</pre>
<script>
const state={navigate:true,bind:false,dispatch_eval:false,resize:'requested',cookie:document.cookie.includes('cats_spike=ok'),websocket:false,notification:'manual',new_window:'manual',keyboard:'manual'};
window.catsEvalPassed=false;
function render(){state.dispatch_eval=!!window.catsEvalPassed;document.getElementById('result').textContent=JSON.stringify(state,null,2);const first=!state.bind;window.catsSpikeReport(JSON.stringify(state)).then(()=>{if(first){state.bind=true;document.getElementById('result').textContent=JSON.stringify(state,null,2);window.catsSpikeReport(JSON.stringify(state));}});}
window.catsRender=render;
const ws=new WebSocket('ws://'+location.host+'/ws');ws.onopen=()=>{state.websocket=true;render()};ws.onerror=()=>{state.websocket='failed';render()};
window.addEventListener('resize',()=>{state.resize='observed';render()});
document.addEventListener('keydown',e=>{state.keyboard='observed';document.getElementById('keys').textContent=JSON.stringify({key:e.key,code:e.code,ctrl:e.ctrlKey,alt:e.altKey,shift:e.shiftKey,meta:e.metaKey,isComposing:e.isComposing},null,2);render()});
async function notify(){try{const p=await Notification.requestPermission();state.notification=p;if(p==='granted')new Notification('CATS WebView2 spike',{body:'Click and focus behavior must be checked manually.'});}catch(e){state.notification='failed: '+e}render()}
render();
</script></body></html>`
}

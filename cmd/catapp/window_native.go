//go:build darwin || (windows && !catapp_windows_nocgo)

package main

import webview "github.com/webview/webview_go"

type nativeWindow struct {
	w webview.WebView
}

func newDesktopWindow(debug bool) desktopWindow { return &nativeWindow{w: webview.New(debug)} }

func (w *nativeWindow) Run()                           { w.w.Run() }
func (w *nativeWindow) Dispatch(fn func())             { w.w.Dispatch(fn) }
func (w *nativeWindow) Destroy()                       { w.w.Destroy() }
func (w *nativeWindow) SetTitle(title string)          { w.w.SetTitle(title) }
func (w *nativeWindow) Navigate(url string)            { w.w.Navigate(url) }
func (w *nativeWindow) SetHtml(page string)            { w.w.SetHtml(page) }
func (w *nativeWindow) Eval(js string)                 { w.w.Eval(js) }
func (w *nativeWindow) Bind(name string, fn any) error { return w.w.Bind(name, fn) }
func (w *nativeWindow) SetSize(width, height int, hint sizeHint) {
	w.w.SetSize(width, height, nativeSizeHint(hint))
}

func nativeSizeHint(hint sizeHint) webview.Hint {
	switch hint {
	case sizeHintFixed:
		return webview.HintFixed
	case sizeHintMin:
		return webview.HintMin
	case sizeHintMax:
		return webview.HintMax
	default:
		return webview.HintNone
	}
}

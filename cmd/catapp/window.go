//go:build darwin || windows || catapp_headless

package main

// desktopWindow is the launcher-owned subset of webview functionality. Common
// mode flow does not import the cgo-backed webview package, which keeps platform
// details in one adapter and allows headless tests of launcher logic.
type desktopWindow interface {
	Run()
	Dispatch(func())
	Destroy()
	SetTitle(string)
	SetSize(int, int, sizeHint)
	Navigate(string)
	SetHtml(string)
	Init(string)
	Eval(string)
	Bind(string, interface{}) error
}

type sizeHint int

const (
	sizeHintNone sizeHint = iota
	sizeHintFixed
	sizeHintMin
	sizeHintMax
)

//go:build darwin || windows || catapp_headless

package main

import "sync"

// cleanupController owns one teardown callback and guarantees that every quit
// path (window close, menu Quit, or platform signal) invokes it at most once.
// Keeping the state in a type makes the ordering/idempotence contract testable
// without resetting package-global sync.Once values.
type cleanupController struct {
	mu   sync.Mutex
	once sync.Once
	fn   func()
}

func (c *cleanupController) register(fn func()) {
	c.mu.Lock()
	c.fn = fn
	c.mu.Unlock()
}

func (c *cleanupController) run() {
	c.once.Do(func() {
		c.mu.Lock()
		fn := c.fn
		c.mu.Unlock()
		if fn != nil {
			fn()
		}
	})
}

var appCleanup cleanupController

func registerCleanup(fn func()) { appCleanup.register(fn) }
func runCleanup()               { appCleanup.run() }

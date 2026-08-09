//go:build windows

package main

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
)

type navigationAction uint8

const (
	navigationAllow navigationAction = iota
	navigationCancel
	navigationExternal
)

type windowsNavigationPolicy struct {
	mu      sync.RWMutex
	trusted string
}

func (p *windowsNavigationPolicy) trust(rawURL string) error {
	origin, err := webOrigin(rawURL)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.trusted = origin
	p.mu.Unlock()
	return nil
}

func (p *windowsNavigationPolicy) decide(rawURL string, newWindow bool) navigationAction {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return navigationCancel
	}
	scheme := strings.ToLower(u.Scheme)
	p.mu.RLock()
	trusted := p.trusted
	p.mu.RUnlock()

	// NavigateToString uses an internal document before the launcher has chosen
	// a backend origin. These schemes are never allowed once a trusted network
	// origin is active.
	if trusted == "" && (scheme == "about" || scheme == "data") {
		return navigationAllow
	}
	if scheme != "http" && scheme != "https" {
		return navigationCancel
	}
	if newWindow {
		return navigationExternal
	}
	origin, err := webOrigin(rawURL)
	if err != nil {
		return navigationCancel
	}
	if origin == trusted {
		return navigationAllow
	}
	return navigationExternal
}

func webOrigin(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return "", fmt.Errorf("invalid URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("URL must use http or https and include a host")
	}
	if u.User != nil {
		return "", fmt.Errorf("URL must not contain user information")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		// Paths are part of navigation, but query/fragment do not participate in
		// origin identity and are therefore accepted below.
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	return u.Scheme + "://" + host + ":" + port, nil
}

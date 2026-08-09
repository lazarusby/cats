//go:build darwin

package main

import "os"

func replaceAppConfigFile(staged, destination string) error {
	return os.Rename(staged, destination)
}

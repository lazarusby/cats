//go:build windows || catapp_headless

package main

import (
	"errors"
	"unicode/utf16"
)

const maxClipboardUTF16Units = 8 << 20 // 16 MiB of CF_UNICODETEXT

func encodeClipboardUTF16(text string) ([]uint16, error) {
	for _, r := range text {
		if r == 0 {
			return nil, errors.New("clipboard text contains NUL")
		}
	}
	encoded := utf16.Encode([]rune(text))
	if len(encoded) > maxClipboardUTF16Units {
		return nil, errors.New("clipboard text exceeds 16 MiB")
	}
	return append(encoded, 0), nil
}

func decodeClipboardUTF16(encoded []uint16) (string, error) {
	end := -1
	for index, unit := range encoded {
		if unit == 0 {
			end = index
			break
		}
	}
	if end < 0 {
		return "", errors.New("clipboard text is not NUL-terminated")
	}
	encoded = encoded[:end]
	if len(encoded) > maxClipboardUTF16Units {
		return "", errors.New("clipboard text exceeds 16 MiB")
	}
	for index := 0; index < len(encoded); index++ {
		unit := encoded[index]
		switch {
		case unit >= 0xd800 && unit <= 0xdbff:
			if index+1 >= len(encoded) || encoded[index+1] < 0xdc00 || encoded[index+1] > 0xdfff {
				return "", errors.New("clipboard text contains an unpaired UTF-16 surrogate")
			}
			index++
		case unit >= 0xdc00 && unit <= 0xdfff:
			return "", errors.New("clipboard text contains an unpaired UTF-16 surrogate")
		}
	}
	return string(utf16.Decode(encoded)), nil
}

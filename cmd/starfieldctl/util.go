package main

import (
	"bytes"
	"os"
	"unicode"
)

func isReadable(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func snakeCase(str string) string {
	var buf bytes.Buffer
	for i, rune := range str {
		if unicode.IsUpper(rune) {
			if i > 0 {
				buf.WriteByte('_')
			}
			buf.WriteRune(unicode.ToLower(rune))
		} else {
			buf.WriteRune(rune)
		}
	}
	return buf.String()
}

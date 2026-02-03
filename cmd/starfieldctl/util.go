package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"unicode"

	"gopkg.in/yaml.v2"
)

func getSchemaDir(sqlcFile string) (string, error) {
	b, err := os.ReadFile(filepath.Clean(sqlcFile))
	if err != nil {
		return "", err
	}
	var config Config
	if err := yaml.Unmarshal(b, &config); err != nil {
		return "", err
	}

	if len(config.SQL) != 1 {
		return "", fmt.Errorf("unexpected number of sql blocks in sqlc.yml")
	}

	return config.SQL[0].Schema, nil
}

func exists(path string) bool {
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

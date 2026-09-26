package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
)

func resolveSchemaDir(sqlcFile string, target string) (string, error) {
	config, err := readConfig(sqlcFile)
	if err != nil {
		return "", err
	}
	return getSchemaDir(config, target)
}

func readConfig(sqlcFile string) (*Config, error) {
	b, err := os.ReadFile(filepath.Clean(sqlcFile))
	if err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal(b, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func readModulePath(goModFile string) (string, error) {
	contents, err := os.ReadFile(filepath.Clean(goModFile))
	if err != nil {
		return "", err
	}

	for line := range strings.SplitSeq(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`"), nil
		}
	}

	return "", fmt.Errorf("no module directive in %s", goModFile)
}

func getSchemaDir(config *Config, target string) (string, error) {
	if len(config.SQL) == 0 {
		return "", errors.New("no sql blocks")
	}

	if target == "" {
		if len(config.SQL) == 1 {
			return config.SQL[0].Schema, nil
		}
		return "", fmt.Errorf("multiple sql blocks: select one with --target %s", targetHint(config))
	}

	for _, entry := range config.SQL {
		if entry.Name != "" && entry.Name == target {
			return entry.Schema, nil
		}
	}

	return "", fmt.Errorf("no sql block named %s %s", target, targetHint(config))
}

func targetHint(config *Config) string {
	names := lo.FilterMap(config.SQL, func(entry Entry, _ int) (string, bool) {
		return entry.Name, entry.Name != ""
	})

	if len(names) == 0 {
		return "(no sql blocks are named: add a name to one)"
	}

	return "(available: " + strings.Join(names, ", ") + ")"
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

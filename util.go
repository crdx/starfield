package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/samber/lo"
)

func PathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func MakeOutputDir(path string) error {
	stat, err := os.Stat(path)

	// If it doesn't exist try to create an empty directory.
	if os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}

	// If it does exist but it's an empty directory then that's okay too.
	if stat.IsDir() && IsDirEmpty(path) {
		return nil
	}

	return fmt.Errorf("file or directory %q already exists (and is not empty)", path)
}

// IsDirEmpty returns whether the given directory is empty.
func IsDirEmpty(path string) bool {
	file := lo.Must(os.Open(path))
	defer file.Close()
	_, err := file.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxFingerprintFileBytes = 8 << 20

// fingerprintTree deterministically hashes a directory tree.
// Symlink leaves are recorded by link target (not followed). Regular files are
// read with a size bound; each file handle is closed before the next walk step.
func fingerprintTree(root string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(h, rel); err != nil {
			return err
		}
		if _, err := io.WriteString(h, "\n"); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", rel, err)
			}
			if _, err := io.WriteString(h, "symlink->"+target+"\n"); err != nil {
				return err
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", rel, err)
		}
		_, copyErr := io.Copy(h, io.LimitReader(f, maxFingerprintFileBytes))
		closeErr := f.Close()
		if copyErr != nil {
			return fmt.Errorf("read %s: %w", rel, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", rel, closeErr)
		}
		if _, err := io.WriteString(h, fmt.Sprintf("|%d|%o\n", info.Size(), info.Mode().Perm())); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

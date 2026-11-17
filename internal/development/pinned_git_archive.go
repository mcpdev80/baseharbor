package development

import (
	"archive/tar"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxPinnedGitArchiveBytes = 128 << 20
	maxPinnedGitArchiveFiles = 10000
)

// StagePinnedGitTree creates an isolated Core-side source tree from one Git
// commit in a previously registered checkout. The caller owns the returned
// directory and must remove it if it is not adopted into an authorized
// deployment transaction. This function neither fetches URLs nor mutates
// deployment records.
func StagePinnedGitTree(ctx context.Context, checkout, revision, parent string) (staged string, err error) {
	if len(revision) != 40 || revision != strings.ToLower(revision) {
		return "", fmt.Errorf("staged Git revision must be a full lowercase commit SHA")
	}
	if _, err := hex.DecodeString(revision); err != nil {
		return "", fmt.Errorf("invalid staged Git commit SHA: %w", err)
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	staged, err = os.MkdirTemp(parent, "baseharbor-git-candidate-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staged)
			staged = ""
		}
	}()
	executionCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(executionCtx, "git", "-C", checkout, "archive", "--format=tar", revision)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start native Git archive: %w", err)
	}
	extractErr := extractPinnedGitArchive(reader, staged)
	if extractErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if extractErr != nil {
		return "", extractErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("native Git archive failed: %w", waitErr)
	}
	return staged, nil
}

func extractPinnedGitArchive(input io.Reader, destination string) error {
	archive := tar.NewReader(io.LimitReader(input, maxPinnedGitArchiveBytes+1))
	var totalBytes int64
	for count := 0; ; count++ {
		if count >= maxPinnedGitArchiveFiles {
			return fmt.Errorf("Git candidate exceeds the maximum number of files")
		}
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Git candidate archive: %w", err)
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if !filepath.IsLocal(name) || name == "." || strings.HasPrefix(name, ".git"+string(filepath.Separator)) || name == ".git" {
			return fmt.Errorf("Git candidate archive contains an unsafe path")
		}
		// Native git archive emits a global PAX header carrying the commit
		// comment. It is metadata only, never a file to materialize.
		// Refuse any PAX record capable of rewriting paths or links.
		if header.Typeflag == tar.TypeXGlobalHeader {
			for key := range header.PAXRecords {
				if key != "comment" {
					return fmt.Errorf("Git candidate contains unsupported global PAX metadata %q", key)
				}
			}
			continue
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxPinnedGitArchiveBytes-totalBytes {
				return fmt.Errorf("Git candidate exceeds the maximum archive size")
			}
			totalBytes += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			mode := os.FileMode(0600)
			if header.Mode&0111 != 0 {
				mode = 0700
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, archive, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("Git candidate contains an unsupported symlink, hardlink or special entry")
		}
	}
}

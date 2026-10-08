package coreupdate

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

// StreamRecoveryPoint stores a provider-native backup without buffering a
// potentially multi-gigabyte PostgreSQL stream in process memory. The caller
// must supply a database-consistent source; volume exports from live writers
// are not valid inputs. A completed file and checksum are immutable on replay.
type StreamRecoveryPoint struct {
    Directory string
    Name string
}

func (p StreamRecoveryPoint) paths() (string, string, error) {
    if p.Directory == "" || p.Name == "" || p.Name == "." || p.Name == ".." ||
       strings.ContainsAny(p.Name, "/\\") || strings.HasPrefix(p.Name, ".") {
        return "", "", errors.New("invalid provider recovery point identity")
    }
    path := filepath.Join(p.Directory, p.Name+".backup")
    return path, path+".sha256", nil
}

func (p StreamRecoveryPoint) Verify() error {
    path, checksumPath, err := p.paths()
    if err != nil { return err }
    dirInfo, err := os.Lstat(p.Directory)
    if err != nil { return err }
    if !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
        return errors.New("provider recovery directory is not private")
    }
    for _, path := range []string{path, checksumPath} {
        info, err := os.Lstat(path)
        if err != nil { return err }
        if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
            return errors.New("provider recovery artifact is not owner-only regular file")
        }
    }
    fd, err := os.Open(path)
    if err != nil { return err }
    defer fd.Close()
    hash := sha256.New()
    size, err := io.Copy(hash, fd)
    if err != nil { return err }
    expected, err := os.ReadFile(checksumPath)
    if err != nil { return err }
    if size == 0 || strings.TrimSpace(string(expected)) != hex.EncodeToString(hash.Sum(nil)) {
        return errors.New("provider recovery stream checksum mismatch")
    }
    return nil
}

func (p StreamRecoveryPoint) Capture(ctx context.Context, source func(context.Context, io.Writer) error) error {
    path, checksumPath, err := p.paths()
    if err != nil { return err }
    if source == nil { return errors.New("provider-native consistent backup source required") }
    if err := ctx.Err(); err != nil { return err }
    if err := os.MkdirAll(p.Directory, 0700); err != nil { return err }
    info, err := os.Lstat(p.Directory)
    if err != nil { return err }
    if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
        return errors.New("native provider backup directory is not owner-only or is a symlink")
    }
    if _, err := os.Lstat(path); err == nil {
        return p.Verify()
    } else if !errors.Is(err, os.ErrNotExist) { return err }
    if _, err := os.Lstat(checksumPath); err == nil {
        return errors.New("unpaired recovery checksum exists; refusing to replace recovery evidence")
    } else if !errors.Is(err, os.ErrNotExist) { return err }
    temp, err := os.CreateTemp(p.Directory, ".provider-backup-")
    if err != nil { return err }
    tempPath := temp.Name()
    defer os.Remove(tempPath)
    if err := temp.Chmod(0600); err != nil { temp.Close(); return err }
    h := sha256.New()
    writer := io.MultiWriter(temp, h)
    sourceErr := source(ctx, writer)
    if sourceErr == nil { sourceErr = ctx.Err() }
    if sourceErr == nil { sourceErr = temp.Sync() }
    stat, statErr := temp.Stat()
    closeErr := temp.Close()
    if sourceErr != nil { return fmt.Errorf("native provider backup interrupted: %w", sourceErr) }
    if statErr != nil { return statErr }
    if closeErr != nil { return closeErr }
    if stat.Size() == 0 { return errors.New("empty native provider recovery point") }
    if err := os.Link(tempPath, path); err != nil {
        return fmt.Errorf("publish immutable provider backup: %w", err)
    }
    // The checksum is published last. An interrupted publication stays
    // intentionally unrecoverable until an operator resolves the partial state.
    checksumFile, err := os.OpenFile(checksumPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if err != nil { return err }
    if _, err = checksumFile.Write([]byte(hex.EncodeToString(h.Sum(nil))+"\n")); err == nil {
        err = checksumFile.Sync()
    }
    closeErr = checksumFile.Close()
    if err != nil { return err }
    if closeErr != nil { return closeErr }
    dir, err := os.Open(p.Directory)
    if err != nil { return err }
    defer dir.Close()
    if err := dir.Sync(); err != nil { return err }
    return p.Verify()
}

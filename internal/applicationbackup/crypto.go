package applicationbackup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	streamMagic     = "BAHABK01"
	streamChunkSize = 1 << 20
	maxCipherChunk  = streamChunkSize + 64
)

func LoadBackupKey(path string, create bool) ([]byte, bool, error) {
	if strings.TrimSpace(path) == "" {
		return nil, false, errors.New("backup key file path is required")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil && filepath.Dir(path) != "." {
			return nil, false, fmt.Errorf("create backup key directory: %w", err)
		}
		key := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, false, fmt.Errorf("generate backup key: %w", err)
		}
		encoded := base64.RawStdEncoding.EncodeToString(key) + "\n"
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, false, fmt.Errorf("create backup key file: %w", err)
		}
		if _, err := io.WriteString(f, encoded); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return nil, false, fmt.Errorf("write backup key file: %w", err)
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return nil, false, err
		}
		return key, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect backup key file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, errors.New("backup key must be a regular file, not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("backup key file is accessible by group or others (%o)", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read backup key file: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != 32 {
		return nil, false, errors.New("backup key file must contain one Base64-encoded 256-bit key")
	}
	return key, false, nil
}

type EncryptWriter struct {
	w       io.Writer
	aead    cipher.AEAD
	prefix  [4]byte
	counter uint64
	buf     []byte
	closed  bool
}

func NewEncryptWriter(w io.Writer, key []byte) (*EncryptWriter, error) {
	if len(key) != 32 {
		return nil, errors.New("backup encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	e := &EncryptWriter{w: w, aead: aead, buf: make([]byte, 0, streamChunkSize)}
	if _, err := io.ReadFull(rand.Reader, e.prefix[:]); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, streamMagic); err != nil {
		return nil, err
	}
	if _, err := w.Write(e.prefix[:]); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *EncryptWriter) Write(p []byte) (int, error) {
	if e.closed {
		return 0, errors.New("encrypted backup writer is closed")
	}
	written := 0
	for len(p) > 0 {
		n := streamChunkSize - len(e.buf)
		if n > len(p) {
			n = len(p)
		}
		e.buf = append(e.buf, p[:n]...)
		p = p[n:]
		written += n
		if len(e.buf) == streamChunkSize {
			if err := e.flush(e.buf); err != nil {
				return written, err
			}
			e.buf = e.buf[:0]
		}
	}
	return written, nil
}

func (e *EncryptWriter) Close() error {
	if e.closed {
		return nil
	}
	if len(e.buf) > 0 {
		if err := e.flush(e.buf); err != nil {
			return err
		}
	}
	// Authenticated zero-length frame is an explicit EOF marker. A truncated
	// backup therefore cannot be mistaken for a complete stream.
	if err := e.flush(nil); err != nil {
		return err
	}
	e.closed = true
	return nil
}

func (e *EncryptWriter) flush(plain []byte) error {
	nonce := make([]byte, e.aead.NonceSize())
	copy(nonce[:4], e.prefix[:])
	binary.BigEndian.PutUint64(nonce[4:], e.counter)
	var ad [12]byte
	binary.BigEndian.PutUint64(ad[:8], e.counter)
	binary.BigEndian.PutUint32(ad[8:], uint32(len(plain)))
	ciphertext := e.aead.Seal(nil, nonce, plain, ad[:])
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(ciphertext)))
	if _, err := e.w.Write(header[:]); err != nil {
		return err
	}
	if _, err := e.w.Write(ciphertext); err != nil {
		return err
	}
	e.counter++
	return nil
}

type DecryptReader struct {
	r       io.Reader
	aead    cipher.AEAD
	prefix  [4]byte
	counter uint64
	buf     []byte
	eof     bool
}

func NewDecryptReader(r io.Reader, key []byte) (*DecryptReader, error) {
	if len(key) != 32 {
		return nil, errors.New("backup decryption key must be 32 bytes")
	}
	magic := make([]byte, len(streamMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != streamMagic {
		return nil, errors.New("invalid BaseHarbor backup header")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	d := &DecryptReader{r: r, aead: aead}
	if _, err := io.ReadFull(r, d.prefix[:]); err != nil {
		return nil, errors.New("truncated BaseHarbor backup header")
	}
	return d, nil
}

func (d *DecryptReader) Read(p []byte) (int, error) {
	for len(d.buf) == 0 && !d.eof {
		if err := d.next(); err != nil {
			return 0, err
		}
	}
	if len(d.buf) == 0 && d.eof {
		return 0, io.EOF
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func (d *DecryptReader) next() error {
	var header [4]byte
	if _, err := io.ReadFull(d.r, header[:]); err != nil {
		return errors.New("encrypted backup is truncated before authenticated EOF")
	}
	n := binary.BigEndian.Uint32(header[:])
	if n < uint32(d.aead.Overhead()) || n > maxCipherChunk {
		return errors.New("encrypted backup contains an invalid frame size")
	}
	ciphertext := make([]byte, n)
	if _, err := io.ReadFull(d.r, ciphertext); err != nil {
		return errors.New("encrypted backup contains a truncated frame")
	}
	nonce := make([]byte, d.aead.NonceSize())
	copy(nonce[:4], d.prefix[:])
	binary.BigEndian.PutUint64(nonce[4:], d.counter)
	plainLen := int(n) - d.aead.Overhead()
	var ad [12]byte
	binary.BigEndian.PutUint64(ad[:8], d.counter)
	binary.BigEndian.PutUint32(ad[8:], uint32(plainLen))
	plain, err := d.aead.Open(nil, nonce, ciphertext, ad[:])
	if err != nil {
		return errors.New("encrypted backup authentication failed")
	}
	d.counter++
	if len(plain) == 0 {
		var extra [1]byte
		n, err := d.r.Read(extra[:])
		if n != 0 || (err != nil && !errors.Is(err, io.EOF)) {
			return errors.New("encrypted backup contains trailing data")
		}
		d.eof = true
		return nil
	}
	d.buf = plain
	return nil
}

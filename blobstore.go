package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Attachment bytes live on the host filesystem, not in poche: poche holds
// documents in memory (6 MB of base64 took the store from 4 MB to 315 MB RSS)
// and its blob store has no HTTP upload route. Metadata stays in poche, so the
// mail model is still one store; this directory only holds opaque bytes.
//
// Back it up alongside poche.data — a poche-only backup restores messages
// whose attachments have vanished.

func blobDir() string {
	return envOr("BLOB_DIR", "/var/lib/poche-resend-webmail/blobs")
}

// blobMinFree keeps a floor of free disk so filling the mailbox cannot take
// the host down with it.
func blobMinFree() int64 {
	return envInt64("BLOB_MIN_FREE_BYTES", 512*1024*1024)
}

func freeBytes(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// putBlob writes bytes under a fresh random id. Ids are random rather than
// content-addressed on purpose: with dedup, deleting one message could pull
// the bytes out from under another that shares the same file.
func putBlob(data []byte) (string, error) {
	dir := blobDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("blob dir: %w", err)
	}
	if free, err := freeBytes(dir); err == nil {
		if free-int64(len(data)) < blobMinFree() {
			return "", fmt.Errorf("refusing to store: only %s free on the blob volume", humanBytes(free))
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	shard := filepath.Join(dir, id[:2])
	if err := os.MkdirAll(shard, 0o750); err != nil {
		return "", err
	}
	final := filepath.Join(shard, id)
	// Write to a temp name first so a crash mid-write cannot leave a
	// truncated file that would later be served as a valid attachment.
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return id, nil
}

func blobPath(id string) (string, bool) {
	if len(id) < 3 || !isHex(id) {
		return "", false
	}
	p := filepath.Join(blobDir(), id[:2], id)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	return "", false
}

func deleteBlob(id string) {
	if p, ok := blobPath(id); ok {
		if err := os.Remove(p); err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"blob_delete_err\",\"id\":%q,\"err\":%q}\n", id, err.Error())
		}
	}
}

// isHex guards the path join: an id is used to build a filesystem path, so
// anything but hex could walk out of the blob directory.
func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return len(s) > 0
}

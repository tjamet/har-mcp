package har

import (
	"crypto/sha256"
	"fmt"
)

// BodyStore holds decoded bodies keyed by the full SHA-256 of their bytes, so
// identical bodies are stored once and referenced by hash. References look
// like "body:<64 hex sha256>". The digest is deliberately not truncated:
// bodies come from untrusted HAR input, and a 64-bit prefix would be
// collision-forgeable at ~2^32 work, letting one body's bytes serve under
// another's reference.
type BodyStore struct {
	// ponytail: plain map — the server is a single-threaded stdio process;
	// add a mutex if an HTTP mode or any concurrency ever lands.
	entries map[string]storedBody
}

// storedBody is one content-addressed body with its decoded size and mime
// type. Size is stored alongside the bytes so Get can report it without
// re-measuring, and mime records what the first storing caller declared.
type storedBody struct {
	data []byte
	size int64
	mime string
}

// NewBodyStore creates an empty body store.
func NewBodyStore() *BodyStore {
	return &BodyStore{entries: make(map[string]storedBody)}
}

// Add stores body under a content-hash reference and returns the reference.
// Empty bodies are not stored and yield an empty reference. Storing an
// already-known body is a no-op: dedup is free because the key is the hash.
func (s *BodyStore) Add(body []byte, mimeType string) string {
	if len(body) == 0 {
		return ""
	}
	hash := sha256.Sum256(body)
	ref := fmt.Sprintf("body:%x", hash)
	if _, ok := s.entries[ref]; !ok {
		s.entries[ref] = storedBody{data: body, size: int64(len(body)), mime: mimeType}
	}
	return ref
}

// Ref returns the content-hash reference for body if that body was stored,
// and "" otherwise. It is the lookup-only counterpart of Add: read paths use
// it so the store is populated exclusively at parse time and never mutated
// by a query.
func (s *BodyStore) Ref(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	hash := sha256.Sum256(body)
	ref := fmt.Sprintf("body:%x", hash)
	if _, ok := s.entries[ref]; ok {
		return ref
	}
	return ""
}

// Get returns the stored body, its decoded size and mime type for a
// reference. The bool reports whether the reference is known.
func (s *BodyStore) Get(ref string) ([]byte, int64, string, bool) {
	entry, ok := s.entries[ref]
	if !ok {
		return nil, 0, "", false
	}
	return entry.data, entry.size, entry.mime, true
}

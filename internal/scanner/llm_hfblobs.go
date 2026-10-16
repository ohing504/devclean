package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/ohing504/devclean/internal/model"
)

// Hugging Face shared blob store, read with huggingface_hub's rules
// (utils/_shared_blobs.py): see docs/decisions/hf-shared-blobs.md.

const (
	hfStoreMarker        = ".huggingface-shared-blobs"
	hfStoreLayoutVersion = "1\n"
	hfManifestSuffix     = ".refs"
	hfLockSuffix         = ".lock"

	hfOrphanRec     = "shared blobs no cached repo references — left when a repo folder was deleted directly"
	hfOrphanDisplay = "delete unreferenced Hugging Face shared blobs (as 'hf cache prune')"
)

var (
	hfHashRe    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hfRepoDirRe = regexp.MustCompile(`^(datasets|spaces|models|kernels)--.+`)
	hfPrefixRe  = regexp.MustCompile(`^[0-9a-f]{2}$`)
)

// hfStore is the shared blob store of one hub cache directory.
type hfStore struct {
	hub string // hub cache directory
	dir string // <hub>/blobs
}

// openHFStore returns hub's shared blob store, or false when hub has none.
func openHFStore(hub string) (hfStore, bool) {
	s := hfStore{hub: hub, dir: filepath.Join(hub, "blobs")}
	if info, err := os.Lstat(s.dir); err != nil || !info.IsDir() {
		return s, false
	}
	marker := filepath.Join(s.dir, hfStoreMarker)
	if !isRegularFile(marker) {
		return s, false
	}
	data, err := os.ReadFile(marker)
	return s, err == nil && string(data) == hfStoreLayoutVersion
}

// payloads returns every payload file in the store.
func (s hfStore) payloads() []string {
	var out []string
	for _, prefix := range subdirs(s.dir) {
		entries, err := os.ReadDir(prefix)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Type().IsRegular() && isPayloadName(filepath.Base(prefix), e.Name()) {
				out = append(out, filepath.Join(prefix, e.Name()))
			}
		}
	}
	return out
}

// references returns payload's manifest entries whose symlink still points
// at it; ok is false when the manifest is unreadable.
func (s hfStore) references(payload string) (refs []string, ok bool) {
	manifest := payload + hfManifestSuffix
	if !isRegularFile(manifest) {
		return nil, false
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		return nil, false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		rel := filepath.FromSlash(line)
		if line == "" || filepath.IsAbs(rel) || filepath.Clean(rel) != rel {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) != 3 || !hfRepoDirRe.MatchString(parts[0]) || parts[1] != "blobs" {
			continue
		}
		if s.target(filepath.Join(s.hub, rel)) == payload {
			refs = append(refs, rel)
		}
	}
	return refs, true
}

// target returns the payload link points at, or "" if none.
func (s hfStore) target(link string) string {
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return ""
	}
	dest, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(filepath.Dir(link), dest)
	}
	rel, err := filepath.Rel(s.dir, filepath.Clean(dest))
	if err != nil {
		return ""
	}
	prefix, hash, found := strings.Cut(rel, string(filepath.Separator))
	if !found || !isPayloadName(prefix, hash) {
		return ""
	}
	return filepath.Join(s.dir, prefix, hash)
}

// isPayloadName reports whether blobs/<prefix>/<hash> names a store payload.
func isPayloadName(prefix, hash string) bool {
	return hfPrefixRe.MatchString(prefix) && hfHashRe.MatchString(hash) && strings.HasPrefix(hash, prefix)
}

// sweepAll sweeps each payload, collecting the errors.
func (s hfStore) sweepAll(payloads []string, remove func(string) error) error {
	var errs []error
	for _, p := range payloads {
		errs = append(errs, s.sweep(p, remove))
	}
	return errors.Join(errs...)
}

// sweep deletes payload and its manifest once nothing references it.
func (s hfStore) sweep(payload string, remove func(string) error) error {
	unlock, ok := lockHFPayload(payload)
	if !ok {
		return nil
	}
	defer unlock()
	refs, ok := s.references(payload)
	if !ok || len(refs) > 0 || !isRegularFile(payload) {
		return nil
	}
	if err := remove(payload); err != nil {
		return err
	}
	return remove(payload + hfManifestSuffix)
}

// lockHFPayload takes huggingface_hub's flock on <payload>.lock without
// waiting; false means it is held.
func lockHFPayload(payload string) (unlock func(), ok bool) {
	f, err := os.OpenFile(payload+hfLockSuffix, os.O_WRONLY|os.O_CREATE|syscall.O_NOFOLLOW, 0o666)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, true
}

// addHFBlobs adds the store's payloads to the sized model results and
// appends one result for the payloads no repo references.
func addHFBlobs(results []model.ScanResult, s hfStore) []model.ScanResult {
	models := make(map[string]*model.ScanResult) // repo dir name → its result
	for i := range results {
		if filepath.Dir(results[i].Path) == s.hub {
			models[filepath.Base(results[i].Path)] = &results[i]
		}
	}
	held := make(map[*model.ScanResult][]string) // model → payloads it references

	var orphans []string
	var orphanSize int64
	for _, p := range s.payloads() {
		refs, ok := s.references(p)
		if !ok {
			continue
		}
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		sys, isUnix := info.Sys().(*syscall.Stat_t)
		if !isUnix {
			continue
		}
		blocks := int64(sys.Blocks) * 512
		key := model.InodeKey{Dev: uint64(sys.Dev), Ino: uint64(sys.Ino)} //nolint:unconvert // Dev is int32 on darwin, uint64 on linux
		if len(refs) == 0 {
			orphans = append(orphans, p)
			orphanSize += blocks
			continue
		}
		seen := make(map[string]uint32)
		for _, ref := range refs {
			seen[strings.SplitN(ref, string(filepath.Separator), 2)[0]]++
		}
		for repo, n := range seen {
			r := models[repo]
			if r == nil {
				continue
			}
			held[r] = append(held[r], p)
			if len(seen) == 1 {
				r.Size += blocks
				continue
			}
			if r.BlobShares == nil {
				r.BlobShares = make(map[model.InodeKey]model.CloneShare)
			}
			r.BlobShares[key] = model.CloneShare{Refcnt: uint32(len(refs)), Seen: n, Blocks: blocks}
		}
	}

	for r, payloads := range held {
		r.Release = func(remove func(string) error) error { return s.sweepAll(payloads, remove) }
	}

	if len(orphans) == 0 {
		return results
	}
	mod := ModTime(s.dir)
	return append(results, model.ScanResult{
		Path:           s.dir,
		Ecosystem:      model.EcoLLM,
		Category:       model.CatCache,
		Size:           orphanSize,
		LastMod:        mod,
		Safety:         model.SafetySafe,
		ProjectRoot:    s.hub,
		Label:          "Hugging Face unreferenced shared blobs",
		Recommendation: hfOrphanRec,
		LastUsedAt:     mod,
		Delete: &model.DeleteMethod{
			Kind:    model.DeleteKindAPI,
			Display: hfOrphanDisplay,
			Run:     func(context.Context) error { return s.sweepAll(orphans, os.Remove) },
		},
	})
}

// isRegularFile reports whether path is a regular file (not via a symlink).
func isRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

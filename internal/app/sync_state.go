package app

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SyncState is the derived, read-only relationship between the sandbox
// outbound snapshot and the host run worktree. It is separate from the
// operational RunStatus and from the persisted Run.Synced safety flag.
type SyncState string

const (
	SyncStateNoChanges SyncState = "no_changes"
	SyncStateUpToDate  SyncState = "up_to_date"
	SyncStateNeedsSync SyncState = "needs_sync"
	SyncStateBlocked   SyncState = "blocked"
	SyncStateUnknown   SyncState = "unknown"
)

func (s SyncState) Label() string {
	switch s {
	case SyncStateNoChanges:
		return "No changes"
	case SyncStateUpToDate:
		return "Up to date"
	case SyncStateNeedsSync:
		return "Sync needed"
	case SyncStateBlocked:
		return "Sync blocked"
	case SyncStateUnknown:
		return "Unable to check"
	default:
		return "Unable to check"
	}
}

const syncProbeTTL = 5 * time.Second

var (
	emptyDiffSHA256 = hashBytes(nil)
	syncProbeInitMu sync.Mutex
)

type syncProbeStore struct {
	mu      sync.Mutex
	entries map[string]syncProbeCacheEntry
}

type syncProbeCacheEntry struct {
	sandbox  string
	probedAt time.Time
	snapshot syncSnapshot
}

// syncSnapshot is the canonical outbound-state representation produced by the
// sandbox probe and by the successful-sync export path.
type syncSnapshot struct {
	Branch     []byte              `json:"branch"`
	Head       string              `json:"head"`
	DiffSHA256 string              `json:"diff_sha256"`
	Untracked  []syncSnapshotEntry `json:"untracked"`
}

type syncSnapshotEntry struct {
	Path   []byte `json:"path"`
	Kind   string `json:"kind"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256,omitempty"`
	Target []byte `json:"target,omitempty"`
}

func (s syncSnapshot) Fingerprint() string {
	entries := append([]syncSnapshotEntry(nil), s.Untracked...)
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].Path, entries[j].Path) < 0
	})
	h := sha256.New()
	writeSnapshotField(h, []byte("agentbox-sync-v1"))
	writeSnapshotField(h, s.Branch)
	writeSnapshotField(h, []byte(s.Head))
	writeSnapshotField(h, []byte(s.DiffSHA256))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(entries)))
	_, _ = h.Write(count[:])
	for _, entry := range entries {
		writeSnapshotField(h, entry.Path)
		writeSnapshotField(h, []byte(entry.Kind))
		writeSnapshotField(h, []byte(entry.Mode))
		writeSnapshotField(h, entry.Target)
		writeSnapshotField(h, []byte(entry.SHA256))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeSnapshotField(w io.Writer, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = w.Write(value)
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func (s syncSnapshot) clean(baseline string) bool {
	return s.Head == baseline && s.DiffSHA256 == emptyDiffSHA256 && len(s.Untracked) == 0
}

// sandboxSyncProbeScript prints one JSON object describing the outbound
// snapshot in /workspace/project. It deliberately hashes only the tracked diff
// and non-ignored untracked entries; clean tracked files are never read.
const sandboxSyncProbeScript = `
const { execFileSync } = require("child_process");
const crypto = require("crypto");
const fs = require("fs");
const root = "/workspace/project";
function git(args) {
  return execFileSync("git", ["-C", root].concat(args), { maxBuffer: 256 * 1024 * 1024 });
}
let branch = Buffer.alloc(0);
try {
  branch = git(["symbolic-ref", "--quiet", "--short", "HEAD"]);
  branch = Buffer.from(branch.toString("utf8").trim());
} catch (_) {}
const head = git(["rev-parse", "HEAD"]).toString("utf8").trim();
const diff = git(["diff", "HEAD", "--binary", "--full-index"]);
const untracked = [];
const raw = git(["ls-files", "--others", "--exclude-standard", "-z"]);
let start = 0;
const names = [];
for (let i = 0; i < raw.length; i += 1) {
  if (raw[i] === 0) {
    if (i > start) names.push(raw.subarray(start, i));
    start = i + 1;
  }
}
const prefix = Buffer.from(root + "/");
for (const name of names) {
  const full = Buffer.concat([prefix, name]);
  const stat = fs.lstatSync(full);
  const mode = (stat.mode & 0o777).toString(8).padStart(4, "0");
  if (stat.isSymbolicLink()) {
    const target = fs.readlinkSync(full, { encoding: "buffer" });
    untracked.push({ path: name.toString("base64"), kind: "symlink", mode, target: target.toString("base64") });
  } else if (stat.isFile()) {
    const digest = crypto.createHash("sha256").update(fs.readFileSync(full)).digest("hex");
    untracked.push({ path: name.toString("base64"), kind: "file", mode, sha256: digest });
  } else {
    throw new Error("unsupported untracked file type: " + name.toString("utf8"));
  }
}
process.stdout.write(JSON.stringify({
  branch: branch.toString("base64"),
  head,
  diff_sha256: crypto.createHash("sha256").update(diff).digest("hex"),
  untracked
}));
`

func (a *App) syncProbeStore() *syncProbeStore {
	syncProbeInitMu.Lock()
	defer syncProbeInitMu.Unlock()
	if a.syncProbes == nil {
		a.syncProbes = &syncProbeStore{entries: map[string]syncProbeCacheEntry{}}
	}
	return a.syncProbes
}

func (s *syncProbeStore) get(id, sandbox string, now time.Time) (syncSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok || entry.sandbox != sandbox || now.Sub(entry.probedAt) > syncProbeTTL {
		return syncSnapshot{}, false
	}
	return entry.snapshot, true
}

func (s *syncProbeStore) set(id, sandbox string, snapshot syncSnapshot, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = syncProbeCacheEntry{sandbox: sandbox, probedAt: now, snapshot: snapshot}
}

func (s *syncProbeStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

func (a *App) invalidateSyncProbe(id string) {
	a.syncProbeStore().delete(id)
}

func (a *App) sandboxSyncSnapshot(ctx context.Context, r Run) (syncSnapshot, error) {
	out, err := a.sandboxExec(ctx, r, "node", "-e", sandboxSyncProbeScript)
	if err != nil {
		return syncSnapshot{}, err
	}
	var snapshot syncSnapshot
	if err := json.Unmarshal(bytes.TrimSpace(out), &snapshot); err != nil {
		return syncSnapshot{}, fmt.Errorf("sandbox sync probe was unreadable: %w", err)
	}
	if err := validateSyncSnapshot(snapshot); err != nil {
		return syncSnapshot{}, err
	}
	return snapshot, nil
}

func validateSyncSnapshot(snapshot syncSnapshot) error {
	if snapshot.Head == "" {
		return fmt.Errorf("sandbox sync probe returned no commit")
	}
	if len(snapshot.DiffSHA256) != sha256.Size*2 {
		return fmt.Errorf("sandbox sync probe returned an invalid diff fingerprint")
	}
	for _, entry := range snapshot.Untracked {
		if len(entry.Path) == 0 || entry.Mode == "" {
			return fmt.Errorf("sandbox sync probe returned an invalid untracked entry")
		}
		switch entry.Kind {
		case "file":
			if len(entry.SHA256) != sha256.Size*2 {
				return fmt.Errorf("sandbox sync probe returned an invalid file fingerprint")
			}
		case "symlink":
		default:
			return fmt.Errorf("sandbox sync probe returned unsupported entry type %q", entry.Kind)
		}
	}
	return nil
}

func (a *App) syncSnapshotForRun(ctx context.Context, r Run, fresh bool) (syncSnapshot, error) {
	store := a.syncProbeStore()
	if !fresh {
		if snapshot, ok := store.get(r.ID, r.Sandbox, time.Now()); ok {
			return snapshot, nil
		}
	}
	snapshot, err := a.sandboxSyncSnapshot(ctx, r)
	if err != nil {
		store.delete(r.ID)
		return syncSnapshot{}, err
	}
	store.set(r.ID, r.Sandbox, snapshot, time.Now())
	return snapshot, nil
}

// deriveSyncState applies the documented decision order. Probe and host
// inspection failures always become unknown; they never become up to date.
func (a *App) deriveSyncState(ctx context.Context, r Run, fresh bool) SyncState {
	if r.Sandbox == "" || r.Worktree == "" {
		return SyncStateUnknown
	}
	snapshot, err := a.syncSnapshotForRun(ctx, r, fresh)
	if err != nil {
		return SyncStateUnknown
	}
	if r.SyncedSandboxFingerprint == "" && snapshot.clean(r.Baseline) {
		return SyncStateNoChanges
	}
	current := snapshot.Fingerprint()
	if r.SyncedSandboxFingerprint != "" && current == r.SyncedSandboxFingerprint {
		return SyncStateUpToDate
	}
	hostFingerprint, err := a.WorktreeFingerprint(ctx, r)
	if err != nil {
		return SyncStateUnknown
	}
	if r.Fingerprint == "" || hostFingerprint != r.Fingerprint {
		return SyncStateBlocked
	}
	return SyncStateNeedsSync
}

// snapshotFromExport builds the successful-sync fingerprint from the exact
// downloaded snapshot that was applied to the host worktree. Using the export
// bytes rather than host git output keeps the recorded fingerprint identical to
// the sandbox probe even when host and sandbox git versions differ.
func snapshotFromExport(branch, head string, patch []byte, archive string) (syncSnapshot, error) {
	entries, err := tarSnapshotEntries(archive)
	if err != nil {
		return syncSnapshot{}, err
	}
	return syncSnapshot{
		Branch:     []byte(branch),
		Head:       head,
		DiffSHA256: hashBytes(patch),
		Untracked:  entries,
	}, nil
}

func tarSnapshotEntries(path string) ([]syncSnapshotEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	var out []syncSnapshotEntry
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			hsh := sha256.New()
			if _, err := io.Copy(hsh, tr); err != nil {
				return nil, err
			}
			out = append(out, syncSnapshotEntry{
				Path:   []byte(h.Name),
				Kind:   "file",
				Mode:   fmt.Sprintf("%04o", h.Mode&0777),
				SHA256: hex.EncodeToString(hsh.Sum(nil)),
			})
		case tar.TypeSymlink:
			out = append(out, syncSnapshotEntry{
				Path:   []byte(h.Name),
				Kind:   "symlink",
				Mode:   fmt.Sprintf("%04o", h.Mode&0777),
				Target: []byte(h.Linkname),
			})
		default:
			return nil, fmt.Errorf("unsupported archive entry %q", h.Name)
		}
	}
}

// collectSnapshotDir is the local counterpart of the sandbox probe. The host
// path uses it for tests and diagnostics; production success bookkeeping uses
// snapshotFromExport so the recorded fingerprint comes from the transferred
// snapshot itself.
func collectSnapshotDir(ctx context.Context, runner CommandRunner, dir string) (syncSnapshot, error) {
	branchOut, err := runner.Output(ctx, "git", "-C", dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	var branch []byte
	if err == nil {
		branch = []byte(strings.TrimSpace(string(branchOut)))
	}
	headOut, err := runner.Output(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	if err != nil {
		return syncSnapshot{}, err
	}
	diffOut, err := runner.Output(ctx, "git", "-C", dir, "diff", "HEAD", "--binary", "--full-index")
	if err != nil {
		return syncSnapshot{}, err
	}
	listOut, err := runner.Output(ctx, "git", "-C", dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return syncSnapshot{}, err
	}
	names := splitNUL(listOut)
	sort.Strings(names)
	entries := make([]syncSnapshotEntry, 0, len(names))
	for _, name := range names {
		full := filepath.Join(dir, name)
		info, err := os.Lstat(full)
		if err != nil {
			return syncSnapshot{}, err
		}
		mode := fmt.Sprintf("%04o", info.Mode().Perm())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(full)
			if err != nil {
				return syncSnapshot{}, err
			}
			entries = append(entries, syncSnapshotEntry{Path: []byte(name), Kind: "symlink", Mode: mode, Target: []byte(target)})
			continue
		}
		if !info.Mode().IsRegular() {
			return syncSnapshot{}, fmt.Errorf("unsupported untracked file type: %s", name)
		}
		f, err := os.Open(full)
		if err != nil {
			return syncSnapshot{}, err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return syncSnapshot{}, copyErr
		}
		if closeErr != nil {
			return syncSnapshot{}, closeErr
		}
		entries = append(entries, syncSnapshotEntry{Path: []byte(name), Kind: "file", Mode: mode, SHA256: hex.EncodeToString(h.Sum(nil))})
	}
	return syncSnapshot{
		Branch:     branch,
		Head:       strings.TrimSpace(string(headOut)),
		DiffSHA256: hashBytes(diffOut),
		Untracked:  entries,
	}, nil
}

package scanner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ohing504/devclean/internal/model"
)

// globalCache describes a shared, home-rooted cache that is not tied to any
// single project. Unlike the per-project scanners, these paths are fixed
// (home-relative) and span package managers and dev tools across ecosystems.
//
// rec carries the consequence-of-deletion note surfaced as ScanResult.Recommendation:
// for caution entries it states what breaks ("re-downloaded on next install"),
// so a user — or an AI agent reading --json — can decide without external knowledge.
//
// tools lists the executables that read and write the cache, in preference
// order. When one of them is installed (findExecutable) the cache is in use: deleting it only
// makes the tool download the same content again, so the entry is reported as
// caution and --yes skips it. When none is installed the entry keeps its declared
// safety and is reported as a cache no installed tool uses. tools is set only
// on entries declared safe (a caution entry already needs explicit opt-in) and
// left empty for caches whose owner has no executable of its own in PATH
// (libraries such as Puppeteer or Electron, and app caches such as Cursor's).
//
// Paths owned by a dedicated scanner are intentionally excluded to avoid
// double-counting: Xcode's DerivedData / DeviceSupport / Archives / CoreSimulator
// belong to the xcode ecosystem, not here.
type globalCache struct {
	relPath     string
	category    model.Category
	safety      model.SafetyLevel
	description string
	rec         string
	tools       []string
}

// globalCaches lists shared caches as home-relative paths. Entries whose path
// does not exist on the current platform are skipped at scan time, so macOS
// (~/Library/Caches/*) and Linux (~/.cache/*) variants can coexist here.
var globalCaches = []globalCache{
	// --- Package managers (cross-platform) ---
	{".npm", model.CatCache, model.SafetySafe, "npm package cache", "", []string{"npm"}},
	{".bun/install/cache", model.CatCache, model.SafetySafe, "Bun install cache", "", []string{"bun"}},
	{".gradle/caches", model.CatCache, model.SafetyCaution, "Gradle dependency & build cache", "Gradle re-downloads dependencies and rebuilds on next run", nil},
	{".gradle/wrapper/dists", model.CatCache, model.SafetyCaution, "Gradle wrapper distributions", "Gradle re-downloads its distribution on next run", nil},
	{".cargo/registry", model.CatCache, model.SafetyCaution, "Cargo registry cache", "Cargo re-downloads crate sources on next build", nil},
	{".cargo/git", model.CatCache, model.SafetyCaution, "Cargo git dependency cache", "Cargo re-clones git dependencies on next build", nil},
	{"go/pkg/mod", model.CatDeps, model.SafetyCaution, "Go module cache", "use 'go clean -modcache' — read-only files make --force fail; Go re-downloads on next build", nil},
	{".pub-cache", model.CatDeps, model.SafetyCaution, "Dart/Flutter pub package cache", "shared by every Flutter project; packages re-download on next 'flutter pub get'", nil},
	{".rustup/toolchains", model.CatRuntime, model.SafetyCaution, "Rust toolchains", "installed toolchains must be reinstalled with 'rustup toolchain install'", nil},
	{".nvm/versions", model.CatRuntime, model.SafetyCaution, "nvm-installed Node.js runtimes", "deletes installed Node.js versions, not a cache — reinstall with 'nvm install'", nil},
	{".pyenv/versions", model.CatRuntime, model.SafetyCaution, "pyenv-installed Python runtimes", "deletes installed Python versions, not a cache — reinstall with 'pyenv install'", nil},
	{".rbenv/versions", model.CatRuntime, model.SafetyCaution, "rbenv-installed Ruby runtimes", "deletes installed Ruby versions, not a cache — reinstall with 'rbenv install'", nil},
	{".local/pipx", model.CatDeps, model.SafetyCaution, "pipx-installed CLI tools", "deletes the installed CLI tools themselves — each must be reinstalled with 'pipx install'", nil},
	{".m2/repository", model.CatDeps, model.SafetyCaution, "Maven local repository", "shared by every Maven project; dependencies re-download on next build", nil},
	// NOTE: `~/.gem` is NOT listed — it is a config root, not a cache. It holds
	// `~/.gem/credentials` (the RubyGems push API key) alongside installed gems.
	// Deleting it would leak-by-loss a credential and remove installed gems, so
	// it must never be a deletion target. (RubyGems' download cache lives under
	// `~/Library/Caches/CocoaPods`-style OS cache dirs, listed below where safe.)
	{".cocoapods", model.CatCache, model.SafetySafe, "CocoaPods spec repos", "", []string{"pod"}},
	// uv and Puppeteer (v19+) use the XDG ~/.cache path on macOS too, so these
	// live here rather than in the Linux section.
	{".cache/uv", model.CatCache, model.SafetySafe, "uv package cache", "", []string{"uv"}},
	{".cache/puppeteer", model.CatCache, model.SafetySafe, "Puppeteer browser cache", "", nil},

	// --- Package managers / dev tools (macOS) ---
	{"Library/Caches/Yarn", model.CatCache, model.SafetySafe, "Yarn cache", "", []string{"yarn"}},
	{"Library/pnpm/store", model.CatCache, model.SafetyCaution, "pnpm content-addressable store", "every project re-downloads dependencies on next install (store is hard-linked)", nil},
	{"Library/Caches/pnpm", model.CatCache, model.SafetySafe, "pnpm download cache", "", []string{"pnpm"}},
	{"Library/Caches/pip", model.CatCache, model.SafetySafe, "pip download cache", "", []string{"pip", "pip3"}},
	// With brew installed, the Homebrew cleanup item (global_homebrew.go)
	// replaces this entry when brew reports reclaimable space here.
	{homebrewCacheRelPath, model.CatCache, model.SafetySafe, "Homebrew download cache", "", []string{"brew"}},
	{"Library/Caches/CocoaPods", model.CatCache, model.SafetySafe, "CocoaPods spec & pod cache", "", []string{"pod"}},
	{"Library/Caches/go-build", model.CatCache, model.SafetySafe, "Go build cache", "", []string{"go"}},
	{"Library/Caches/ms-playwright", model.CatCache, model.SafetyCaution, "Playwright browser binaries", "Playwright re-downloads browsers on next install", nil},
	{"Library/Caches/electron", model.CatCache, model.SafetySafe, "Electron binary cache", "", nil},
	{"Library/Caches/node-gyp", model.CatCache, model.SafetySafe, "node-gyp header cache", "", nil},
	{"Library/Caches/typescript", model.CatCache, model.SafetySafe, "TypeScript installer cache", "", nil},
	{"Library/Caches/uv", model.CatCache, model.SafetySafe, "uv package cache", "", []string{"uv"}},
	{"Library/Caches/Cypress", model.CatCache, model.SafetySafe, "Cypress binary cache", "", nil},
	{"Library/Caches/deno", model.CatCache, model.SafetySafe, "Deno cache", "", []string{"deno"}},
	{"Library/Caches/pypoetry", model.CatCache, model.SafetySafe, "Poetry cache", "", []string{"poetry"}},

	// --- Dev tools (Linux ~/.cache equivalents) ---
	{".cache/go-build", model.CatCache, model.SafetySafe, "Go build cache", "", []string{"go"}},
	{".cache/pip", model.CatCache, model.SafetySafe, "pip download cache", "", []string{"pip", "pip3"}},
	{".cache/ms-playwright", model.CatCache, model.SafetyCaution, "Playwright browser binaries", "Playwright re-downloads browsers on next install", nil},
	{".cache/node-gyp", model.CatCache, model.SafetySafe, "node-gyp header cache", "", nil},
	{".cache/yarn", model.CatCache, model.SafetySafe, "Yarn cache", "", []string{"yarn"}},
	{".cache/pnpm", model.CatCache, model.SafetySafe, "pnpm download cache", "", []string{"pnpm"}},
	{".cache/electron", model.CatCache, model.SafetySafe, "Electron binary cache", "", nil},
	{".cache/Cypress", model.CatCache, model.SafetySafe, "Cypress binary cache", "", nil},
	{".cache/deno", model.CatCache, model.SafetySafe, "Deno cache", "", []string{"deno"}},
	{".cache/pypoetry", model.CatCache, model.SafetySafe, "Poetry cache", "", []string{"poetry"}},

	// --- Android SDK (home-rooted, large; no dedicated scanner yet) ---
	// NOTE: `~/.android/avd` is deliberately excluded. An emulator's virtual disk
	// can hold user-created state (installed apps, files written inside the
	// emulated device) that a fresh AVD does not restore — irreplaceable data,
	// not regenerable cache. Only re-downloadable SDK artifacts are listed below.
	{"Library/Android/sdk/system-images", model.CatRuntime, model.SafetyCaution, "Android emulator system images", "emulators won't start until images are re-downloaded", nil},
	{"Library/Android/sdk/ndk", model.CatDeps, model.SafetyCaution, "Android NDK installations", "NDK is re-downloaded on next native build", nil},
	{"Library/Android/sdk/build-tools", model.CatRuntime, model.SafetyCaution, "Android SDK build tools", "builds fail until build-tools are re-downloaded via the SDK manager", nil},

	// --- AI tools ---
	// NOTE: AI coding-tool home directories are deliberately NOT in this
	// catalog. The whole `~/.claude` tree (session transcripts, project memory,
	// agents, skills, plugins, todos), plus `~/.codex` and `~/.gemini`, is user
	// state — deleting any of it is unrecoverable loss, and the "caches" inside
	// (plugin cache, shell snapshots, CLI logs) only reappear as install-time
	// scaffolding, so reclaiming them is worthless against that risk. These
	// tools are excluded entirely rather than listed with per-subdir caches.
	// `~/.cursor` (Cursor extensions & settings — a config root) is excluded for
	// the same reason. Only Cursor's OS-level cache SUBDIRS below are eligible.
	//
	// Only Cursor's cache subdirectories — Application Support/Cursor itself
	// holds settings and must never be offered for deletion.
	{"Library/Application Support/Cursor/Cache", model.CatCache, model.SafetySafe, "Cursor cache", "", nil},
	{"Library/Application Support/Cursor/CachedData", model.CatCache, model.SafetySafe, "Cursor cached data", "", nil},
	{"Library/Application Support/Cursor/Code Cache", model.CatCache, model.SafetySafe, "Cursor code cache", "", nil},
}

// GlobalScanner scans for shared, home-rooted developer caches that are not
// owned by any single project (package-manager and dev-tool caches). With brew
// installed it reports a Homebrew cleanup item reclaimed by `brew cleanup`. On
// macOS it additionally reports leftover browser code-sign clones under the
// per-user system temp root.
type GlobalScanner struct {
	// TmpRoot is the per-user system temp root searched for browser
	// code-sign clones (macOS: /private/var/folders). A field so tests can
	// point it at a fixture directory.
	TmpRoot string
	// ProcessRunning reports whether a process with exactly the given name
	// is currently running (default: pgrep -x). A field so tests can stub
	// browser run state.
	ProcessRunning func(processName string) bool
	// LookPath resolves an installed executable (default: findExecutable,
	// which searches PATH and then user-level install directories). It
	// decides which caches are in use by an installed tool, whether the
	// Homebrew cleanup item applies, and which VendorCleanups are offered. A
	// field so tests can stub which tools are installed.
	LookPath func(file string) (string, error)
	// RunCommand runs an external tool with extra environment variables and
	// returns its standard output (default: execCommand). A field so tests can
	// stub tool output and record the commands a delete would run.
	RunCommand func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

func NewGlobalScanner() *GlobalScanner {
	return &GlobalScanner{
		TmpRoot:        "/private/var/folders",
		ProcessRunning: processRunning,
		LookPath:       findExecutable,
		RunCommand:     execCommand,
	}
}

// execCommand runs name with args, adding env to the current environment, and
// returns standard output. A non-zero exit is returned as an error that
// includes the command's standard error, so the cause reaches the user.
func execCommand(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
		}
		return out, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

func (s *GlobalScanner) Name() string               { return "global" }
func (s *GlobalScanner) Ecosystem() model.Ecosystem { return model.EcoGlobal }

// userToolDirs lists directories that installers put executables in and
// that a login shell adds to PATH through profile scripts. A scan started
// without those scripts (launchd, cron, an agent's non-login shell) lacks
// them in PATH, and a tool missed there would make its in-use cache look
// unused.
func userToolDirs(home string) []string {
	dirs := []string{
		"/opt/homebrew/bin", // Homebrew on Apple silicon
		"/usr/local/bin",    // Homebrew on Intel macOS, manual installs
		"/usr/local/go/bin", // official Go installer
		"/home/linuxbrew/.linuxbrew/bin",
		filepath.Join(home, ".local", "bin"), // pipx, uv and Poetry installers
		filepath.Join(home, ".cargo", "bin"),
		filepath.Join(home, ".bun", "bin"),
		filepath.Join(home, ".deno", "bin"),
		filepath.Join(home, ".volta", "bin"),
		filepath.Join(home, "Library", "pnpm"),                  // pnpm standalone installer (macOS)
		filepath.Join(home, ".local", "share", "pnpm"),          // pnpm standalone installer (Linux)
		filepath.Join(home, ".pyenv", "shims"),                  // pip of pyenv Pythons
		filepath.Join(home, ".rbenv", "shims"),                  // pod of rbenv Rubies
		filepath.Join(home, ".asdf", "shims"),                   // asdf-managed runtimes
		filepath.Join(home, ".local", "share", "mise", "shims"), // mise-managed runtimes
	}
	// nvm keeps one bin directory per installed Node.js version, each with
	// its own npm (and yarn/pnpm when installed globally).
	if nvm, err := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin")); err == nil {
		dirs = append(dirs, nvm...)
	}
	return dirs
}

// findExecutable resolves file in PATH, then in userToolDirs. It is
// GlobalScanner.LookPath's default.
func findExecutable(file string) (string, error) {
	if p, err := exec.LookPath(file); err == nil {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", exec.ErrNotFound
	}
	for _, dir := range userToolDirs(home) {
		p := filepath.Join(dir, file)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return p, nil
		}
	}
	return "", exec.ErrNotFound
}

// installedTool returns the first of tools that is installed, as its name
// and resolved path, or "" and "" when none is.
func (s *GlobalScanner) installedTool(tools []string) (name, path string) {
	for _, t := range tools {
		if p, err := s.lookPath(t); err == nil {
			return t, p
		}
	}
	return "", ""
}

// globalVendorCleanup describes a package manager's native cache-prune command.
// tools lists candidate executables in preference order (first one installed
// is used), so pip3-only machines still match the pip entry.
type globalVendorCleanup struct {
	id    string
	tools []string
	args  []string
	desc  string
}

// globalVendorCleanups are vendor-native prune commands for the global caches.
// All are non-destructive — they only reclaim regenerable download/store caches,
// so no destructive-action gate is needed here. Homebrew is not listed: its
// cleanup is a scan item of its own (global_homebrew.go), selected and sized
// like any other item.
var globalVendorCleanups = []globalVendorCleanup{
	{"npm-cache-clean", []string{"npm"}, []string{"cache", "clean", "--force"}, "Clear the npm package cache"},
	{"yarn-cache-clean", []string{"yarn"}, []string{"cache", "clean"}, "Clear the Yarn cache"},
	{"pnpm-store-prune", []string{"pnpm"}, []string{"store", "prune"}, "Remove unreferenced packages from the pnpm store"},
	{"pip-cache-purge", []string{"pip", "pip3"}, []string{"cache", "purge"}, "Remove all wheels from the pip cache"},
	{"uv-cache-prune", []string{"uv"}, []string{"cache", "prune"}, "Remove outdated entries from the uv cache"},
}

// VendorCleanups returns prune commands for the package managers installed on
// this machine. Tools absent from PATH are skipped so the offer only lists what
// can actually run. Since every global cache shares the one ecosystem, these run
// together whenever the global ecosystem is in a --vendor-cleanup scope.
func (s *GlobalScanner) VendorCleanups() []VendorCleanup {
	var out []VendorCleanup
	for _, v := range globalVendorCleanups {
		tool, toolPath := s.installedTool(v.tools)
		if tool == "" {
			continue // none of the candidate executables are installed
		}
		args := v.args
		out = append(out, VendorCleanup{
			ID:          v.id,
			Description: v.desc,
			DeleteMethod: model.DeleteMethod{
				Kind:    model.DeleteKindCommand,
				Display: tool + " " + strings.Join(args, " "),
				Run: func(ctx context.Context) error {
					return exec.CommandContext(ctx, toolPath, args...).Run()
				},
			},
		})
	}
	return out
}

func (s *GlobalScanner) Scan(ctx context.Context, root string) ([]model.ScanResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	// The Homebrew cleanup reaches outside home (old versions under the
	// Homebrew prefix), so like the code-sign clones it is included only when
	// the scan root covers home. Its dry-run takes seconds, so it runs
	// concurrently with the catalog stat and sizing below.
	var homebrew <-chan homebrewScan
	if isUnderRoot(home, absRoot) {
		if brew, err := s.lookPath("brew"); err == nil {
			homebrew = s.startHomebrewScan(ctx, brew)
		}
	}

	var results []model.ScanResult
	for _, c := range globalCaches {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		full := filepath.Join(home, c.relPath)
		if !isUnderRoot(full, absRoot) || !isDir(full) {
			continue
		}
		results = append(results, s.catalogResult(c, full))
		ReportProgress(ctx, len(results))
	}

	// Browser code-sign clones live outside home (per-user system temp), so
	// isUnderRoot on the path itself would never match. Gate them the same
	// way as the home caches: include only when the scan root covers the
	// home directory (the default scan root is ~). A --path scan of a home
	// subdirectory must not surface system temp entries.
	if runtime.GOOS == "darwin" && isUnderRoot(home, absRoot) {
		results = append(results, s.scanCodeSignClones(ctx, len(results))...)
	}

	// Size all caches (and any code-sign clones) in one bounded worker pool
	// rather than serially as each is discovered.
	if err := sizePending(ctx, results); err != nil {
		return results, err
	}

	if homebrew != nil {
		results = mergeHomebrew(results, <-homebrew, filepath.Join(home, homebrewCacheRelPath))
		ReportProgress(ctx, len(results))
	}
	return results, nil
}

// lookPath resolves an executable with the scanner's LookPath.
func (s *GlobalScanner) lookPath(file string) (string, error) {
	if s.LookPath == nil {
		return exec.LookPath(file)
	}
	return s.LookPath(file)
}

// runCommand runs an external tool with the scanner's RunCommand.
func (s *GlobalScanner) runCommand(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	if s.RunCommand == nil {
		return execCommand(ctx, env, name, args...)
	}
	return s.RunCommand(ctx, env, name, args...)
}

// catalogResult builds the unsized result for a catalog entry found at full.
// An installed tool that uses the cache raises the entry to caution; an entry
// whose tools are all absent keeps its declared safety with a note saying no
// installed tool uses it. A declared rec always takes precedence over these
// generated notes.
func (s *GlobalScanner) catalogResult(c globalCache, full string) model.ScanResult {
	safety, rec := c.safety, c.rec
	if len(c.tools) > 0 {
		if tool, _ := s.installedTool(c.tools); tool != "" {
			safety = model.SafetyCaution
			if rec == "" {
				rec = fmt.Sprintf("%s is installed and uses this cache; deleting it only makes %s download the same content again", tool, tool)
			}
		} else if rec == "" {
			rec = fmt.Sprintf("%s not installed, so no installed tool is known to use this cache", strings.Join(c.tools, "/"))
		}
	}
	return model.ScanResult{
		Path:           full,
		Ecosystem:      model.EcoGlobal,
		Category:       c.category,
		LastMod:        ModTime(full),
		Safety:         safety,
		ProjectRoot:    filepath.Dir(full),
		Label:          c.description,
		Recommendation: rec,
	}
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

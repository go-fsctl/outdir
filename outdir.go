// Copyright (c) the go-fsctl authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

// Package outdir chooses where a file that must never be committed may be
// written.
//
// ⛔ THE MISTAKE IT PREVENTS HAS HAPPENED. A live test wrote a capture of a
// whole desktop into a PUBLIC repository's testdata/, untracked -- one
// `git add -A` from publication. A capture of a real display is a picture of a
// person at work, and a camera frame is a picture of a person; a log of a
// signed-in account's traffic is data about a real person's account.
//
// ⛔ A .gitignore ENTRY IS THE WRONG FIX. Ignoring is a safety net, not a
// barrier: `git add -f`, a fresh clone, or any tool that does not consult it
// publishes the file anyway. The barrier is to write somewhere that is not in a
// work tree at all, and to REFUSE when the chosen place is.
//
// The directory is also DURABLE, which the temporary one a test framework hands
// out is not: a t.TempDir() is removed when the test ends, so the artefact is
// gone before anybody can open it. What a person needs after a failure is the
// picture, and a path in the log that still exists.
//
// # Using it
//
//	dir, err := outdir.Choose(outdir.Spec{
//	    App: "xrdesk",
//	    Env: "XRDESK_CAPTURE_DIR",
//	    Sub: "captures",
//	})
//
// A caller who lets a person override the place through an environment
// variable gets the same check applied to their choice, because the mistake
// this prevents is exactly the one a person makes.
package outdir

import (
	"fmt"
	"os"
	"path/filepath"
)

// Spec says what is being written and where it may go.
type Spec struct {
	// App is the program's name, used to make the default directory. It is
	// required: a shared parent with everybody's captures in it is a directory
	// nobody can clean up.
	App string

	// Env is an environment variable that overrides the default. Empty means
	// there is no override.
	//
	// The value is CHECKED like any other, and refused the same way. A person
	// who points it at their work tree has made the exact mistake this exists
	// to prevent, and taking their word for it would be the one case where the
	// barrier is not there.
	Env string

	// Sub is a subdirectory under the default, so a program that writes two
	// kinds of thing can keep them apart. Empty puts them directly under the
	// program's own directory.
	Sub string

	// Want is an explicit directory, which takes precedence over Env and over
	// the default. It is checked the same way.
	Want string

	// Base overrides where the default directory is rooted. Empty asks
	// os.UserConfigDir, which is the durable per-user place on every platform
	// -- ~/Library/Application Support on macOS, $XDG_CONFIG_HOME on Linux,
	// %AppData% on Windows.
	//
	// It exists so a test can put a whole run somewhere of its own without
	// setting an environment variable for the process.
	Base string
}

// Choose reports the absolute directory to write in, or refuses.
//
// It does NOT create the directory: a caller that decides not to write should
// not leave an empty one behind. Call [Ensure] when the file is about to be
// written.
func Choose(s Spec) (string, error) {
	if s.App == "" {
		return "", fmt.Errorf("outdir: no program name, so there is no directory to choose")
	}

	// chosen names the directory the way the person reading a refusal would:
	// by the variable when they set one, by what it is otherwise.
	dir, chosen := s.Want, "the directory given"
	if dir == "" && s.Env != "" {
		if v := os.Getenv(s.Env); v != "" {
			dir, chosen = v, s.Env
		}
	}
	if dir == "" {
		chosen = "the default output directory"
		base := s.Base
		if base == "" {
			var err error
			base, err = userConfigDir()
			if err != nil {
				return "", fmt.Errorf("outdir: no per-user configuration directory to write in: %w", err)
			}
		}
		dir = filepath.Join(base, s.App)
		if s.Sub != "" {
			dir = filepath.Join(dir, s.Sub)
		}
	}

	abs, err := absPath(dir)
	if err != nil {
		return "", fmt.Errorf("outdir: %s (%q): %w", chosen, dir, err)
	}
	if root := RepoRootOf(abs); root != "" {
		return "", fmt.Errorf("outdir: %s (%q) is inside the git work tree at %s; "+
			"a file that must never be committed cannot be written where it can be",
			chosen, abs, root)
	}
	return abs, nil
}

// Ensure is [Choose] with the directory made.
func Ensure(s Spec) (string, error) {
	dir, err := Choose(s)
	if err != nil {
		return "", err
	}
	if err := mkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("outdir: %q: %w", dir, err)
	}
	return dir, nil
}

// RepoRootOf is the git work tree this path is inside, or "".
//
// ⛔ IT WALKS UP TO THE FILESYSTEM ROOT, and that is the whole of the check: a
// directory is inside a work tree when ANY ancestor holds a .git, not only when
// its immediate parent does. testdata/ is three levels down from the .git that
// would publish it.
//
// A .git FILE counts as well as a directory. That is what a git worktree and a
// submodule leave behind -- a file holding "gitdir: ..." -- and a capture
// written into one is as committable as any other.
//
// ⛔ IT WALKS FROM THE NEAREST ANCESTOR THAT EXISTS, resolved through symbolic
// links. Both halves matter, and they interact:
//
//   - A path reaching a work tree through a link is IN it, and walking the
//     unresolved path would answer no. On macOS every temporary directory is
//     reached through /var, which is a link to /private/var.
//   - The path usually does NOT exist yet -- a program asks where it may write
//     before making anything -- so EvalSymlinks on it fails outright.
//     Resolving only when the whole path exists would make the ANSWER's form
//     depend on whether the directory had been made, which is how this was
//     found: the same tree came back as /var/... before it existed and
//     /private/var/... after.
//
// A component that does not exist cannot hold a .git, so nothing is lost by
// starting at the deepest one that does.
func RepoRootOf(path string) string {
	dir := nearestExisting(path)
	for {
		if isRepoMarker(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isRepoMarker reports whether this path is a .git of either shape.
func isRepoMarker(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.IsDir() || fi.Mode().IsRegular()
}

// nearestExisting is the deepest ancestor of path that exists, resolved through
// symbolic links -- or path itself, absolute, when nothing on it does.
func nearestExisting(path string) string {
	abs, err := absPath(path)
	if err != nil {
		abs = path
	}
	for dir := abs; ; {
		if resolved, err := evalSymlinks(dir); err == nil {
			return resolved
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		dir = parent
	}
}

// The platform calls, replaced in tests.
//
// ⛔ THE FAILURE PATHS ARE THE ONES THAT MATTER, and they are exactly the ones
// a test cannot reach by asking the real filesystem: a Mac with no
// configuration directory, a directory that cannot be made, a working
// directory that has been removed under the process. Each is a place this
// package could hand back a path it should have refused, and without a seam
// each would be a branch nothing had ever taken.
var (
	userConfigDir = os.UserConfigDir
	mkdirAll      = os.MkdirAll
	absPath       = filepath.Abs
	evalSymlinks  = filepath.EvalSymlinks
)

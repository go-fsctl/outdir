// Copyright (c) the go-fsctl authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package outdir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInsideAWorkTreeIsRefused, and outside it is not.
//
// ⛔ BOTH DIRECTIONS, because a check that always says no is a check that will
// be deleted the first time it is in the way. The refusal must NAME the tree,
// so the person reading it knows which repository they were about to write
// into.
func TestInsideAWorkTreeIsRefused(t *testing.T) {
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Three levels down, which is where a testdata/ directory actually sits.
	deep := filepath.Join(tree, "pkg", "internal", "testdata")

	_, err := Choose(Spec{App: "probe", Want: deep})
	if err == nil {
		t.Fatal("a directory inside a work tree was accepted")
	}
	if !strings.Contains(err.Error(), tree) {
		t.Errorf("the refusal does not name the tree at %s: %v", tree, err)
	}

	// And outside: a sibling of the tree, not under it.
	outside := t.TempDir()
	got, err := Choose(Spec{App: "probe", Want: outside})
	if err != nil {
		t.Fatalf("a directory outside every work tree was refused: %v", err)
	}
	if got != abs(t, outside) {
		t.Errorf("Choose = %q, want %q", got, outside)
	}
}

// TestAGitFileCountsToo.
//
// ⛔ A worktree and a submodule leave a .git FILE holding "gitdir: ...", not a
// directory. A capture written into one is as committable as any other, and a
// check that only looked for a directory would let it through.
func TestAGitFileCountsToo(t *testing.T) {
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(tree, ".git"),
		[]byte("gitdir: /elsewhere/.git/worktrees/w\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Choose(Spec{App: "probe", Want: filepath.Join(tree, "out")}); err == nil {
		t.Error("a directory inside a linked worktree was accepted")
	}
}

// TestTheOverrideIsCheckedToo.
//
// ⛔ The mistake this prevents is exactly the one a PERSON makes, so taking
// their word for it would be the one case where the barrier is not there.
func TestTheOverrideIsCheckedToo(t *testing.T) {
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE_OUT", filepath.Join(tree, "captures"))

	_, err := Choose(Spec{App: "probe", Env: "PROBE_OUT"})
	if err == nil {
		t.Fatal("an environment variable pointing into a work tree was accepted")
	}
	// And the refusal names the VARIABLE, because that is what the person has
	// to change.
	if !strings.Contains(err.Error(), "PROBE_OUT") {
		t.Errorf("the refusal does not name the variable: %v", err)
	}
}

// TestWhereItGoesWhenNobodySaid.
func TestWhereItGoesWhenNobodySaid(t *testing.T) {
	base := t.TempDir()
	got, err := Choose(Spec{App: "xrdesk", Sub: "captures", Base: base})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "xrdesk", "captures")
	if got != abs(t, want) {
		t.Errorf("Choose = %q, want %q", got, want)
	}
	// Without a subdirectory it is the program's own.
	got, err = Choose(Spec{App: "xrdesk", Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if got != abs(t, filepath.Join(base, "xrdesk")) {
		t.Errorf("Choose = %q", got)
	}
}

// TestPrecedence: an explicit directory beats the variable, which beats the
// default. A caller that has been told exactly where to write must not be
// overruled by something in the environment.
func TestPrecedence(t *testing.T) {
	base, want, env := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("PROBE_OUT", env)

	got, err := Choose(Spec{App: "p", Base: base, Env: "PROBE_OUT", Want: want})
	if err != nil {
		t.Fatal(err)
	}
	if got != abs(t, want) {
		t.Errorf("Choose = %q, want the explicit %q", got, want)
	}
	got, err = Choose(Spec{App: "p", Base: base, Env: "PROBE_OUT"})
	if err != nil {
		t.Fatal(err)
	}
	if got != abs(t, env) {
		t.Errorf("Choose = %q, want the variable's %q", got, env)
	}
	// An EMPTY variable is not a choice: it falls through to the default
	// rather than becoming the current directory, which is usually a work tree.
	t.Setenv("PROBE_OUT", "")
	got, err = Choose(Spec{App: "p", Base: base, Env: "PROBE_OUT"})
	if err != nil {
		t.Fatal(err)
	}
	if got != abs(t, filepath.Join(base, "p")) {
		t.Errorf("an empty variable gave %q", got)
	}
}

// TestAProgramWithNoNameIsRefused: a shared parent with everybody's captures in
// it is a directory nobody can clean up.
func TestAProgramWithNoNameIsRefused(t *testing.T) {
	if _, err := Choose(Spec{}); err == nil {
		t.Error("a Spec with no App was accepted")
	}
}

// TestChooseMakesNothing, and Ensure makes it.
//
// A caller that decides not to write must not leave an empty directory behind:
// a program that made one at start-up and never used it would litter every
// machine it ran on.
func TestChooseMakesNothing(t *testing.T) {
	base := t.TempDir()
	dir, err := Choose(Spec{App: "p", Sub: "captures", Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Choose created %s", dir)
	}
	made, err := Ensure(Spec{App: "p", Sub: "captures", Base: base})
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(made); err != nil || !fi.IsDir() {
		t.Errorf("Ensure did not make %s: %v", made, err)
	}
	// Ensure refuses for the same reasons Choose does, rather than making a
	// directory first and refusing afterwards.
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(tree, "out")
	if _, err := Ensure(Spec{App: "p", Want: inside}); err == nil {
		t.Error("Ensure accepted a directory inside a work tree")
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Error("Ensure made the directory it then refused")
	}
}

// TestRepoRootOfWalksToTheTop, which is the whole of the check: testdata/ is
// three levels below the .git that would publish it.
func TestRepoRootOfWalksToTheTop(t *testing.T) {
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(tree, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoRootOf(deep); got != resolved(t, tree) {
		t.Errorf("RepoRootOf(%s) = %q, want %q", deep, got, tree)
	}
	// A path in no work tree at all answers "", walking all the way to the
	// filesystem root without looping.
	if got := RepoRootOf(t.TempDir()); got != "" {
		t.Errorf("a directory in no work tree answered %q", got)
	}
}

// TestADirectoryThatDoesNotExistYetIsStillChecked.
//
// The normal case: a program asks where to write before making anything. Every
// ancestor that DOES exist is still walked, so a capture directory that has not
// been made yet inside a work tree is refused before it is made.
func TestADirectoryThatDoesNotExistYetIsStillChecked(t *testing.T) {
	tree := t.TempDir()
	if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := RepoRootOf(filepath.Join(tree, "not", "made", "yet")); got != resolved(t, tree) {
		t.Errorf("a path that does not exist yet answered %q", got)
	}
}

// abs is what Choose gives back: absolute, and NOT resolved through symbolic
// links. Choose reports the directory a caller asked for, so it stays the path
// they will recognise -- on macOS a temporary directory is reached through
// /var, which is a link to /private/var, and answering the resolved form would
// hand back a path nobody wrote down.
func abs(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// resolved is what RepoRootOf gives back for a path that exists. It DOES
// resolve, because a path reaching a work tree through a link is in it, and
// comparing the unresolved form would answer no.
func resolved(t *testing.T, p string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(p)
	if err != nil {
		return abs(t, p)
	}
	return abs(t, got)
}

// TestWhenThePlatformCannotAnswer.
//
// ⛔ These are the branches a test cannot reach by asking the real filesystem,
// and each is a place this package could hand back a path it should have
// refused. A Mac with no configuration directory, a directory that cannot be
// made, a working directory removed under the process: without a seam, every
// one would be a branch nothing had ever taken.
func TestWhenThePlatformCannotAnswer(t *testing.T) {
	boom := errors.New("the platform said no")

	t.Run("no configuration directory", func(t *testing.T) {
		swap(t, &userConfigDir, func() (string, error) { return "", boom })
		_, err := Choose(Spec{App: "p"})
		if !errors.Is(err, boom) {
			t.Errorf("Choose = %v, want the platform's own error", err)
		}
	})

	t.Run("no working directory", func(t *testing.T) {
		swap(t, &absPath, func(string) (string, error) { return "", boom })
		_, err := Choose(Spec{App: "p", Want: "relative/path"})
		if !errors.Is(err, boom) {
			t.Errorf("Choose = %v, want the platform's own error", err)
		}
	})

	t.Run("the directory cannot be made", func(t *testing.T) {
		swap(t, &mkdirAll, func(string, os.FileMode) error { return boom })
		_, err := Ensure(Spec{App: "p", Base: t.TempDir()})
		if !errors.Is(err, boom) {
			t.Errorf("Ensure = %v, want the platform's own error", err)
		}
	})

	t.Run("nothing on the path can be resolved", func(t *testing.T) {
		// Every ancestor refuses, all the way to the filesystem root. The walk
		// must END rather than spin: filepath.Dir("/") is "/", so a loop that
		// did not notice would never return.
		swap(t, &evalSymlinks, func(string) (string, error) { return "", boom })
		if got := RepoRootOf("/a/b/c"); got != "" {
			t.Errorf("RepoRootOf = %q, want nothing", got)
		}
	})

	t.Run("a relative path that cannot be made absolute is still walked", func(t *testing.T) {
		// ⛔ IT MUST STILL FIND THE TREE. A working directory that cannot be
		// read is not a reason to stop checking: this test runs INSIDE a work
		// tree, so a relative path here is in one, and the answer must say so
		// rather than fall through to "nowhere" -- which is the shape of every
		// check that fails open.
		swap(t, &absPath, func(string) (string, error) { return "", boom })
		swap(t, &evalSymlinks, func(string) (string, error) { return "", boom })
		if got := RepoRootOf("relative"); got == "" {
			t.Error("a relative path inside this repository answered nothing")
		}
	})
}

// swap installs a replacement for the length of one test.
func swap[T any](t *testing.T, p *T, v T) {
	t.Helper()
	was := *p
	t.Cleanup(func() { *p = was })
	*p = v
}

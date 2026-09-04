# outdir

Choose where a file that must **never be committed** may be written.

```go
dir, err := outdir.Ensure(outdir.Spec{
    App: "xrdesk",
    Env: "XRDESK_CAPTURE_DIR",
    Sub: "captures",
})
```

Pure Go, `CGO_ENABLED=0`, no dependencies.

## Why

⛔ **The mistake it prevents has happened.** A live test wrote a capture of a
whole desktop into a **public** repository's `testdata/`, untracked — one
`git add -A` from publication. A capture of a real display is a picture of a
person at work; a camera frame is a picture of a person; a log of a signed-in
account's traffic is data about a real person's account.

⛔ **A `.gitignore` entry is the wrong fix.** Ignoring is a safety net, not a
barrier: `git add -f`, a fresh clone, or any tool that does not consult it
publishes the file anyway. The barrier is to write somewhere that is not in a
work tree at all — and to **refuse** when the chosen place is.

The directory is also **durable**, which a `t.TempDir()` is not: that is removed
when the test ends, so the artefact is gone before anybody can open it. What a
person needs after a failure is the picture, and a path in the log that still
exists.

## What it checks

- The work-tree walk goes **up to the filesystem root**. `testdata/` is three
  levels below the `.git` that would publish it.
- A `.git` **file** counts as well as a directory — that is what a linked
  worktree and a submodule leave behind.
- Symbolic links are resolved, from the **nearest ancestor that exists**: the
  path usually does not exist yet, and on macOS every temporary directory is
  reached through `/var`, a link to `/private/var`.
- An **override is checked too**. The mistake this prevents is exactly the one
  a person makes, so taking their word for it would be the one case where the
  barrier is not there.
- `Choose` creates nothing; `Ensure` creates it. A caller that decides not to
  write must not leave an empty directory behind.

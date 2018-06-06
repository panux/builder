package main

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/tools/godoc/vfs"
)

// GitRepo is a git repository.
type GitRepo struct {
	lck sync.Mutex
	dir string
}

// NewGitRepo creates a git repository with the URL in repo and the given path.
// This will clone the repo, which may be cancelled with the context.
func NewGitRepo(ctx context.Context, repo string, path string) (*GitRepo, error) {
	gr := &GitRepo{
		dir: path,
	}
	err := gr.clone(ctx, repo)
	if err != nil {
		return nil, err
	}
	return gr, nil
}

// clone clones a git repo.
// Not concurrency safe.
// Supports context cancellation.
func (gr *GitRepo) clone(ctx context.Context, repo string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", repo, gr.dir)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// Checkout checks out a branch.
// Not concurrency safe.
// Supports context cancellation.
func (gr *GitRepo) Checkout(ctx context.Context, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", gr.dir, "checkout", branch)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// Pull runs a git pull.
// Not concurrency safe.
// Supports context cancellation.
func (gr *GitRepo) Pull(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "-C", gr.dir, "pull")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// WithBranch locks the git repo, pulls, checks out a branch, and executes a callback.
// Protects from concurrent checkouts.
// The callback is called with the context and a VFS of the repo.
func (gr *GitRepo) WithBranch(ctx context.Context, branch string, callback func(ctx context.Context, repo vfs.FileSystem) error) error {
	gr.lck.Lock()
	defer gr.lck.Unlock()

	err := gr.Pull(ctx)
	if err != nil {
		return err
	}
	err = gr.Checkout(ctx, branch)
	if err != nil {
		return err
	}

	return callback(ctx, vfs.OS(gr.dir))
}

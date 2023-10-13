package rgit

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/sirupsen/logrus"
)

// This package contains functions that expand around the limitaiton of the go-git library
var (
	ErrRemotebranchNotFound = errors.New("remote branch not found")
)

func RunCommandInDir(dir, cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	command.Env = append(command.Env, "GIT_EDITOR=true")
	var outb, errb bytes.Buffer
	command.Stdout = &outb
	command.Stderr = &errb
	command.Dir = dir
	err := command.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %s", cmd+strings.Join(args, " "), errb.String())
	}
	return outb.String(), nil
}

func CleanRepo(dir string) error {
	if _, err := RunCommandInDir(dir, "git", "clean", "-xfd"); err != nil {
		return err
	}
	if _, err := RunCommandInDir(dir, "git", "checkout", "."); err != nil {
		return err
	}
	return nil
}

func GetLatestTagFromRepo(repo *git.Repository, filter string) (string, error) {
	tagRefs, err := repo.Tags()
	if err != nil {
		return "", err
	}

	var latestTagCommit *object.Commit
	var latestTag *plumbing.Reference
	err = tagRefs.ForEach(func(tagRef *plumbing.Reference) error {
		tagName := tagRef.Name().String()
		revision := plumbing.Revision(tagName)
		tagCommitHash, err := repo.ResolveRevision(revision)
		if err != nil {
			return err
		}

		commit, err := repo.CommitObject(*tagCommitHash)
		if err != nil {
			return err
		}

		if strings.Contains(tagName, filter) && latestTagCommit == nil {
			latestTagCommit = commit
			latestTag = tagRef
		}

		if strings.Contains(tagName, filter) && commit.Committer.When.After(latestTagCommit.Committer.When) {
			latestTagCommit = commit
			latestTag = tagRef
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return latestTag.Name().Short(), nil
}

func CreateNewBranchFromTag(dir, newBranchName, tag string) error {
	output, err := RunCommandInDir(dir, "git", "checkout", "-b", newBranchName, tag)
	if err != nil {
		return err
	}
	logrus.Debug(output)
	return nil
}

// CheckoutRemoteBranch is equivalent to `git checkout --track <REMOTE>/<BRANCH>`
// Due to go-git being half finished, this simple function is not easy to replicate. See https://github.com/go-git/go-git/issues/241
func CheckoutRemoteBranch(dir, remote, branchName string) error {
	remoteRef := fmt.Sprintf("%s/%s", remote, branchName)
	logrus.Info("Checking out remote branch ", remoteRef)
	output, err := RunCommandInDir(dir, "git", "checkout", "--track", remoteRef)
	logrus.Debug(output)
	if err != nil {
		errCatch := fmt.Sprintf("fatal: '%s' is not a commit and a branch '%s' cannot be created from it\n", remoteRef, branchName)
		if strings.Contains(err.Error(), errCatch) {
			return ErrRemotebranchNotFound
		}
		return err
	}
	return nil
}

// ActOnFiles peforms the given action on the list of files
func ActOnFiles(dir string, action string, files []string) error {
	if action == "add" {
		return AddFiles(dir, files)
	}
	if action == "remove" {
		return RemoveFiles(dir, files)
	}
	return nil
}

// RemoveFiles removes the list of files from the repo
func RemoveFiles(dir string, files []string) error {
	for _, file := range files {
		if _, err := RunCommandInDir(dir, "git", "rm", file); err != nil {
			return err
		}
	}
	return nil
}

// AddFiles adds the list of files to the repo
func AddFiles(dir string, files []string) error {
	for _, file := range files {
		if _, err := RunCommandInDir(dir, "git", "add", file); err != nil {
			return err
		}
	}
	return nil
}

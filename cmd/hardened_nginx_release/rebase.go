package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/rancher/ecm-distro-tools/release/rgit"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	nginxURL = "https://github.com/ORG/ingress-nginx"
	nginxDir = "ingress-nginx"
)

// List of the last hardened commit messages
var hardenedCommits = []string{
	"Adding drone and build artifacts",
	"Changes to E2E tests",
	"Hardened Nginx and S390x changes",
	"Use BCI base image",
	"Downgrade nginx to 1.21.4 for pcre compatability",
	"add arm64 support",
	"remove e2e like s390x",
}

func checkFlags(c *cli.Context) error {
	if c.Bool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
	}
	return nil
}

func Rebase(c *cli.Context) error {
	if err := checkFlags(c); err != nil {
		return err
	}
	ghUser := c.String("user")
	tag := c.String("tag")
	previous := c.String("previous")
	repo, err := setupNginxRemotes(ghUser)
	if err != nil {
		return err
	}

	if tag == "" {
		tag, err = rgit.GetLatestTagFromRepo(repo, "controller")
		if err != nil {
			return err
		}
		logrus.Info("found latest controller tag: ", tag)
	}

	newBranch, err := switchToHardenedBranch(repo, ghUser, tag, previous)
	if err != nil {
		return err
	}
	if newBranch != "" {
		logrus.Infof("No remote branch %s found, creating new branch based of tag %s", newBranch, tag)
		if err := rgit.CreateNewBranchFromTag(nginxDir, newBranch, tag); err != nil {
			return err
		}
		if previous != "" {
			return cherryPickHardened(repo, ghUser, previous)
		}
		hardenedBranch := getLastHardenedBranch(tag)
		return cherryPickHardened(repo, ghUser, hardenedBranch)
	}
	return rebaseUpstream(repo, tag)
}

// setupNginxRemotes will clone the k8s ingress-nginx upstream repo and
// set up remotes for rancher and user's forks, then it will fetch branches and tags from all remotes
func setupNginxRemotes(user string) (*git.Repository, error) {
	// clone the upstream repo
	_, err := os.Stat(nginxDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(nginxDir, 0755); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	// clone the repo
	repo, err := git.PlainClone(nginxDir, false, &git.CloneOptions{
		URL:             strings.Replace(nginxURL, "ORG", "kubernetes", 1),
		Progress:        os.Stdout,
		InsecureSkipTLS: true,
	})
	if err != nil {
		if err == git.ErrRepositoryAlreadyExists {
			repo, err = git.PlainOpen(nginxDir)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	// set up the rancher and user remotes
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "rancher",
		URLs: []string{strings.Replace(nginxURL, "ORG", "rancher", 1)},
	}); err != nil {
		if err != git.ErrRemoteExists {
			return nil, fmt.Errorf("unable to add rancher remote: %v", err)
		}
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: user,
		URLs: []string{strings.Replace(nginxURL, "ORG", user, 1)},
	}); err != nil {
		if err != git.ErrRemoteExists {
			return nil, fmt.Errorf("unable to add user remote: %v", err)
		}
	}
	// fetch branches and tags from all remotes
	if err := repo.Fetch(&git.FetchOptions{
		RemoteName:      "origin",
		Progress:        os.Stdout,
		Tags:            git.AllTags,
		InsecureSkipTLS: true,
	}); err != nil {
		if err != git.NoErrAlreadyUpToDate {
			return nil, fmt.Errorf("unable to fetch upstream remote: %v", err)
		}
	}
	if err := repo.Fetch(&git.FetchOptions{
		RemoteName:      "rancher",
		Progress:        os.Stdout,
		Tags:            git.AllTags,
		InsecureSkipTLS: true,
	}); err != nil {
		if err != git.NoErrAlreadyUpToDate {
			return nil, fmt.Errorf("unable to fetch rancher remote: %v", err)
		}
	}
	if err := repo.Fetch(&git.FetchOptions{
		RemoteName:      user,
		Progress:        os.Stdout,
		Tags:            git.AllTags,
		InsecureSkipTLS: true,
	}); err != nil {
		if err != git.NoErrAlreadyUpToDate {
			return nil, fmt.Errorf("unable to fetch user remote: %v", err)
		}
	}
	// Using git library, we must manually setup the username and email
	rgit.RunCommandInDir(nginxDir, "git", "config", "user.name", user)
	email, err := rgit.RunCommandInDir(nginxDir, "git", "config", "-f", os.Getenv("HOME")+"/.gitconfig", "--get", "user.email")
	if err != nil {
		return nil, fmt.Errorf("help %v", err)
	}
	if _, err := rgit.RunCommandInDir(nginxDir, "git", "config", "user.email", email); err != nil {
		return nil, err
	}
	return repo, nil
}

func switchToHardenedBranch(repo *git.Repository, user, tag, previous string) (string, error) {
	rgit.CleanRepo(nginxDir)
	// Switch to the latest "hardened-nginx-vX.Y.Z-fix" branch based on the upstream tag
	// upstream:controller-v1.9.3 ==> rancher:hardened-nginx-v1.9.x-fix
	branchName := getHardenedBranch(tag)
	// If the hardened branch doesn't exist, pull down the previous hardened branch instead
	// and switch back to main
	ref := plumbing.NewBranchReferenceName(branchName)
	if err := repo.Storer.RemoveReference(ref); err != nil {
		return "", err
	}
	if err := rgit.CheckoutRemoteBranch(nginxDir, user, branchName); err != nil {
		if err != nil && err != rgit.ErrRemotebranchNotFound {
			return "", err
		}
		var latestHardenedBranch string
		if previous != "" {
			latestHardenedBranch = previous
		} else {
			latestHardenedBranch = getLastHardenedBranch(tag)
		}
		if err := rgit.CheckoutRemoteBranch(nginxDir, user, latestHardenedBranch); err != nil {
			return "", fmt.Errorf("unable to find %s: %v", latestHardenedBranch, err)
		}
		return branchName, nil
	}
	logrus.Info("switching to branch: ", branchName)
	return "", nil
}

// rebaseUpstream will rebase the latest upstream tag onto the user's fork and deal with conflicts
func rebaseUpstream(repo *git.Repository, tag string) error {
	_, err := rgit.RunCommandInDir(nginxDir, "git", "rebase", "--onto", tag, "-Xtheirs", fmt.Sprintf("HEAD~%d", len(hardenedCommits)))
	if err != nil && strings.Contains(err.Error(), hardenedCommits[0]) {
		if err2 := rgit.RemoveFiles(nginxDir, []string{".github/workflows/ci.yaml", ".github/workflows/depreview.yaml"}); err2 != nil {
			return err2
		}
	} else if err != nil {
		return err
	}
	_, err = rgit.RunCommandInDir(nginxDir, "git", "rebase", "--continue")
	if err != nil && strings.Contains(err.Error(), hardenedCommits[2]) {
		if err2 := rgit.RemoveFiles(nginxDir, []string{"images/nginx/rootfs/Dockerfile"}); err2 != nil {
			return err2
		}
	} else if err != nil {
		return err
	}
	if _, err2 := rgit.RunCommandInDir(nginxDir, "git", "rebase", "--continue"); err2 != nil {
		return err2
	}
	logrus.Infof("Successfully rebase tag %s onto %s branch, use the 'create-pr' command to open a PR to rancher", tag, getHardenedBranch(tag))
	return nil
}

// cherryPickHardened will cherry-pick the hardened-nginx commits on top of the new branch
func cherryPickHardened(repo *git.Repository, user, tag string) error {
	// Cherry-pick the latest hardened-nginx commits onto the new branch
	for i := len(hardenedCommits) - 1; i >= 0; i-- {
		commit := fmt.Sprintf("%s~%d", tag, i)
		_, err := rgit.RunCommandInDir(nginxDir, "git", "cherry-pick", "-Xtheirs", commit)
		if err != nil {
			if strings.Contains(err.Error(), hardenedCommits[0]) {
				if err2 := rgit.RemoveFiles(nginxDir, []string{".github/workflows/ci.yaml", ".github/workflows/depreview.yaml"}); err2 != nil {
					return err2
				}
				if out, err2 := rgit.RunCommandInDir(nginxDir, "git", "cherry-pick", "--continue"); err2 != nil {
					return fmt.Errorf("unable to cherry-pick: %s: %v", out, err2)
				}
			} else if strings.Contains(err.Error(), hardenedCommits[2]) {
				if err2 := rgit.RemoveFiles(nginxDir, []string{"images/nginx/rootfs/Dockerfile"}); err2 != nil {
					return err2
				}
				if out, err2 := rgit.RunCommandInDir(nginxDir, "git", "cherry-pick", "--continue"); err2 != nil {
					return fmt.Errorf("unable to cherry-pick: %s: %v", out, err2)
				}
			} else if strings.Contains(err.Error(), hardenedCommits[3]) {
				if out, err2 := rgit.RunCommandInDir(nginxDir, "git", "add", "Dockerfile.dapper"); err2 != nil {
					return fmt.Errorf("unable to cherry-pick: %s: %v", out, err2)
				}
				if out, err2 := rgit.RunCommandInDir(nginxDir, "git", "cherry-pick", "--continue"); err2 != nil {
					return fmt.Errorf("unable to cherry-pick: %s: %v", out, err2)
				}
			} else {
				return err
			}
		}
	}
	logrus.Infof("Successfully cherry-picked hardened commits, use the 'push-rancher' command")

	return nil
}

func getHardenedBranch(tag string) string {
	re := regexp.MustCompile(`v(\d+).(\d+).\d+`)
	groups := re.FindStringSubmatch(tag)
	version := groups[0]
	major := groups[1]
	minor := groups[2]
	if version == "" {
		return ""
	}
	return fmt.Sprintf("hardened-nginx-%s.%s.x-fix", major, minor)
}

func getLastHardenedBranch(tag string) string {
	re := regexp.MustCompile(`v(\d+).(\d+).\d+`)
	groups := re.FindStringSubmatch(tag)
	major := groups[1]
	minor := groups[2]
	minorInt, err := strconv.Atoi(minor)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("hardened-nginx-%s.%d.x-fix", major, minorInt-1)
}

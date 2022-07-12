package main

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func PushUser(c *cli.Context) error {
	if err := checkFlags(c); err != nil {
		return err
	}
	ghRemote := c.String("user")
	ghToken := c.String("token")
	if err := pushHardenedBranch(ghRemote, ghToken, true); err != nil {
		return err
	}
	return nil
}

func PushRancher(c *cli.Context) error {
	if err := checkFlags(c); err != nil {
		return err
	}
	ghToken := c.String("token")
	if err := pushHardenedBranch("rancher", ghToken, false); err != nil {
		return err
	}
	return nil
}

func pushHardenedBranch(remote, token string, force bool) error {
	repo, err := git.PlainOpen(nginxDir)
	if err != nil {
		return err
	}
	r, err := repo.Head()
	if err != nil {
		return err
	}

	if err := repo.Push(&git.PushOptions{
		RemoteName: remote,
		Auth: &http.BasicAuth{
			Username: "placeholder", // it just can't be blank
			Password: token,
		},
		Force: force,
	}); err != nil {
		return err
	}
	logrus.Info("new branch pushed to https://github.com/", remote, "/ingress-nginx/tree/", r.Name().Short())
	return nil
}

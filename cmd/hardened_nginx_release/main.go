package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	tokenFlag = &cli.StringFlag{
		Name:   "token, t",
		Usage:  "GitHub read, workflow access token",
		EnvVar: "GITHUB_TOKEN",
	}
	userFlag = &cli.StringFlag{
		Name:   "user, u",
		Usage:  "GitHub username",
		EnvVar: "USER",
	}
	debugFlag = &cli.BoolFlag{
		Name:  "debug,d ",
		Usage: "debug mode",
	}
	rootFlags = []cli.Flag{
		tokenFlag,
		userFlag,
		debugFlag,
	}
	rebaseFlags = []cli.Flag{
		&cli.StringFlag{
			Name:  "tag",
			Usage: "upstream tag to rebase onto",
		},
		userFlag,
		debugFlag,
	}
)

func main() {
	app := cli.NewApp()
	app.Name = "hardened-nginx-release"
	app.Usage = "Start a k3s release"
	app.Commands = []cli.Command{
		{
			Name:   "rebase",
			Usage:  "Attempt to rebase hardened changes onto a new ingress-nginx release",
			Flags:  rebaseFlags,
			Action: Rebase,
		},
		{
			Name:   "push-user",
			Usage:  "Push a hardened-nginx patch to user fork for PR",
			Flags:  rootFlags,
			Action: PushUser,
		},
		{
			Name:   "push-rancher",
			Usage:  "Push a new hardened-nginx branch to rancher",
			Flags:  rootFlags,
			Action: PushRancher,
		},
	}
	app.Flags = rootFlags
	err := app.Run(os.Args)
	if err != nil {
		logrus.Fatal(err)
	}

}

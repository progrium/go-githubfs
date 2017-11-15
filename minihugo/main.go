package main

import (
	"context"
	"log"
	"os"

	"golang.org/x/oauth2"

	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"
	"github.com/google/go-github/github"
	"github.com/spf13/afero"

	"github.com/progrium/go-githubfs"
)

func InitializeConfig() (*deps.DepsCfg, error) {

	var cfg *deps.DepsCfg = &deps.DepsCfg{}

	// Init file systems. This may be changed at a later point.
	osFs := hugofs.Os

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	fs, err := githubfs.NewGitHubFs(client, "progrium", "go-githubfs", "demo")
	if err != nil {
		panic(err)
	}

	config, err := hugolib.LoadConfig(fs, "/", "/config.toml")
	if err != nil {
		return cfg, err
	}

	// Init file systems. This may be changed at a later point.
	cfg.Cfg = config

	config.Set("publishDir", "/tmp/public")

	config.Set("workingDir", "/")

	cfg.Fs = hugofs.NewFrom(osFs, config)

	cfg.Fs.Source = fs
	cfg.Fs.WorkingDir = afero.NewBasePathFs(afero.NewReadOnlyFs(fs), "/").(*afero.BasePathFs)

	return cfg, nil

}

func main() {
	cfg, err := InitializeConfig()
	if err != nil {
		log.Fatal(err)
	}
	h, err := hugolib.NewHugoSites(*cfg)
	if err != nil {
		log.Fatal(err)
	}
	err = h.Build(hugolib.BuildCfg{CreateSitesFromConfig: true, Watching: false, PrintStats: false})
	if err != nil {
		log.Fatal(err)
	}
}

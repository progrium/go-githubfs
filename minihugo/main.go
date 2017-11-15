package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/gohugoio/hugo/deps"
	"github.com/gohugoio/hugo/hugofs"
	"github.com/gohugoio/hugo/hugolib"

  "github.com/progrium/go-githubfs"
)

func InitializeConfig() (*deps.DepsCfg, error) {

	var cfg *deps.DepsCfg = &deps.DepsCfg{}

	// Init file systems. This may be changed at a later point.
	osFs := hugofs.Os

	config, err := hugolib.LoadConfig(osFs, os.Getenv("HUGO_SOURCE"), os.Getenv("HUGO_CONFIG"))
	if err != nil {
		return cfg, err
	}

	// Init file systems. This may be changed at a later point.
	cfg.Cfg = config


	config.Set("publishDir", "public")

	var dir string
	if os.Getenv("HUGO_SOURCE") != "" {
		dir, _ = filepath.Abs(os.Getenv("HUGO_SOURCE"))
	} else {
		dir, _ = os.Getwd()
	}
	config.Set("workingDir", dir)

  cfg.Fs = hugofs.NewFrom(osFs, config)
  cfg.Fs.Source =

	return cfg, nil

}

func main() {
	cfg, err := InitializeConfig()
	h, err := hugolib.NewHugoSites(*cfg)
	if err != nil {
		log.Fatal(err)
	}
	err = h.Build(hugolib.BuildCfg{CreateSitesFromConfig: true, Watching: false, PrintStats: false})
	if err != nil {
		log.Fatal(err)
	}
}

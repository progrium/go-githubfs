package main

import (
	"context"
	"log"
	"os"

	"github.com/google/go-github/github"
	"github.com/progrium/go-githubfs"
	"golang.org/x/oauth2"
)

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	fs, err := githubfs.NewGitHubFs(client, "progrium", "go-githubfs", "master")
	if err != nil {
		panic(err)
	}

	f, err := fs.OpenFile("test/baz2", os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	f.Write([]byte("Hello world....\n"))

	err = f.Close()
	if err != nil {
		log.Fatal(err)
	}
	//fmt.Printf("%# v", pretty.Formatter(fs))
}

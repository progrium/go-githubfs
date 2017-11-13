package main

import (
  "context"
  "os"
  "fmt"
  //"encoding/base64"
  
  "github.com/google/go-github/github"
  "golang.org/x/oauth2"
  "github.com/kr/pretty"
)

func main() {
  ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	// list all repositories for the authenticated user
	//repo, _, _ := client.Repositories.Get(ctx, "progrium", "go-githubfs")
  branch, _, _ := client.Repositories.GetBranch(ctx, "progrium", "go-githubfs", "master")
  treeHash := branch.Commit.Commit.Tree.GetSHA()
  tree, _, _ := client.Git.GetTree(ctx, "progrium", "go-githubfs", treeHash, true)
  //blob, _, _ := client.Git.GetBlob(ctx, "progrium", "go-githubfs", tree.Entries[0].GetSHA())
  //_, _ := base64.StdEncoding.DecodeString(blob.GetContent())
  fmt.Printf("%# v", pretty.Formatter(tree))
}
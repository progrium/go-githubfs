package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	//"encoding/base64"

	"github.com/google/go-github/github"
	"github.com/kr/pretty"
	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"golang.org/x/oauth2"
)

func CreateFile(name string) *mem.File {
	fileData := mem.CreateFile(name)
	file := mem.NewFileHandle(fileData)
	return file
}

type githubFs struct {
	client *github.Client
	user   string
	repo   string
	branch string
	tree   *github.Tree
	mu     sync.Mutex
}

func NewGitHubFs(client *github.Client, user string, repo string, branch string) (afero.Fs, error) {
	ghfs := &githubFs{
		client: client,
		user:   user,
		repo:   repo,
		branch: branch,
	}
	ctx := context.Background()
	b, _, err := client.Repositories.GetBranch(ctx, user, repo, branch)
	if err != nil {
		return nil, err
	}
	treeHash := b.Commit.Commit.Tree.GetSHA()
	ghfs.tree, _, _ = client.Git.GetTree(ctx, user, repo, treeHash, true)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("%# v", pretty.Formatter(ghfs.tree))
	return ghfs, nil
}

// Create creates a file in the filesystem, returning the file and an
// error, if any happens.
func (fs *githubFs) Create(name string) (afero.File, error) {
	return CreateFile(name), nil
}

// Mkdir creates a directory in the filesystem, return an error if any
// happens.
func (fs *githubFs) Mkdir(name string, perm os.FileMode) error {
	dir := mem.CreateDir(name)
	mem.SetMode(dir, perm)
	return nil
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func (fs *githubFs) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

// Open opens a file, returning it or an error, if any happens.
func (fs *githubFs) Open(name string) (afero.File, error) {
	dir := mem.CreateDir(name)
	normalDir := strings.TrimPrefix(name, "/")
	if normalDir == "" {
		normalDir = "."
	}
	for _, e := range fs.tree.Entries {
		if path.Dir(e.GetPath()) != normalDir {
			continue
		}
		normalName := strings.TrimPrefix(e.GetPath(), path.Dir(e.GetPath())+"/")
		switch e.GetType() {
		case "blob":
			f := mem.CreateFile(normalName)
			mem.SetMode(f, os.FileMode(int(0644)))
			mem.AddToMemDir(dir, f)
		case "tree":
			d := mem.CreateDir(normalName)
			mem.SetMode(d, os.FileMode(int(040000)))
			mem.AddToMemDir(dir, d)
		default:
			continue
		}
	}

	return mem.NewFileHandle(dir), nil
}

// OpenFile opens a file using the given flags and the given mode.
func (fs *githubFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return nil, nil
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func (fs *githubFs) Remove(name string) error {
	return nil
}

// RemoveAll removes a directory path and any children it contains. It
// does not fail if the path does not exist (return nil).
func (fs *githubFs) RemoveAll(path string) error {
	return nil
}

// Rename renames a file.
func (fs *githubFs) Rename(oldname, newname string) error {
	return nil
}

// Stat returns a FileInfo describing the named file, or an error, if any
// happens.
func (fs *githubFs) Stat(name string) (os.FileInfo, error) {
	return nil, nil
}

// The name of this FileSystem
func (fs *githubFs) Name() string {
	return "github-api"
}

//Chmod changes the mode of the named file to mode.
func (fs *githubFs) Chmod(name string, mode os.FileMode) error {
	return nil
}

//Chtimes changes the access and modification times of the named file
func (fs *githubFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	// no-op
	return nil
}

func main() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_ACCESS_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)

	fs, err := NewGitHubFs(client, "progrium", "go-githubfs", "master")
	if err != nil {
		panic(err)
	}

	//blob, _, _ := client.Git.GetBlob(ctx, "progrium", "go-githubfs", tree.Entries[0].GetSHA())
	//_, _ := base64.StdEncoding.DecodeString(blob.GetContent())
	info, err := afero.ReadDir(fs, "/test/foo")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%# v", pretty.Formatter(info))
}

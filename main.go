package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/kr/pretty"
	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"golang.org/x/oauth2"
)

const CommitMessage = "automatic commit from githubfs 🎆"

func String(s string) *string {
	return &s
}

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
	err = ghfs.updateTree(b.Commit.Commit.Tree.GetSHA())
	if err != nil {
		return nil, err
	}
	//fmt.Printf("%# v", pretty.Formatter(ghfs.tree))
	return ghfs, nil
}

func (fs *githubFs) updateTree(sha string) (err error) {
	fs.tree, _, err = fs.client.Git.GetTree(context.TODO(), fs.user, fs.repo, sha, true)
	return
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

func (fs *githubFs) findEntry(name string) *github.TreeEntry {
	normalName := strings.TrimPrefix(name, "/")
	for _, e := range fs.tree.Entries {
		if e.GetPath() == normalName {
			return &e
		}
	}
	return nil
}

// Open opens a file, returning it or an error, if any happens.
func (fs *githubFs) Open(name string) (afero.File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimPrefix(name, "/")
	entry := fs.findEntry(name)
	if entry == nil {
		return nil, afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		// if file
		fd := mem.CreateFile(name)
		mem.SetMode(fd, os.FileMode(int(0644)))
		f := mem.NewFileHandle(fd)
		blob, _, err := fs.client.Git.GetBlob(context.TODO(), fs.user, fs.repo, entry.GetSHA())
		if err != nil {
			return nil, err
		}
		b, _ := base64.StdEncoding.DecodeString(blob.GetContent())
		f.Write(b)
		f.Seek(0, 0)
		return f, nil
	}
	// else if tree/dir
	dir := mem.CreateDir(name)
	if normalName == "" {
		normalName = "."
	}
	for _, e := range fs.tree.Entries {
		if path.Dir(e.GetPath()) != normalName {
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
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.remove(name)
}

func (fs *githubFs) remove(name string) error {
	normalName := strings.TrimPrefix(name, "/")
	entry := fs.findEntry(name)
	if entry == nil {
		return afero.ErrFileNotFound
	}
	resp, _, err := fs.client.Repositories.DeleteFile(context.TODO(), fs.user, fs.repo, normalName, &github.RepositoryContentFileOptions{
		Message: String(CommitMessage),
		SHA:     String(entry.GetSHA()),
		Branch:  String(fs.branch),
	})
	if err != nil {
		return err
	}
	return fs.updateTree(resp.Tree.GetSHA())
}

// RemoveAll removes a directory path and any children it contains. It
// does not fail if the path does not exist (return nil).
func (fs *githubFs) RemoveAll(path string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimPrefix(path, "/")
	entry := fs.findEntry(path)
	if entry == nil {
		return afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		return fs.remove(path)
	}
	// TODO: remove all files in a single commit
	normalName = strings.TrimSuffix(normalName, "/") + "/"
	for _, e := range fs.tree.Entries {
		if strings.HasPrefix(e.GetPath(), normalName) {
			err := fs.remove(e.GetPath())
			if err != nil {
				return err
			}
		}
	}
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
	// info, err := afero.ReadDir(fs, "/test/foo")
	// if err != nil {
	// 	panic(err)
	// }
	err = fs.RemoveAll("/test/foo")
	// data, _ := afero.ReadFile(fs, "/test/file1")
	// os.Stdout.Write(data)
	fmt.Printf("%# v", pretty.Formatter(err))
}

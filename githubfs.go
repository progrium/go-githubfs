package githubfs

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/spf13/afero"
)

const CommitMessage = "automatic commit from githubfs 🎆"

func String(s string) *string {
	return &s
}

type githubFs struct {
	client *github.Client
	user   string
	repo   string
	branch *github.Branch
	tree   *github.Tree
	mu     sync.Mutex
}

func NewGitHubFs(client *github.Client, user string, repo string, branch string) (afero.Fs, error) {
	fs := &githubFs{
		client: client,
		user:   user,
		repo:   repo,
	}
	ctx := context.Background()
	var err error
	fs.branch, _, err = client.Repositories.GetBranch(ctx, user, repo, branch)
	if err != nil {
		return nil, err
	}
	err = fs.updateTree(fs.branch.Commit.Commit.Tree.GetSHA())
	if err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *githubFs) updateTree(sha string) (err error) {
	fs.tree, _, err = fs.client.Git.GetTree(context.TODO(), fs.user, fs.repo, sha, true)
	return
}

// Create creates a file in the filesystem, returning the file and an
// error, if any happens.
func (fs *githubFs) Create(name string) (afero.File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimPrefix(name, "/")
	if normalName == "" {
		return nil, os.ErrInvalid
	}
	e := fs.findEntry(normalName)
	if e != nil {
		return nil, afero.ErrFileExists
	}
	var parent *github.TreeEntry
	if strings.Contains(normalName, FilePathSeparator) {
		parent = fs.findEntry(filepath.Dir(normalName))
		if parent == nil {
			return nil, os.ErrNotExist
		}
	}
	blob, _, err := fs.client.Git.CreateBlob(context.TODO(), fs.user, fs.repo, &github.Blob{
		Content: String(""),
	})
	if err != nil {
		return nil, err
	}
	entry := github.TreeEntry{
		Type: String("blob"),
		Mode: String("100644"),
		Path: String(normalName),
		SHA:  blob.SHA,
	}
	fs.tree.Entries = append(fs.tree.Entries, entry)
	if parent != nil {
		err = fs.createTreesFromEntries(parent.GetPath(), false)
		if err != nil {
			return nil, err
		}
	}
	err = fs.commit()
	if err != nil {
		return nil, err
	}

	// TODO: add necessary references
	fileData := CreateFile(name)
	file := NewFileHandle(fileData, fs, entry)

	return file, nil
}

func (fs *githubFs) createTreesFromEntries(path string, force bool) error {
	entry := fs.findEntry(path)
	if entry == nil {
		return fmt.Errorf("entry not found for path '%s'", path)
	}
	if entry.SHA == nil || force {
		var children []github.TreeEntry
		for _, e := range fs.tree.Entries {
			if strings.HasPrefix(e.GetPath(), path+"/") {
				relativeName := strings.TrimPrefix(e.GetPath(), path+"/")
				if !strings.Contains(relativeName, FilePathSeparator) {
					relativeEntry := e
					relativeEntry.Path = String(relativeName)
					children = append(children, relativeEntry)
				}
			}
		}
		tree, _, err := fs.client.Git.CreateTree(context.TODO(), fs.user, fs.repo, "", children)
		if err != nil {
			return err
		}
		for i, e := range fs.tree.Entries {
			if e.GetPath() == entry.GetPath() {
				fs.tree.Entries[i].SHA = tree.SHA
			}
		}
	}
	parentDir := filepath.Dir(path)
	if parentDir == "." || parentDir == "" {
		return nil
	}
	return fs.createTreesFromEntries(parentDir, force)
}

// Mkdir creates a directory in the filesystem, return an error if any
// happens.
func (fs *githubFs) Mkdir(name string, perm os.FileMode) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimPrefix(name, "/")
	if strings.Contains(normalName, FilePathSeparator) {
		if p := fs.findEntry(filepath.Dir(normalName)); p == nil {
			return afero.ErrFileNotFound // parent path does not exist
		}
	}
	fs.tree.Entries = append(fs.tree.Entries, github.TreeEntry{
		Type: String("tree"),
		Mode: String("040000"),
		Path: String(normalName),
	})
	return nil
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func (fs *githubFs) MkdirAll(path string, perm os.FileMode) error {
	normalName := strings.TrimPrefix(path, "/")
	parentNames := strings.Split(filepath.Dir(normalName), FilePathSeparator)
	if len(parentNames) == 0 {
		return fs.Mkdir(path, perm)
	}
	for i, _ := range parentNames {
		fs.mu.Lock()
		parentPath := strings.Join(parentNames[0:i+1], FilePathSeparator)
		parent := fs.findEntry(parentPath)
		fs.mu.Unlock()
		if parent == nil {
			err := fs.Mkdir(parentPath, perm)
			if err != nil {
				return err
			}
		}
	}
	return fs.Mkdir(path, perm)
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

func (fs *githubFs) open(name string) (afero.File, *FileData, error) {
	normalName := strings.TrimPrefix(name, "/")
	entry := fs.findEntry(name)
	if entry == nil {
		return nil, nil, afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		// if file
		fd := CreateFile(name)
		SetMode(fd, os.FileMode(int(0644)))
		blob, _, err := fs.client.Git.GetBlob(context.TODO(), fs.user, fs.repo, entry.GetSHA())
		if err != nil {
			return nil, nil, err
		}
		fd.data, _ = base64.StdEncoding.DecodeString(blob.GetContent())
		return NewFileHandle(fd, fs, *entry), fd, nil
	}
	// else if tree/dir
	dir := CreateDir(name)
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
			f := CreateFile(normalName)
			SetMode(f, os.FileMode(int(0644)))
			AddToMemDir(dir, f)
		case "tree":
			d := CreateDir(normalName)
			SetMode(d, os.FileMode(int(040000)))
			AddToMemDir(dir, d)
		default:
			continue
		}
	}
	return NewFileHandle(dir, fs, github.TreeEntry{Type: String("tree")}), dir, nil
}

// Open opens a file, returning it or an error, if any happens.
func (fs *githubFs) Open(name string) (afero.File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	f, _, err := fs.open(name)
	return f, err
}

// OpenFile opens a file using the given flags and the given mode.
func (fs *githubFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	fs.mu.Lock()
	_, fd, err := fs.open(name)
	fs.mu.Unlock()
	if err == afero.ErrFileNotFound && flag&os.O_CREATE != 0 {
		return fs.Create(name)
	}
	entry := fs.findEntry(name)
	if fd != nil && entry != nil {
		SetMode(fd, perm)
		file := NewFileHandle(fd, fs, *entry)
		if flag&os.O_APPEND > 0 {
			_, err := file.Seek(0, os.SEEK_END)
			if err != nil {
				file.Close()
				return nil, err
			}
		}
		return file, nil
	}
	return nil, err
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
		Branch:  String(fs.branch.GetName()),
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
	normalName := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")
	entry := fs.findEntry(normalName)
	if entry == nil {
		return afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		return fs.remove(path)
	}
	// TODO: remove all files in a single commit
	for _, e := range fs.tree.Entries {
		if e.GetType() == "tree" {
			continue
		}
		if strings.HasPrefix(e.GetPath(), normalName+"/") {
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
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalOld := strings.TrimPrefix(oldname, "/")
	normalNew := strings.TrimPrefix(newname, "/")
	for i, e := range fs.tree.Entries {
		if e.GetPath() == normalOld {
			fs.tree.Entries[i].Path = String(normalNew)
		}
	}
	return fs.commit()
}

func (fs *githubFs) updateBranch() (err error) {
	fs.branch, _, err = fs.client.Repositories.GetBranch(context.TODO(), fs.user, fs.repo, fs.branch.GetName())
	return
}

func (fs *githubFs) commit() error {
	// TODO: can we do this with less requests?
	branch, _, err := fs.client.Repositories.GetBranch(context.TODO(), fs.user, fs.repo, fs.branch.GetName())
	if err != nil {
		return err
	}
	// TODO: can we remove this constraint? make it more like user workflow?
	if branch.GetCommit().GetSHA() != fs.branch.GetCommit().GetSHA() {
		return errors.New("commits have been made since last filesystem operation")
	}
	fs.branch = branch

	tree, _, err := fs.client.Git.CreateTree(context.TODO(), fs.user, fs.repo, "", fs.tree.Entries)
	if err != nil {
		return err
	}
	err = fs.updateTree(tree.GetSHA())
	if err != nil {
		return err
	}

	commit, _, err := fs.client.Git.CreateCommit(context.TODO(), fs.user, fs.repo, &github.Commit{
		Message: String(CommitMessage),
		Tree:    fs.tree,
		Parents: []github.Commit{{SHA: fs.branch.GetCommit().SHA}},
	})
	if err != nil {
		return err
	}
	_, _, err = fs.client.Git.UpdateRef(context.TODO(), fs.user, fs.repo, &github.Reference{
		Ref: String("heads/" + fs.branch.GetName()),
		Object: &github.GitObject{
			SHA: commit.SHA,
		},
	}, false)
	if err != nil {
		return err
	}
	return fs.updateBranch()
}

// Stat returns a FileInfo describing the named file, or an error, if any
// happens.
func (fs *githubFs) Stat(name string) (os.FileInfo, error) {
	// TODO: properly
	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

// The name of this FileSystem
func (fs *githubFs) Name() string {
	return "github-api"
}

//Chmod changes the mode of the named file to mode.
func (fs *githubFs) Chmod(name string, mode os.FileMode) error {
	// TODO: NOT YET IMPLEMENTED
	return nil
}

//Chtimes changes the access and modification times of the named file
func (fs *githubFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	// no-op
	return nil
}

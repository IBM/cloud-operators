package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pkg/errors"
)

type Maintainer struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	contributions uint64
}

func getMaintainers(repoRoot string) ([]Maintainer, error) {
	fmt.Fprintln(os.Stderr, "Generating maintainers...")
	start := time.Now()
	defer func() {
		fmt.Fprintln(os.Stderr, "Done.", time.Since(start))
	}()

	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open repo at %s", repoRoot)
	}

	commitIter, err := repo.Log(&git.LogOptions{
		Order: git.LogOrderBSF,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get commit log")
	}

	var commits []*object.Commit
	const maxCommits = 100
	for i := 0; i < maxCommits; i++ {
		commit, err := commitIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error processing next commit:", err)
			continue
		}
		commits = append(commits, commit)
	}
	commitIter.Close()

	maintainersChan := make(chan Maintainer)
	errsChan := make(chan error)
	var wg sync.WaitGroup
	for _, commit := range commits {
		wg.Add(1)
		hash := commit.Hash
		go func() {
			defer wg.Done()
			maintainer, err := NewFromCommit(repoRoot, hash)
			if err != nil {
				errsChan <- err
				return
			}
			maintainersChan <- maintainer
		}()
	}
	go func() {
		wg.Wait()
		close(maintainersChan)
	}()

	uniqueEmails := make(map[string]*Maintainer)
	for {
		select {
		case m, ok := <-maintainersChan:
			if !ok {
				return topContributors(uniqueEmails), nil
			}
			_, exists := uniqueEmails[m.Email]
			if !exists {
				mCopy := m
				uniqueEmails[m.Email] = &mCopy
			} else {
				uniqueEmails[m.Email].contributions += m.contributions
			}
		case err := <-errsChan:
			return nil, errors.Wrap(err, "Failed to fetch contributor info from git repo")
		}
	}
}

func topContributors(uniqueEmails map[string]*Maintainer) []Maintainer {
	var maintainers []Maintainer
	for _, m := range uniqueEmails {
		maintainers = append(maintainers, *m)
	}

	sort.Slice(maintainers, func(a, b int) bool {
		return maintainers[a].contributions >= maintainers[b].contributions
	})

	const maxMaintainers = 5
	if len(maintainers) < maxMaintainers {
		return maintainers
	}
	return maintainers[:maxMaintainers]
}

// NewFromCommit generates a Maintainer from the given commit hash.
// NOTE: This is very slow when processing large commits.
func NewFromCommit(repoRoot string, hash plumbing.Hash) (Maintainer, error) {
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return Maintainer{}, errors.Wrapf(err, "Failed to get repo at %q", repoRoot)
	}
	commit, err := repo.CommitObject(hash)
	if err != nil {
		return Maintainer{}, errors.Wrapf(err, "Failed to get commit for hash %s", hash.String())
	}
	maintainer := Maintainer{
		Name:  commit.Author.Name,
		Email: commit.Author.Email,
	}
	stats, err := commit.Stats()
	if err != nil {
		return Maintainer{}, errors.Wrapf(err, "Failed to get stats for hash %s", hash.String())
	}
	const commitWeight = 100
	for _, stat := range stats {
		maintainer.contributions += commitWeight
		maintainer.contributions += uint64(stat.Addition)
		maintainer.contributions += uint64(stat.Deletion)
	}
	return maintainer, nil
}

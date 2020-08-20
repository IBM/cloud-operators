package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/go-git/go-git/v5"
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
		return nil, err
	}
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	commitIter, err := repo.Log(&git.LogOptions{
		From: head.Hash(),
	})
	if err != nil {
		return nil, err
	}

	var maintainers []*Maintainer
	uniqueEmails := make(map[string]*Maintainer)
	const maxCommits = 200
	for i := 0; i < maxCommits; i++ {
		commit, err := commitIter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		maintainer := &Maintainer{Name: commit.Author.Name, Email: commit.Author.Email}
		if m, ok := uniqueEmails[maintainer.Email]; ok {
			stats, err := commit.Stats()
			if err != nil {
				return nil, err
			}
			const commitWeight = 100
			for _, stat := range stats {
				m.contributions += commitWeight
				m.contributions += uint64(stat.Addition)
				m.contributions += uint64(stat.Deletion)
			}
		} else {
			maintainers = append(maintainers, maintainer)
			uniqueEmails[maintainer.Email] = maintainer
		}
	}

	var maintainersSlice []Maintainer
	for _, m := range maintainers {
		maintainersSlice = append(maintainersSlice, *m)
	}
	sort.Slice(maintainersSlice, func(a, b int) bool {
		return maintainersSlice[a].contributions >= maintainersSlice[b].contributions
	})

	const maxMaintainers = 5
	return maintainersSlice[:maxMaintainers], nil
}

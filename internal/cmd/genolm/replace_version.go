package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/go-git/go-git/v5"
	"github.com/pkg/errors"
)

func parseGoSemver(name string) (semver.Version, error) {
	if !strings.HasPrefix(name, "v") {
		return semver.Version{}, errors.Errorf("Go semver tag must start with a 'v': %s", name)
	}
	s := strings.TrimPrefix(name, "v")
	return semver.Parse(s)
}

func getReplaceVersion(repoRoot string, newVersion semver.Version) (string, error) {
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return "", err
	}

	tags, err := repo.Tags()
	if err != nil {
		return "", err
	}

	var versions semver.Versions
	for {
		tag, err := tags.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		t, err := parseGoSemver(tag.Name().Short())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		} else {
			versions = append(versions, t)
		}
	}
	sort.Sort(sort.Reverse(versions))
	for _, version := range versions {
		if version.LT(newVersion) {
			return version.String(), nil
		}
	}
	return "", errors.Errorf("No semver tag found that comes before %s: %v", newVersion, versions)
}

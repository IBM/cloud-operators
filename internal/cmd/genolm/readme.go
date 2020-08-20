package main

import (
	"bufio"
	"bytes"
	"io"
	"path"
	"strings"

	"github.com/johnstarich/go/regext"
)

func prepREADME(readme io.Reader) string {
	scanner := bufio.NewScanner(readme)
	include := false
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		line = convertToAbsoluteLinks(line)

		if include {
			if line != "" || buf.Len() > 0 { // skip leading blank lines
				buf.WriteString(line)
				buf.WriteRune('\n')
			}
		} else if strings.HasPrefix(line, "# ") {
			// skip up to and including main header lines
			include = true
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return buf.String()
}

// convertToAbsoluteLinks replaces relative links with absolute links to the repo
func convertToAbsoluteLinks(s string) string {
	type readmeReplacement struct {
		newText    string
		start, end int
	}

	const urlPrefix = "https://github.com/IBM/cloud-operators/blob/master"
	markdownLinkRe := regext.MustCompile(`
		\[
			[^ \] ]*      # link text
		\]
		\(
			( [^ \) ]* )  # capture link URL (capture group index 1)
		\)
	`)

	matches := markdownLinkRe.FindAllStringSubmatchIndex(s, -1)
	var replacements []readmeReplacement
	for _, match := range matches {
		start, end := match[2], match[3]
		linkPath := s[start:end]
		if strings.Contains(linkPath, "://") || strings.HasPrefix(linkPath, "#") {
			// skip absolute URLs or anchor-only URLs
			continue
		}
		linkPath = urlPrefix + path.Join("/", linkPath)
		replacements = append(replacements, readmeReplacement{
			newText: linkPath,
			start:   start,
			end:     end,
		})
	}
	for i := len(replacements) - 1; i >= 0; i-- {
		// replace matches going backward, so indexes don't change
		r := replacements[i]
		s = s[:r.start] + r.newText + s[r.end:]
	}
	return s
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
)

type GitHub struct {
	token string

	doRequest func(*http.Request) (*http.Response, error)
}

func newGitHub(token string) *GitHub {
	return &GitHub{
		token:     token,
		doRequest: http.DefaultClient.Do,
	}
}

func (g *GitHub) request(ctx context.Context, method string, url url.URL, requestBody, responseBody interface{}) error {
	var requestReader io.Reader
	if requestBody != nil {
		requestBytes, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		requestReader = bytes.NewReader(requestBytes)
	}
	url.Scheme = "https"
	url.Host = "github.com"
	req, err := http.NewRequestWithContext(ctx, method, url.String(), requestReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.doRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		responseBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.Errorf("Request failed with %d: %s", resp.StatusCode, string(responseBytes))
	}
	if responseBody != nil {
		return json.NewDecoder(resp.Body).Decode(responseBody)
	}
	return nil
}

type SetFileContentsParams struct {
	Org, Repo      string // Required
	BranchName     string // Required
	FilePath       string // Required. Repo-relative file path.
	Message        string
	OldContentsSHA string // Optional if file is new, required otherwise.
	NewContents    []byte // Required
}

// SetFileContents creates or updates a file on the given branch.
//
// Reference for updating a file (proposing changes via commit) reference: https://docs.github.com/en/rest/reference/repos#create-or-update-file-contents
func (g *GitHub) SetFileContents(ctx context.Context, params SetFileContentsParams) error {
	if params.Message == "" {
		params.Message = "Update " + params.FilePath
	}
	body := struct {
		Message string `json:"message"` // Required. The commit message.
		Content []byte `json:"content"` // Required. The new file content, using Base64 encoding.
		SHA     string `json:"sha"`     // Required if you are updating a file. The blob SHA of the file being replaced.
		Branch  string `json:"branch"`  // The branch name. Default: the repository’s default branch (usually master)
	}{
		Message: params.Message,
		Content: params.NewContents,
		SHA:     params.OldContentsSHA,
		Branch:  params.BranchName,
	}
	return g.request(ctx, http.MethodPut, url.URL{Path: fmt.Sprintf("/repos/%s/%s/contents/%s", params.Org, params.Repo, params.FilePath)}, body, nil)
}

type FileContents struct {
	Type    string // "file", "dir", etc
	Content []byte
	SHA     string
}

func (g *GitHub) GetFileContents(ctx context.Context, org, repo, branchName, repoFilePath string) (FileContents, error) {
	var resp FileContents
	u := url.URL{
		Path: fmt.Sprintf("/repos/%s/%s/contents/%s", org, repo, repoFilePath),
	}
	if branchName != "" {
		u.RawQuery = url.Values{
			"ref": []string{branchName},
		}.Encode()
	}
	err := g.request(ctx, http.MethodGet, u, nil, &resp)
	return resp, err
}

func ForkHead(owner, branch string) string {
	return owner + ":" + branch
}

type CreatePullRequestParams struct {
	Org   string
	Repo  string
	Head  string // branch name
	Base  string
	Title string
	Body  string
	Draft bool
}

func (g *GitHub) CreatePullRequest(ctx context.Context, params CreatePullRequestParams) (prURL string, err error) {
	body := struct {
		Title string `json:"title"` // The title of the new pull request.
		Head  string `json:"head"`  // Required. The name of the branch where your changes are implemented. For cross-repository pull requests in the same network, namespace head with a user like this: username:branch.
		Base  string `json:"base"`  // Required. The name of the branch you want the changes pulled into. This should be an existing branch on the current repository. You cannot submit a pull request to one repository that requests a merge to a base of another repository.
		Body  string `json:"body"`  // The contents of the pull request.
		Draft bool   `json:"draft"` // Indicates whether the pull request is a draft.
	}{
		Title: params.Title,
		Head:  params.Head,
		Base:  params.Base,
		Body:  params.Body,
		Draft: params.Draft,
	}
	var resp struct {
		URL string `json:"html_url"`
	}
	err = g.request(ctx, http.MethodPost, url.URL{Path: fmt.Sprintf("/repos/%s/%s/pulls", params.Org, params.Repo)}, body, &resp)
	return resp.URL, err
}

func (g *GitHub) ListPullRequests(ctx context.Context, org, repo, head string) (prURLs []string, err error) {
	u := url.URL{
		Path: fmt.Sprintf("/repos/%s/%s/pulls", org, repo),
		RawQuery: url.Values{
			"head": []string{head},
		}.Encode(),
	}
	var resp []struct {
		URL string `json:"html_url"`
	}
	err = g.request(ctx, http.MethodGet, u, nil, &resp)
	for _, r := range resp {
		prURLs = append(prURLs, r.URL)
	}
	return prURLs, err
}

func (g *GitHub) EnsurePullRequest(ctx context.Context, params CreatePullRequestParams) (prURL string, err error) {
	prs, err := g.ListPullRequests(ctx, params.Org, params.Repo, params.Head)
	if err != nil {
		return "", err
	}
	if len(prs) > 0 {
		return prs[0], nil
	}
	return g.CreatePullRequest(ctx, params)
}

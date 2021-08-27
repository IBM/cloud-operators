// Command release publishes a new release of IBM Cloud Operator.
// Picks up pre-generated output files and creates pull requests in the appropriate repos.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Args struct {
	Version     string
	GitHubToken string
	ForkOrg     string
	CSVFile     string
	PackageFile string
	DraftPRs    bool

	Output io.Writer
}

func main() {
	args := Args{
		Output: os.Stdout,
	}
	flag.StringVar(&args.Version, "version", "", "The release's version to publish.")
	flag.StringVar(&args.GitHubToken, "gh-token", "", "The GitHub token used to open pull requests in OperatorHub repos.")
	flag.StringVar(&args.ForkOrg, "fork-org", "", "The fork org to use for opening PRs on repos of the same name.")
	flag.StringVar(&args.CSVFile, "csv", "", "Path to the OLM cluster service version file. e.g. out/ibmcloud_operator.vX.Y.Z.clusterserviceversion.yaml")
	flag.StringVar(&args.PackageFile, "package", "", "Path to the OLM package file. e.g. out/ibmcloud-operator.package.yaml")
	flag.BoolVar(&args.DraftPRs, "draft", false, "Open PRs as drafts instead of normal PRs.")
	flag.Parse()

	err := run(args, Deps{
		GitHub: newGitHub(args.GitHubToken),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Release failed: %+v\n", err)
		os.Exit(1)
		return
	}
}

const (
	//kubernetesOperatorsOrg  = "k8s-operatorhub"
	kubernetesOperatorsOrg  = "johnstarich"
	kubernetesOperatorsRepo = "community-operators"

	//openshiftOperatorsOrg   = "redhat-openshift-ecosystem"
	openshiftOperatorsOrg  = "johnstarich"
	openshiftOperatorsRepo = "community-operators-prod"

	defaultBranch = "main"
)

type Deps struct {
	GitHub *GitHub
}

func run(args Args, deps Deps) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if args.Version == "" {
		return errors.New("version is required")
	}
	if args.ForkOrg == "" {
		return errors.New("fork org is required")
	}
	if args.GitHubToken == "" {
		return errors.New("GitHub token is required")
	}
	version := "v" + strings.TrimPrefix(args.Version, "v")

	csvContents, err := ioutil.ReadFile(args.CSVFile)
	if err != nil {
		return errors.Wrap(err, "failed to read cluster service version file")
	}
	packageContents, err := ioutil.ReadFile(args.PackageFile)
	if err != nil {
		return errors.Wrap(err, "failed to read package file")
	}

	const timeFormat = "2006-01-02T15-04-05Z"
	branchName := fmt.Sprintf("release-%s-%s", version, time.Now().Format(timeFormat))
	err = setReleaseFiles(ctx, deps.GitHub, kubernetesOperatorsOrg, kubernetesOperatorsRepo, args.ForkOrg, branchName, version, csvContents, packageContents)
	if err != nil {
		return errors.Wrap(err, "failed to update kubernetes operator repo")
	}

	err = setReleaseFiles(ctx, deps.GitHub, openshiftOperatorsOrg, openshiftOperatorsRepo, args.ForkOrg, branchName, version, csvContents, packageContents)
	if err != nil {
		return errors.Wrap(err, "failed to update openshift operator repo")
	}

	kubernetesPR, err := openPR(ctx, deps.GitHub, kubernetesOperatorsOrg, kubernetesOperatorsRepo, args.ForkOrg, branchName, version, args.DraftPRs)

	if err != nil {
		return errors.Wrap(err, "failed to open kubernetes operator PR")
	}
	fmt.Fprintln(args.Output, "Kubernetes PR opened:", kubernetesPR)

	openshiftPR, err := openPR(ctx, deps.GitHub, openshiftOperatorsOrg, openshiftOperatorsRepo, args.ForkOrg, branchName, version, args.DraftPRs)
	if err != nil {
		return errors.Wrap(err, "failed to open openshift operator PR")
	}
	fmt.Fprintln(args.Output, "OpenShift PR opened:", openshiftPR)
	return nil
}

func setReleaseFiles(ctx context.Context, gh *GitHub, org, repo, forkOrg, branchName, version string, csvContents, packageContents []byte) error {
	mainSHA, err := gh.GetRef(ctx, org, repo, BranchRef(defaultBranch))
	if err != nil {
		return err
	}
	err = gh.UpdateRef(ctx, forkOrg, repo, BranchRef(defaultBranch), mainSHA, true)
	if err != nil {
		return err
	}
	err = gh.CreateRef(ctx, forkOrg, repo, BranchRef(branchName), mainSHA)
	if err != nil {
		return err
	}

	trimmedVersion := strings.TrimPrefix(version, "v")
	repoCSVPath := path.Join(
		"operators", "ibmcloud-operator", trimmedVersion,
		fmt.Sprintf("ibmcloud_operator.%s.clusterserviceversion.yaml", version))
	err = gh.SetFileContents(ctx, SetFileContentsParams{
		Org:         forkOrg,
		Repo:        repo,
		BranchName:  branchName,
		FilePath:    repoCSVPath,
		NewContents: csvContents,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set contents of file %q", repoCSVPath)
	}

	packagePath := path.Join("operators", "ibmcloud-operator", "ibmcloud-operator.package.yaml")
	oldPackageFile, err := gh.GetFileContents(ctx, org, repo, "", packagePath)
	if err != nil {
		return errors.Wrapf(err, "failed to get old contents of file %q", packagePath)
	}
	err = gh.SetFileContents(ctx, SetFileContentsParams{
		Org:            forkOrg,
		Repo:           repo,
		BranchName:     branchName,
		FilePath:       packagePath,
		NewContents:    packageContents,
		OldContentsSHA: oldPackageFile.SHA,
	})
	return errors.Wrapf(err, "failed to set contents of file %q", packagePath)
}

func openPR(ctx context.Context, gh *GitHub, org, repo, forkOrg, branchName, version string, draft bool) (string, error) {
	return gh.EnsurePullRequest(ctx, CreatePullRequestParams{
		Org:   org,
		Repo:  repo,
		Head:  ForkHead(forkOrg, branchName),
		Base:  defaultBranch,
		Title: fmt.Sprintf("Update latest release of IBM Cloud Operator: %s", version),
		Body:  fmt.Sprintf("Automated release of IBM Cloud Operator %s.", version),
		Draft: draft,
	})
}

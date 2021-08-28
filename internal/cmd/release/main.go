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
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

type Args struct {
	Version      string
	GitHubToken  string
	ForkOrg      string
	CSVFile      string
	PackageFile  string
	CRDFileGlob  string
	DraftPRs     bool
	GitUserName  string
	GitUserEmail string

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
	flag.StringVar(&args.CRDFileGlob, "crd-glob", "", "Path to the OLM custom resource definition files. e.g. out/apiextensions.k8s.io_v1beta1_customresourcedefinition_*.ibmcloud.ibm.com.yaml")
	flag.BoolVar(&args.DraftPRs, "draft", false, "Open PRs as drafts instead of normal PRs.")
	flag.StringVar(&args.GitUserName, "signoff-name", "", "The Git user name to use when signing off commits.")
	flag.StringVar(&args.GitUserEmail, "signoff-email", "", "The Git email to use when signing off commits.")
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
	kubernetesOperatorsOrg  = "k8s-operatorhub"
	kubernetesOperatorsRepo = "community-operators"

	openshiftOperatorsOrg  = "redhat-openshift-ecosystem"
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
	if args.GitUserName == "" {
		return errors.New("Git user name is required")
	}
	if args.GitUserEmail == "" {
		return errors.New("Git user email is required")
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
	crds, err := filepath.Glob(args.CRDFileGlob)
	if err != nil {
		return errors.Wrap(err, "failed to find CRD files")
	}
	crdContents := make(map[string][]byte)
	for _, crd := range crds {
		contents, err := ioutil.ReadFile(crd)
		if err != nil {
			return errors.Wrap(err, "failed to read CRD file")
		}
		crdContents[filepath.Base(crd)] = contents
	}

	signoff := fmt.Sprintf("%s <%s>", args.GitUserName, args.GitUserEmail)
	branchName := fmt.Sprintf("release-%s", version)
	err = setReleaseFiles(ctx, deps.GitHub, kubernetesOperatorsOrg, kubernetesOperatorsRepo, args.ForkOrg, branchName, version, signoff, csvContents, packageContents, crdContents)
	if err != nil {
		return errors.Wrap(err, "failed to update kubernetes operator repo")
	}

	err = setReleaseFiles(ctx, deps.GitHub, openshiftOperatorsOrg, openshiftOperatorsRepo, args.ForkOrg, branchName, version, signoff, csvContents, packageContents, crdContents)
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

func setReleaseFiles(ctx context.Context, gh *GitHub, org, repo, forkOrg, branchName, version, signoff string, csvContents, packageContents []byte, crdContents map[string][]byte) error {
	// ensure fork default branch is set to same as upstream commit (makes latest commit "available" to fork)
	mainSHA, mainFound, err := gh.GetRef(ctx, org, repo, BranchRef(defaultBranch))
	if err != nil {
		return err
	}
	if !mainFound {
		return errors.Errorf("Branch %q not found in upstream repo %s/%s", defaultBranch, org, repo)
	}
	err = gh.UpdateRef(ctx, forkOrg, repo, BranchRef(defaultBranch), mainSHA, true)
	if err != nil {
		return err
	}

	// ensure fork branch is set to same as default branch commit
	_, forkBranchExists, err := gh.GetRef(ctx, forkOrg, repo, BranchRef(branchName))
	if err != nil {
		return err
	}
	if forkBranchExists {
		err = gh.UpdateRef(ctx, forkOrg, repo, BranchRef(branchName), mainSHA, true)
		if err != nil {
			return err
		}
	} else {
		err = gh.CreateRef(ctx, forkOrg, repo, BranchRef(branchName), mainSHA)
		if err != nil {
			return err
		}
	}

	message := strings.TrimSpace(fmt.Sprintf(`
Add IBM Cloud Operator release %s

Signed-off-by: %s
`, version, signoff))
	trimmedVersion := strings.TrimPrefix(version, "v")
	versionPath := path.Join("operators", "ibmcloud-operator", trimmedVersion)

	for fileName, contents := range crdContents {
		filePath := path.Join(versionPath, fileName)
		oldCRDFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", filePath)
		if err != nil {
			return errors.Wrapf(err, "failed to get old contents of CRD file %q", fileName)
		}
		err = gh.SetFileContents(ctx, SetFileContentsParams{
			Org:            forkOrg,
			Repo:           repo,
			BranchName:     branchName,
			FilePath:       filePath,
			NewContents:    contents,
			OldContentsSHA: oldCRDFile.SHA,
			Message:        message,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to set contents of file %q", filePath)
		}
	}

	repoCSVPath := path.Join(versionPath, fmt.Sprintf("ibmcloud_operator.%s.clusterserviceversion.yaml", version))
	oldCSVFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", repoCSVPath)
	if err != nil {
		return errors.Wrapf(err, "failed to get old contents of file %q", repoCSVPath)
	}
	err = gh.SetFileContents(ctx, SetFileContentsParams{
		Org:            forkOrg,
		Repo:           repo,
		BranchName:     branchName,
		FilePath:       repoCSVPath,
		NewContents:    csvContents,
		OldContentsSHA: oldCSVFile.SHA,
		Message:        message,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to set contents of file %q", repoCSVPath)
	}

	packagePath := path.Join("operators", "ibmcloud-operator", "ibmcloud-operator.package.yaml")
	oldPackageFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", packagePath)
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
		Message:        message,
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

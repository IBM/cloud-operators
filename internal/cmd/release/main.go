// Command release publishes a new release of IBM Cloud Operator on Operator Hub by opening PRs.
//
// Picks up pre-generated output files and creates pull requests in the appropriate repos.
// See: https://operatorhub.io/operator/ibmcloud-operator
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

	"github.com/ibm/cloud-operators/internal/pipe"
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
}

func main() {
	exitCode := runMain(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(exitCode)
}

func runMain(osArgs []string, stdout, stderr io.Writer) int {
	args, err := parseArgs(osArgs, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	err = run(args, Deps{
		Output: stdout,
		GitHub: newGitHub(args.GitHubToken),
	})
	if err != nil {
		fmt.Fprintf(stderr, "Release failed: %+v\n", err)
		return 1
	}
	return 0
}

var optionalFlags = map[string]bool{
	"draft": true,
}

func parseArgs(osArgs []string, output io.Writer) (args Args, err error) {
	set := flag.NewFlagSet("release", flag.ContinueOnError)
	set.SetOutput(output)
	defer func() {
		if err != nil {
			set.Usage()
		}
	}()

	set.StringVar(&args.Version, "version", "", "The release's version to publish.")
	set.StringVar(&args.GitHubToken, "gh-token", "", "The GitHub token used to open pull requests in OperatorHub repos.")
	set.StringVar(&args.ForkOrg, "fork-org", "", "The fork org to use for opening PRs on repos of the same name.")
	set.StringVar(&args.CSVFile, "csv", "", "Path to the OLM cluster service version file. e.g. out/ibmcloud_operator.vX.Y.Z.clusterserviceversion.yaml")
	set.StringVar(&args.PackageFile, "package", "", "Path to the OLM package file. e.g. out/ibmcloud-operator.package.yaml")
	set.StringVar(&args.CRDFileGlob, "crd-glob", "", "Path to the OLM custom resource definition files. e.g. out/apiextensions.k8s.io_v1beta1_customresourcedefinition_*.ibmcloud.ibm.com.yaml")
	set.BoolVar(&args.DraftPRs, "draft", false, "Open PRs as drafts instead of normal PRs.")
	set.StringVar(&args.GitUserName, "signoff-name", "", "The Git user name to use when signing off commits.")
	set.StringVar(&args.GitUserEmail, "signoff-email", "", "The Git email to use when signing off commits.")
	err = set.Parse(osArgs)
	if err != nil {
		return Args{}, err
	}
	providedFlags := make(map[string]bool)
	set.Visit(func(f *flag.Flag) {
		switch f.Value.String() {
		case "", "false", "0":
		default:
			providedFlags[f.Name] = true
		}
	})
	var unsetFlagErrs []string
	set.VisitAll(func(f *flag.Flag) {
		if !providedFlags[f.Name] && !optionalFlags[f.Name] {
			unsetFlagErrs = append(unsetFlagErrs, "    -"+f.Name)
		}
	})
	if len(unsetFlagErrs) > 0 {
		return Args{}, errors.Errorf("Missing required flags:\n%s", strings.Join(unsetFlagErrs, "\n"))
	}
	return args, nil
}

const (
	kubernetesOperatorsOrg  = "k8s-operatorhub"
	kubernetesOperatorsRepo = "community-operators"

	openshiftOperatorsOrg  = "redhat-openshift-ecosystem"
	openshiftOperatorsRepo = "community-operators-prod"

	defaultBranch = "main"
)

type Deps struct {
	Output io.Writer
	GitHub *GitHub
}

func run(args Args, deps Deps) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	var csvContents, packageContents []byte
	var crds []string
	var crdContents map[string][]byte

	version := "v" + strings.TrimPrefix(args.Version, "v")
	signoff := fmt.Sprintf("%s <%s>", args.GitUserName, args.GitUserEmail)
	branchName := fmt.Sprintf("release-%s", version)
	return pipe.Chain([]pipe.Op{
		func() error {
			csvContents, err = ioutil.ReadFile(args.CSVFile)
			return errors.Wrap(err, "failed to read cluster service version file")
		},
		func() error {
			packageContents, err = ioutil.ReadFile(args.PackageFile)
			return errors.Wrap(err, "failed to read package file")
		},
		func() error {
			crds, err = filepath.Glob(args.CRDFileGlob)
			return errors.Wrap(err, "failed to find CRD files")
		},
		func() error {
			var ops []pipe.Op
			crdContents = make(map[string][]byte)
			for _, crd := range crds {
				crd := crd // loop-local copy for closure
				ops = append(ops, func() error {
					contents, err := ioutil.ReadFile(crd) // #nosec G304 comes from user input points to a concrete CRD
					crdContents[filepath.Base(crd)] = contents
					return errors.Wrap(err, "failed to read CRD file")
				})
			}
			return pipe.Chain(ops)
		},
		func() error {
			err := setReleaseFiles(ctx, deps.GitHub, kubernetesOperatorsOrg, kubernetesOperatorsRepo, args.ForkOrg, branchName, version, signoff, csvContents, packageContents, crdContents)
			return errors.Wrap(err, "failed to update kubernetes operator repo")
		},
		func() error {
			err := setReleaseFiles(ctx, deps.GitHub, openshiftOperatorsOrg, openshiftOperatorsRepo, args.ForkOrg, branchName, version, signoff, csvContents, packageContents, crdContents)
			return errors.Wrap(err, "failed to update openshift operator repo")
		},
		func() error {
			kubernetesPR, err := openPR(ctx, deps.GitHub, kubernetesOperatorsOrg, kubernetesOperatorsRepo, args.ForkOrg, branchName, version, args.DraftPRs)
			fmt.Fprintln(deps.Output, "Kubernetes PR:", kubernetesPR)
			return errors.Wrap(err, "failed to open kubernetes operator PR")
		},
		func() error {
			openshiftPR, err := openPR(ctx, deps.GitHub, openshiftOperatorsOrg, openshiftOperatorsRepo, args.ForkOrg, branchName, version, args.DraftPRs)
			fmt.Fprintln(deps.Output, "OpenShift PR:", openshiftPR)
			return errors.Wrap(err, "failed to open openshift operator PR")
		},
	})
}

func setReleaseFiles(ctx context.Context, gh *GitHub, org, repo, forkOrg, branchName, version, signoff string, csvContents, packageContents []byte, crdContents map[string][]byte) error {
	var err error
	var mainSHA string
	var mainFound, forkBranchExists bool

	message := strings.TrimSpace(fmt.Sprintf(`
Add IBM Cloud Operator release %s

Signed-off-by: %s
`, version, signoff))
	return pipe.Chain([]pipe.Op{
		func() error {
			mainSHA, mainFound, err = gh.GetRef(ctx, org, repo, BranchRef(defaultBranch))
			return err
		},
		func() error {
			return pipe.ErrIf(!mainFound, errors.Errorf("Branch %q not found in upstream repo %s/%s", defaultBranch, org, repo))
		},
		func() error {
			// ensure fork default branch is set to same as upstream commit (makes latest commit "available" to fork)
			return gh.UpdateRef(ctx, forkOrg, repo, BranchRef(defaultBranch), mainSHA, true)
		},
		func() error {
			_, forkBranchExists, err = gh.GetRef(ctx, forkOrg, repo, BranchRef(branchName))
			return err
		},
		func() error {
			// ensure fork branch is set to same as default branch commit
			if forkBranchExists {
				return gh.UpdateRef(ctx, forkOrg, repo, BranchRef(branchName), mainSHA, true)
			}
			return gh.CreateRef(ctx, forkOrg, repo, BranchRef(branchName), mainSHA)
		},
		func() error {
			return setCRDFiles(ctx, gh, forkOrg, repo, branchName, version, message, crdContents)
		},
		func() error {
			return setCSVFile(ctx, gh, forkOrg, repo, branchName, version, message, csvContents)
		},
		func() error {
			return setPackageFile(ctx, gh, forkOrg, repo, branchName, version, message, packageContents)
		},
	})
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

func versionBasePath(version string) string {
	return path.Join("operators", "ibmcloud-operator", strings.TrimPrefix(version, "v"))
}

func setCRDFiles(ctx context.Context, gh *GitHub, forkOrg, repo, branchName, version, message string, crdContents map[string][]byte) error {
	var ops []pipe.Op
	for fileName, contents := range crdContents {
		fileName, contents := fileName, contents // loop-local copy for closure
		filePath := path.Join(versionBasePath(version), fileName)
		var oldSHA string
		ops = append(ops,
			func() error {
				oldCRDFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", filePath)
				oldSHA = oldCRDFile.SHA
				return errors.Wrapf(err, "failed to get old contents of CRD file %q", fileName)
			},
			func() error {
				err := gh.SetFileContents(ctx, SetFileContentsParams{
					Org:            forkOrg,
					Repo:           repo,
					BranchName:     branchName,
					FilePath:       filePath,
					NewContents:    contents,
					OldContentsSHA: oldSHA,
					Message:        message,
				})
				return errors.Wrapf(err, "failed to set contents of file %q", filePath)
			})
	}
	return pipe.Chain(ops)
}

func setCSVFile(ctx context.Context, gh *GitHub, forkOrg, repo, branchName, version, message string, csvContents []byte) error {
	repoCSVPath := path.Join(versionBasePath(version), fmt.Sprintf("ibmcloud_operator.%s.clusterserviceversion.yaml", version))
	var oldSHA string
	return pipe.Chain([]pipe.Op{
		func() error {
			oldCSVFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", repoCSVPath)
			oldSHA = oldCSVFile.SHA
			return errors.Wrapf(err, "failed to get old contents of file %q", repoCSVPath)
		},
		func() error {
			err := gh.SetFileContents(ctx, SetFileContentsParams{
				Org:            forkOrg,
				Repo:           repo,
				BranchName:     branchName,
				FilePath:       repoCSVPath,
				NewContents:    csvContents,
				OldContentsSHA: oldSHA,
				Message:        message,
			})
			return errors.Wrapf(err, "failed to set contents of file %q", repoCSVPath)
		},
	})
}

func setPackageFile(ctx context.Context, gh *GitHub, forkOrg, repo, branchName, version, message string, packageContents []byte) error {
	packagePath := path.Join("operators", "ibmcloud-operator", "ibmcloud-operator.package.yaml")
	var oldSHA string
	return pipe.Chain([]pipe.Op{
		func() error {
			oldPackageFile, _, err := gh.GetFileContents(ctx, forkOrg, repo, "", packagePath)
			oldSHA = oldPackageFile.SHA
			return errors.Wrapf(err, "failed to get old contents of file %q", packagePath)
		},
		func() error {
			err := gh.SetFileContents(ctx, SetFileContentsParams{
				Org:            forkOrg,
				Repo:           repo,
				BranchName:     branchName,
				FilePath:       packagePath,
				NewContents:    packageContents,
				OldContentsSHA: oldSHA,
				Message:        message,
			})
			return errors.Wrapf(err, "failed to set contents of file %q", packagePath)
		},
	})
}

# Contributing to Cloud Operators

You can report issues or open a pull request (PR) to suggest changes. If you open a PR, make sure to follow the Git commit and Golang code style guidelines.

## Table of contents
*   [Reporting an issue](#reporting-an-issue)
*   [Suggesting a change](#suggesting-a-change)
    *   [Assigning and owning work](#assigning-and-owning-work)
    *   [Code style](#code-style)
    *   [Git commit guidelines](#git-commit-guidelines)
    *   [Documentation guidelines](#documentation-guidelines)

## Reporting an issue

To report an issue, or to suggest an idea for a change that you haven't
had time to write-up yet:
1.  [Review existing issues](https://github.com/IBM/cloud-operators/issues) to see if a similar issue has been opened or discussed.
2.  [Open an
issue](https://github.com/IBM/cloud-operators/issues/new). Be sure to include any helpful information, such as your Kubernetes environment details, error messages, or logs that you might have.

[Back to table of contents](#table-of-contents)

## Suggesting a change

To suggest a change to this repository, [submit a pull request](https://github.com/IBM/cloud-operators/pulls) with the complete set of changes that you want to suggest. Make sure your PR meets the following guidelines for code, Git commit messages and signing, and documentation.

### Assigning and owning work

If you want to own and work on an issue, add a comment asking about ownership. A maintainer then adds the **Assigned** label and modifies the first comment in the issue to include `Assigned to: @person`.

[Back to table of contents](#table-of-contents)

### Code style

This project is written in `Go` and follows the Golang community coding style. For guidelines, see the [Go code review style doc](https://github.com/golang/go/wiki/CodeReviewComments).

[Back to table of contents](#table-of-contents)

### Git commit guidelines

#### Conventional Commits

This project uses [Conventional Commits](https://www.conventionalcommits.org) as a guide for commit messages. Make sure that your commit message follows this structure:

```
type(component?): message
```

where

*   *type* is one of: feat, fix, docs, chore, style, refactor, perf, test
*   *component* optionally is the name of the module you are fixing

#### Sign your work

The sign-off is a simple line at the end of the explanation for the patch. Your signature certifies that you wrote the patch or otherwise have the right to pass it on as an open-source patch.

1.  Review the following developer certificate of origin (DCO). Your signature in your commit certifies this statement. The DCO is taken from [developercertificate.org](http://developercertificate.org/).

    ```
    Developer Certificate of Origin
    Version 1.1

    Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
    1 Letterman Drive
    Suite D4700
    San Francisco, CA, 94129

    Everyone is permitted to copy and distribute verbatim copies of this
    license document, but changing it is not allowed.

    Developer's Certificate of Origin 1.1

    By making a contribution to this project, I certify that:

    (a) The contribution was created in whole or in part by me and I
        have the right to submit it under the open source license
        indicated in the file; or

    (b) The contribution is based upon previous work that, to the best
        of my knowledge, is covered under an appropriate open source
        license and I have the right under that license to submit that
        work with modifications, whether created in whole or in part
        by me, under the same open source license (unless I am
        permitted to submit under a different license), as indicated
        in the file; or

    (c) The contribution was provided directly to me by some other
        person who certified (a), (b) or (c) and I have not modified
        it.

    (d) I understand and agree that this project and the contribution
        are public and that a record of the contribution (including all
        personal information I submit with it, including my sign-off) is
        maintained indefinitely and may be redistributed consistent with
        this project or the open source license(s) involved.
    ```
2.  Add the following line to every git commit message in your pull request. Use your real name (sorry, no pseudonyms or anonymous contributions).
    ```
    Signed-off-by: Joe Smith <joe.smith@email.com>
    ```

**Tip**: If you set your `user.name` and `user.email` in your git config, you can sign your
commits automatically with `git commit -s`. (And don't forget your message! `git commit -s -m "type: message"`)

Not sure if your git config is set? Run `git config --list` and review the `user.name` and `user.email` fields. Then, run `git log` for your commit and verify that the `Author` and `Signed-off-by` lines match. If the lines do not match, your PR is rejected by the automated DCO checker.

```
Author: Joe Smith <joe.smith@email.com>
Date:   Thu Feb 2 11:41:15 2018 -0800

    docs: Update README

    Signed-off-by: Joe Smith <joe.smith@email.com>
```

[Back to table of contents](#table-of-contents)

### Documentation guidelines

For documentation within `Go` code, see the Golang community [guidelines](https://github.com/golang/go/wiki/CodeReviewComments#doc-comments) and [commentary](https://golang.org/doc/effective_go.html#commentary).

For documentation pages, this project uses Markdown (`.md` files), and generally follows IBM Style. Some basics of IBM Style include:
*   American English spelling. When in doubt, consult the Merriam Webster dictionary.
*   Active voice and present tense.
*   Sentence case (as opposed to Title Case).

Example docs that you can contribute to:
*   [`/docs` directory](./docs/): You can update an existing file, or add your own. Example files include installation and user guides.
*   [`README.md`](./README.md): Try to keep the `README.md` file about top-level information that is general to most implementations, or that guides users to other content for more detailed information. Keep in mind that this file is reused in other components, such as the OperatorHub.
*   [CONTRIBUTING.md](./CONTRIBUTING.md): Yes, you can even suggest a change to how you want to contribute.

[Back to table of contents](#table-of-contents)


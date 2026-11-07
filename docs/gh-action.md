[← back to docs](./README.md)

<p align="center">
  <h3 align="center">GitHub Action for CODEOWNERS Validator</h3>
  <p align="center">Ensures the correctness of your CODEOWNERS file.</p>
  <p align="center">
    <a href="/LICENSE"><img alt="Software License" src="https://img.shields.io/badge/license-Apache-brightgreen.svg?style=flat-square"></a>
  </p>
</p>

##
The [Codeowners Validator](https://github.com/mlarraz/codeowners-validator) is available as a GitHub Action. It prints a report like this:

```text
==> Executing Duplicated Pattern Checker (63.375µs)
    Check OK
==> Executing Valid Syntax Checker (34.542µs)
    Check OK
==> Executing File Exist Checker (5.030625ms)
    Check OK
==> Executing [Experimental] Not Owned File Checker (118.596ms)
    Check OK

4 check(s) executed, no failure(s)
```


## Usage

Create a workflow (eg: `.github/workflows/sanity.yml` see [Creating a Workflow file](https://help.github.com/en/articles/configuring-a-workflow#creating-a-workflow-file))

```yaml
name: "Codeowners Validator"

on:
  pull_request:
    paths:
      - ".github/CODEOWNERS"
  schedule:
    # Runs at 08:00 UTC every day
    - cron:  '0 8 * * *'

permissions:
  contents: read

jobs:
  sanity:
    runs-on: ubuntu-latest
    steps:
      # Checks out only the CODEOWNERS file, see "Checkout" below
      - uses: actions/checkout@v7
        with:
          sparse-checkout: .github/CODEOWNERS
          sparse-checkout-cone-mode: false
          persist-credentials: false
      - name: GitHub CODEOWNERS Validator
        uses: mlarraz/codeowners-validator@v0.8.0
        # input parameters
        with:
          # ==== GitHub Auth ====

          ## ==== Token ====
          # The token the `owners` check uses to read CODEOWNERS errors from the GitHub API. Defaults to the workflow's token.
          github_access_token: "${{ github.token }}"

          ## ==== App ====
          # GitHub App ID for authentication. When set, the app is used instead of github_access_token.
          # github_app_id: ${{ secrets.APP_ID }}

          # GitHub App Installation ID. Required when github_app_id is set.
          # github_app_installation_id: ${{ secrets.APP_INSTALLATION_ID }}

          # GitHub App private key in PEM format. Required when github_app_id is set.
          # github_app_private_key: ${{ secrets.APP_PRIVATE_KEY }}

          # ==== GitHub Auth ====

          # "The list of checks that will be executed. By default, all checks are executed. Possible values: files,owners,duppatterns,syntax"
          checks: "files,owners,duppatterns,syntax"

          # "The comma-separated list of experimental checks that should be executed. By default, all experimental checks are turned off. Possible values: notowned,avoid-shadowing"
          experimental_checks: "notowned,avoid-shadowing"

          # The GitHub base URL for API requests. Defaults to the public GitHub API, but can be set to a domain endpoint to use with GitHub Enterprise.
          github_base_url: "https://api.github.com/"

          # The GitHub upload URL for uploading files. It is taken into account only when the GITHUB_BASE_URL is also set. If only the GITHUB_BASE_URL is provided then this parameter defaults to the GITHUB_BASE_URL value.
          github_upload_url: "https://uploads.github.com/"

          # The repository path in which CODEOWNERS file should be validated."
          repository_path: "."

          # Specifies whether Git should trust the repository path. The action's container runs as a different user than the one that owns the checkout.
          trust_workspace: "true"

          # Defines the level on which the application should treat check issues as failures. Defaults to warning, which treats both errors and warnings as failures, and exits with error code 3. Possible values are error and warning. Default: warning"
          check_failure_level: "warning"

          # The comma-separated list of patterns that should be ignored by not-owned-checker. For example, you can specify * and as a result, the * pattern from the CODEOWNERS file will be ignored and files owned by this pattern will be reported as unowned unless a later specific pattern will match that path. It's useful because often we have default owners entry at the begging of the CODOEWNERS file, e.g. * @global-owner1 @global-owner2"
          not_owned_checker_skip_patterns: ""

          # The owner and repository name whose CODEOWNERS file the `owners` check validates."
          owner_checker_repository: "${{ github.repository }}"

          # The branch, tag, or commit SHA whose CODEOWNERS file the `owners` check validates. It must exist on GitHub. Defaults to the commit checked out in repository_path.
          owner_checker_ref: ""

          # The comma-separated list of owners that should not be validated. Example: @owner1,@owner2,@org/team1,example@email.com."
          owner_checker_ignored_owners: "@ghost"

          # Specifies whether CODEOWNERS may have unowned files. For example, `/infra/oncall-rotator/oncall-config.yml` doesn't have owner and this is not reported.
          owner_checker_allow_unowned_patterns: "true"

          # Specifies whether only teams are allowed as owners of files.
          owner_checker_owners_must_be_teams: "false"

          # Only check listed subdirectories for CODEOWNERS ownership that don't have owners.
          not_owned_checker_subdirectories: ""
```

The best is to run this as a cron job and not only if you applying changes to CODEOWNERS file itself, e.g. the CODEOWNERS file can be invalidate when you removing someone from the organization.

> **Note**
>
> The `owners` check uses the workflow's `GITHUB_TOKEN`, which needs the `contents: read` permission. See [GitHub authentication](./gh-auth.md) for tokens and GitHub Apps.

## Checkout

No check reads the contents of any file but CODEOWNERS:

- `syntax`, `duppatterns`, `avoid-shadowing`, and `owners` read the CODEOWNERS file. `owners` sends the commit to GitHub, which evaluates the CODEOWNERS file at that commit.
- `files` and `notowned` read the list of tracked files from Git's index.

A sparse checkout of the CODEOWNERS file is therefore enough, however big the repository is. `actions/checkout` makes sparse checkouts blobless (`--filter=blob:none`), so it downloads the file tree of the commit but none of the other files. Turn off cone mode so that only the CODEOWNERS file is checked out.

On `pull_request` events, `actions/checkout` checks out the pull request's merge commit, which exists on GitHub, so `owners` validates the CODEOWNERS file as it would be after merging.

<!--- example repository when failed -->

## Configuration

For the GitHub Action, use the configuration described in the main README under the [Configuration](../README.md#configuration) section but **specify it as the [Action input parameters](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions#jobsjob_idstepswith) instead of environment variables**. See the [Usage](#usage) section for the full syntax.

If you want to use environment variables anyway, you must add the `INPUT_` prefix to each environment variable. For example, `OWNER_CHECKER_IGNORED_OWNERS` becomes `INPUT_OWNER_CHECKER_IGNORED_OWNERS`.

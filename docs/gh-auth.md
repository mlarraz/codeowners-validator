[← back to docs](./README.md)

# GitHub authentication

The `owners` check asks the GitHub API which errors GitHub finds in the CODEOWNERS file ([`GET /repos/{owner}/{repo}/codeowners/errors`](https://docs.github.com/en/rest/repos/repos#list-codeowners-errors)). It's the same evaluation GitHub uses to request reviews, so it reports owners that don't exist, can't be seen, or don't have write access to the repository.

The check works without credentials for public repositories, but unauthenticated requests are limited to 60 per hour per IP address. Private repositories, GitHub Enterprise, and shared CI runners need credentials: either a [token](#github-tokens) or a [GitHub App](#github-app). The other checks don't call GitHub.

## GitHub tokens

The token needs read access to the repository's contents:

| Token                                                                                                                                                                | Permission                                                                                                                 |
|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| [`GITHUB_TOKEN`](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication) in GitHub Actions                     | `contents: read`, the default. The GitHub Action uses it unless you set `github_access_token` or a GitHub App.             |
| [Fine-grained personal access token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)           | Repository permission **Contents: Read-only** on the repository.                                                           |
| [Personal access token (classic)](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)              | No scope for public repositories, `repo` for private repositories.                                                         |


## GitHub App

Here are the steps to create a GitHub App and use it for this tool:

1. [Create a GitHub App](https://docs.github.com/en/developers/apps/building-github-apps/creating-a-github-app).
    > **Note**
    > Your app does not need a callback or a webhook URL.
2. Add the **Contents: Read-only** repository permission.
3. [Install the app](https://docs.github.com/en/developers/apps/managing-github-apps/installing-github-apps) on the repository whose CODEOWNERS file you validate.
4. Done! To authenticate with your app, you need:

    | Name                       | Description                                                                                                                                                                                                                                       |
    |---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
    | GitHub App Private Key     | PEM-format key generated when the app is installed. If you lost it, you can regenerate it ([docs](https://docs.github.com/en/developers/apps/building-github-apps/authenticating-with-github-apps#generating-a-private-key)).                     |
    | GitHub App ID              | Found in the app's "About" page (Organization settings -> Developer settings -> Edit button on your app).                                                                                                                                         |
    | GitHub App Installation ID | Found in the URL your organization's app install page (Organization settings -> Github Apps -> Configure button on your app). It's the last number in the URL, ex: `https://github.com/organizations/{my-org}/settings/installations/1234567890`. |

5. Depends on the usage you need to:

   1. **CLI:** Export them as environment variable:
      - `GITHUB_APP_INSTALLATION_ID`
      - `GITHUB_APP_ID`
      - `GITHUB_APP_PRIVATE_KEY`

   2. [**GitHub Action:**](gh-action.md) Define them as GitHub secrets and use under the `with` property:

      ```yaml
      - name: GitHub CODEOWNERS Validator
        uses: mlarraz/codeowners-validator@v0.9.1
        with:
          # ...
          github_app_id: ${{ secrets.APP_ID }}
          github_app_installation_id: ${{ secrets.APP_INSTALLATION_ID }}
          github_app_private_key: ${{ secrets.APP_PRIVATE_KEY }}
      ```

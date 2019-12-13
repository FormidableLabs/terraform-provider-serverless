Contributing
============

Thanks for contributing!

## Prerequisites

- Access to the Formidable AWS account (or your own if you're an external contributor).
- `aws-vault`. Follow [these instructions](https://github.com/99designs/aws-vault) to install and set up your profile.
- Terraform and `tfenv`. `tfenv` conflicts with Homebrew `terraform`, so uninstall it before proceeding. In the root of the repo, run `brew install tfenv && tfenv install`. `tfenv` will download and install the Terraform version pinned in `.terraform-version`.
- [`golangci-lint`](https://github.com/golangci/golangci-lint) for linting Go code. Install with `brew install golangci/tap/golangci-lint`.

## Development

We have an example Terraform config for the provider at `example-test.tf`. This config has access to the custom provider binary. To manually test provider changes with this config, run the following:

```sh
aws-vault exec --no-session {ACCOUNT_NAME} # or your own account's profile
go build -o terraform-provider-serverless
terraform init
terraform apply
exit

# To see debug logs, run `TF_LOG=debug terraform apply` instead.
```

### Debugging

Terraform infers log levels from string prefixes. To log values from the provider, you can use:

```go
log.Println("[WARN]", value)
```

You can then use `TF_LOG=warn` before `terraform apply` or `make testacc` to see your logs.

### Linting

We lint the the codebase with [`golangci-lint`](https://github.com/golangci/golangci-lint). To lint:

```sh
make lint
```

### Testing

To run acceptance tests:

```sh
aws-vault exec --no-session {ACCOUNT_NAME} # or your own account's profile
make testacc
exit
```

## Before submitting a PR...

- Run lint and the acceptance tests listed above.
- Run `make fmt` to format your code! CI will catch this, but it'll save you a build!

## Releases

1. Update `CHANGELOG.md`, following format for previous versions
2. Commit as "Changes for version NUMBER"
3. Tag a version in git master branch after merging the PRs you want to release. E.g. `git tag -a "v1.2.3" -m "1.2.3"`
4. Run `git push && git push --tags`

After a git tag is pushed, a Travis `deploy` step will use [Goreleaser](https://goreleaser.com/) to build cross-platform binaries and place them in a GitHub release automatically.

Contributing
============

Thanks for contributing!

## Prerequisites

- Access to the Formidable AWS account (or your own if you're an external contributor).
- `aws-vault`. Follow [these instructions](https://github.com/99designs/aws-vault) to install and set up your profile.
- Terraform and `tfenv`. `tfenv` conflicts with Homebrew `terraform`, so uninstall it before proceeding. In the root of the repo, run `brew install tfenv && tfenv install`. `tfenv` will download and install the Terraform version pinned in `.terraform-version`.

## Development

We have an example Terraform config for the provider at `example-test.tf`. This config has access to the custom provider binary. To manually test provider changes with this config, run the following:

```sh
aws-vault exec --no-session formidable # or your own account's profile
go build -o terraform-provider-serverless
terraform init
terraform apply
exit

# To see debug logs, run `TF_LOG=debug terraform apply` instead.
```

## Debugging

Terraform infers log levels from string prefixes. To log values from the provider, you can use:

```go
log.Println("[WARN]", value)
```

You can then use `TF_LOG=warn` before `terraform apply` or `make testacc` to see your logs.

## Testing

To run acceptance tests:

```sh
aws-vault exec --no-session formidable # or your own account's profile
make testacc
exit
```

## Before submitting a PR...

- run the acceptance tests! See the instructions above.
- run `make fmt` to format your code! CI will catch this, but it'll save you a build!

## Releases

- Push a version tag (e.g. v1.2.3) to master after merging the PRs you want to release.
- [Goreleaser](https://goreleaser.com/) will build cross-platform binaries and place them in a GitHub release.

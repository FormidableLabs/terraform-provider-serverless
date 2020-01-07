Terraform Provider for Serverless
=================================
[![Build Status](https://travis-ci.com/FormidableLabs/terraform-provider-serverless.svg?branch=master)](https://travis-ci.com/FormidableLabs/terraform-provider-serverless)

Integrate Serverless into your Terraform dependency graph!

## Why?

It's common to augment the simplicity and developer experience of Serverless with the power and flexibility of Terraform, either to fill in gaps in Serverless or to provision its supporting resources. However, Serverless and Terraform don't interoperate out-of-the-box. Even if you get the two to communicate, neither of them are aware of their dependencies on each other. This can lead to chicken-and-egg problems where one Terraform resource must exist before Serverless, but the rest of your resources depend on a Serverless deploy.

The Serverless provider integrates Serverless deploys as a Terraform resource so that Terraform can resolve resource dependencies correctly. It's a great tool for shimming Serverless into a Terraform-heavy project or easing the migration cost away from Serverless to pure Terraform.

## Installation

The Serverless provider is not an official provider, so `terraform init` doesn't automatically download and install it. See HashiCorp's instructions for [installing third-party providers.](https://www.terraform.io/docs/configuration/providers.html#third-party-plugins)

## Packaging requirements

This provider avoids deploying Serverless when code or configuration hasn't changed. This saves deployment time, supports deployment of bit-identical package artifacts across different environments, and prevents application code changes from reaching users when you only need to update configuration for other Terraform resources.

To avoid spurious deploys, Serverless requires you to package out-of-band from `sls deploy`. To update the package and trigger a deploy on the next `terraform apply`, run `sls package -p .terraform-serverless`. You should no longer use the `sls deploy` command.

## Resources

```hcl
resource "serverless_deployment" "example" {
  # The Serverless stage. Usually corresponds to the stage/environment of the Terraform module.
  stage               = "sandbox"

  # The directory where your `serverless.yml` config lives. Must be an absolute path.
  config_dir         = abspath("example")

  # The directory where your Serverless package lives. Defaults to `.terraform-serverless`.
  # **NOTE:** must be relative to `config_dir`!
  package_dir         = ".terraform-serverless"

  # The directory where your `serverless` binary lives. Defaults to the `node_modules/.bin` in your `config_dir`.
  serverless_bin_dir = abspath("example/node_modules/.bin")
}
```

## Contributing

See our contribution guidelines [here!](CONTRIBUTING.md)

## Maintenance Status

**Experimental:** This project is quite new. We're not sure what our ongoing maintenance plan for this project will be. Bug reports, feature requests and pull requests are welcome. If you like this project, let us know!

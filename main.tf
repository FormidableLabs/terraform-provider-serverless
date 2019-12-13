resource "serverless_deployment" "example" {
  config_dir         = abspath("example")
  # Relative to config path
  package_dir         = ".terraform-serverless"
  stage               = "sandbox"
  serverless_bin_dir = abspath("example/node_modules/.bin")
}

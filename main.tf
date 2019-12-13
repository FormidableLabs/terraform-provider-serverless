resource "serverless_deployment" "example" {
  config_dir         = abspath("serverless.yml")
  # Relative to config path
  package_dir         = ".terraform-serverless"
  stage               = "sandbox"
  serverless_bin_dir = abspath("node_modules/.bin")
}

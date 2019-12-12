resource "serverless_deployment" "example" {
  config_path         = abspath("serverless.yml")
  # Relative to config path
  package_dir         = ".terraform-serverless"
  stage               = "sandbox"
  serverless_bin_path = abspath("node_modules/.bin/serverless")
}

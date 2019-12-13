package serverless

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

type hashWrapper struct {
	value string
}

func TestAccServerlessDeployment_basic(t *testing.T) {
	hash := hashWrapper{}

	resource.Test(t, resource.TestCase{
		Providers:    testAccProviders,
		PreCheck:     func() { testAccPreCheck(t) },
		CheckDestroy: testAccCheckServerlessDeploymentDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccServerlessDeploymentConfig(),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"serverless_deployment.example", "package_dir", ".terraform-serverless",
					),
					resource.TestCheckResourceAttr(
						"serverless_deployment.example", "stage", "sandbox",
					),
					testAccCheckServerlessDeploymentExists(&hash),
				),
			},
			{
				Config: testAccServerlessDeploymentConfig(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServerlessDeploymentNoOps(&hash),
				),
			},
		},
	})
}

func testAccPreCheck(t *testing.T) {
	dir, err := filepath.Abs("../")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("yarn")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(fmt.Errorf("%v\n%w", string(output), err))
	}

	configDir, err := filepath.Abs("../")
	if err != nil {
		t.Fatal(err)
	}

	serverlessBinPath, err := filepath.Abs("../node_modules/.bin/serverless")
	if err != nil {
		t.Fatal(err)
	}

	err = runServerless(&serverlessParams{
		command:           "package",
		configDir:         configDir,
		serverlessBinPath: serverlessBinPath,
		packageDir:        ".terraform-serverless",
		stage:             "sandbox",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testAccCheckServerlessDeploymentDestroy(s *terraform.State) error {
	sess := session.Must(session.NewSession())
	cf := cloudformation.New(sess)
	_, err := cf.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String("sls-provider-sandbox"),
	})

	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "ValidationError" && strings.Contains(aerr.Message(), "does not exist") {
			return nil
		}
	}
	return err
}

func testAccServerlessDeploymentConfig() string {
	return `
resource "serverless_deployment" "example" {
  config_dir         = abspath("../")
  serverless_bin_path = abspath("../node_modules/.bin/serverless")
  # Relative to config path
  package_dir         = ".terraform-serverless"
  stage               = "sandbox"
}`
}

func testAccCheckServerlessDeploymentExists(hashWrapper *hashWrapper) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		sess := session.Must(session.NewSession())
		cf := cloudformation.New(sess)
		_, err := cf.DescribeStacks(&cloudformation.DescribeStacksInput{
			StackName: aws.String("sls-provider-sandbox"),
		})

		hashWrapper.value = s.RootModule().Resources["serverless_deployment.example"].Primary.Attributes["package_hash"]

		return err
	}
}

func testAccCheckServerlessDeploymentNoOps(hashWrapper *hashWrapper) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		newHash := s.RootModule().Resources["serverless_deployment.example"].Primary.Attributes["package_hash"]
		if hashWrapper.value != newHash {
			return fmt.Errorf("Content hash changed unexpectedly.\nOld hash: %v\nNew hash: %v", hashWrapper.value, newHash)
		}

		return nil
	}
}

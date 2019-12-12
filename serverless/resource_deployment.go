package serverless

import (
	"fmt"
	"io"
	"io/ioutil"

	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"

	yaml "gopkg.in/yaml.v2"

	"github.com/hashicorp/terraform-plugin-sdk/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

type serverlessConfig struct {
	Service string
}

func getServiceName(configPath string) (string, error) {
	configBytes, err := ioutil.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	config := serverlessConfig{}
	err = yaml.Unmarshal(configBytes, &config)

	return config.Service, err
}

func hashServerlessDir(configPath string, packagePath string) (string, error) {
	absolutePackagePath := filepath.Join(filepath.Dir(configPath), packagePath)
	zipPath := filepath.Join(absolutePackagePath, "sls-provider.zip")

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return "", err
	}

	// https://github.com/golang/mod/blob/master/sumdb/dirhash/hash.go#L75
	osOpen := func(name string) (io.ReadCloser, error) {
		return os.Open(name)
	}

	configHash, err := dirhash.Hash1([]string{configPath}, osOpen)
	if err != nil {
		return "", err
	}

	return strings.Join([]string{zipHash, configHash}, "-"), nil
}

type serverlessParams struct {
	command           string
	serverlessBinPath string
	configPath        string
	packageDir        string
	stage             string
	args              []interface{}
}

func runServerless(params *serverlessParams) error {
	stringArgs := make([]string, len(params.args))
	for i, v := range stringArgs {
		stringArgs[i] = fmt.Sprint(v)
	}

	requiredArgs := []string{
		params.command,
		"-s",
		params.stage,
	}

	if params.command == "deploy" || params.command == "package" {
		requiredArgs = append(requiredArgs, "-p", params.packageDir)
	}

	stringArgs = append(
		requiredArgs,
		stringArgs...,
	)
	cmd := exec.Command(params.serverlessBinPath, stringArgs...)
	cmd.Dir = filepath.Dir(params.configPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%w", string(output), err)
	}

	return nil
}

func resourceDeployment() *schema.Resource {
	return &schema.Resource{
		Create: resourceDeploymentCreate,
		Read:   resourceDeploymentRead,
		Update: resourceDeploymentUpdate,
		Delete: resourceDeploymentDelete,

		Schema: map[string]*schema.Schema{
			"config_path": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"serverless_bin_path": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"package_dir": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"stage": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"args": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"package_hash": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},

		CustomizeDiff: customdiff.ComputedIf("package_hash", func(d *schema.ResourceDiff, meta interface{}) bool {
			configPath := d.Get("config_path").(string)
			packageDir := d.Get("package_dir").(string)
			currentHash := d.Get("package_hash").(string)

			hash, err := hashServerlessDir(configPath, packageDir)
			if err != nil {
				return false
			}

			return hash != currentHash
		}),
	}
}

func resourceDeploymentCreate(d *schema.ResourceData, m interface{}) error {
	configPath := d.Get("config_path").(string)
	serverlessBinPath := d.Get("serverless_bin_path").(string)
	packageDir := d.Get("package_dir").(string)
	stage := d.Get("stage").(string)
	args := d.Get("args").([]interface{})

	id, err := getServiceName(configPath)
	if err != nil {
		return err
	}
	d.SetId(id)

	hash, err := hashServerlessDir(configPath, packageDir)
	if err != nil {
		return err
	}
	err = d.Set("package_hash", hash)
	if err != nil {
		return err
	}

	err = runServerless(&serverlessParams{
		command:           "deploy",
		serverlessBinPath: serverlessBinPath,
		configPath:        configPath,
		packageDir:        packageDir,
		stage:             stage,
		args:              args,
	})

	if err != nil {
		return err
	}

	return resourceDeploymentRead(d, m)
}

func resourceDeploymentRead(d *schema.ResourceData, m interface{}) error {
	id := d.Id()
	stage := d.Get("stage").(string)

	sess := session.Must(session.NewSession())
	cf := cloudformation.New(sess)
	_, err := cf.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String(strings.Join([]string{id, stage}, "-")),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "ValidationError" && strings.Contains(aerr.Message(), "does not exist") {
				d.SetId("")
				return nil
			}
		}
		return err
	}

	return nil
}

func resourceDeploymentUpdate(d *schema.ResourceData, m interface{}) error {
	shouldChange := d.HasChanges(
		"config_path",
		"package_dir",
		"stage",
		"args",
		"serverless_bin_path",
		"package_hash",
	)

	if shouldChange {
		return resourceDeploymentCreate(d, m)
	}

	return resourceDeploymentRead(d, m)
}

func resourceDeploymentDelete(d *schema.ResourceData, m interface{}) error {
	configPath := d.Get("config_path").(string)
	serverlessBinPath := d.Get("serverless_bin_path").(string)
	packageDir := d.Get("package_dir").(string)
	stage := d.Get("stage").(string)
	args := d.Get("args").([]interface{})

	err := runServerless(&serverlessParams{
		command:           "remove",
		serverlessBinPath: serverlessBinPath,
		configPath:        configPath,
		packageDir:        packageDir,
		stage:             stage,
		args:              args,
	})
	if err != nil {
		return err
	}

	d.SetId("")

	return nil
}

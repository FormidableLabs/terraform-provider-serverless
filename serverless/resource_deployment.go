package serverless

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/hashicorp/terraform-plugin-sdk/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

type serverlessConfig struct {
	Service string
}

func getServerlessConfig(configDir string, serverlessBinDir string) ([]byte, error) {
	cmd := exec.Command(getServerlessBin(serverlessBinDir), "print", "--format", "json")
	cmd.Dir = configDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return []byte{}, fmt.Errorf("%v\n%w", string(output), err)
	}

	return output, nil
}

func getServiceName(configJson []byte) (string, error) {
	config := serverlessConfig{}
	err := json.Unmarshal([]byte(configJson), &config)

	return config.Service, err
}

func hashServerlessDir(configDir string, packagePath string, configJson []byte) (string, error) {
	absolutePackagePath := filepath.Join(configDir, packagePath)
	zipPath := filepath.Join(absolutePackagePath, "sls-provider.zip")

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return "", err
	}

	configHashBytes := sha256.Sum256([]byte(configJson))
	configHash := hex.EncodeToString(configHashBytes[:])

	return strings.Join([]string{zipHash, configHash}, "-"), nil
}

func getServerlessBin(binPath string) string {
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".cmd"
	}
	return filepath.Join(binPath, fmt.Sprintf("serverless%s", suffix))
}

type serverlessParams struct {
	command          string
	serverlessBinDir string
	configDir        string
	packageDir       string
	stage            string
	args             []interface{}
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

	cmd := exec.Command(getServerlessBin(params.serverlessBinDir), stringArgs...)
	cmd.Dir = params.configDir

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
			"config_dir": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"serverless_bin_dir": &schema.Schema{
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
			configDir := d.Get("config_dir").(string)
			packageDir := d.Get("package_dir").(string)
			serverlessBinDir := d.Get("serverless_bin_dir").(string)
			currentHash := d.Get("package_hash").(string)

			configJson, err := getServerlessConfig(configDir, serverlessBinDir)
			if err != nil {
				return false
			}

			hash, err := hashServerlessDir(configDir, packageDir, configJson)
			if err != nil {
				return false
			}

			return hash != currentHash
		}),
	}
}

func resourceDeploymentCreate(d *schema.ResourceData, m interface{}) error {
	configDir := d.Get("config_dir").(string)
	serverlessBinDir := d.Get("serverless_bin_dir").(string)
	packageDir := d.Get("package_dir").(string)
	stage := d.Get("stage").(string)
	args := d.Get("args").([]interface{})

	configJson, err := getServerlessConfig(configDir, serverlessBinDir)
	if err != nil {
		return err
	}

	id, err := getServiceName(configJson)
	if err != nil {
		return err
	}
	d.SetId(id)

	hash, err := hashServerlessDir(configDir, packageDir, configJson)
	if err != nil {
		return err
	}
	err = d.Set("package_hash", hash)
	if err != nil {
		return err
	}

	err = runServerless(&serverlessParams{
		command:          "deploy",
		serverlessBinDir: serverlessBinDir,
		configDir:        configDir,
		packageDir:       packageDir,
		stage:            stage,
		args:             args,
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
		"config_dir",
		"package_dir",
		"stage",
		"args",
		"serverless_bin_dir",
		"package_hash",
	)

	if shouldChange {
		return resourceDeploymentCreate(d, m)
	}

	return resourceDeploymentRead(d, m)
}

func resourceDeploymentDelete(d *schema.ResourceData, m interface{}) error {
	configDir := d.Get("config_dir").(string)
	serverlessBinDir := d.Get("serverless_bin_dir").(string)
	packageDir := d.Get("package_dir").(string)
	stage := d.Get("stage").(string)
	args := d.Get("args").([]interface{})

	err := runServerless(&serverlessParams{
		command:          "remove",
		serverlessBinDir: serverlessBinDir,
		configDir:        configDir,
		packageDir:       packageDir,
		stage:            stage,
		args:             args,
	})
	if err != nil {
		return err
	}

	d.SetId("")

	return nil
}

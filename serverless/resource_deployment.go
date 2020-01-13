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

// Code Review Comments
// - servlessConfig struct + getServiceName seem somewhat redundant/useless
// - we're type casting the same data in multiple places, might be worth investigating
//   if we can capture this in a helper function or struct + method
//   idea: type getter interface + struct for serverless config/data
// - mixing passing strings + []byte, might want to be consistent
// - do we want out resource name to be 'deployment' vs 'serverless' or 'serverless_deployment'
//   see https://www.terraform.io/docs/extend/best-practices/naming.html
// - investigate utilizing io.Reader + io.Writer interfaces

// Questions
// - Follow up on `hashServerlessDir` in the case when multiple things need hashed
// - How does %w work in fmt.Errorf
//

type getter interface {
	Get(key string) interface{}
}

// TODO investigate if we can remove some of these fields if they're only being used
// to construct other values in one place (move that logic to the constructor)
type serverless struct {
	binDir     string
	binPath    string
	configDir  string
	packageDir string
	stage      string
	hash       string
	args       []string
}

func (s serverless) run(command string) error {
	args := []string{
		command, "-s",
		s.stage,
	}

	if command == "deploy" || command == "package" {
		args = append(args, "-p", s.packageDir)
	}

	args = append(args, s.args...)

	cmd := exec.Command(s.binPath, args...)
	cmd.Dir = s.configDir

	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("%v\n%w", string(output), err)
	}

	return nil
}

func newServerless(resource getter) serverless {
	resourceArgs := resource.Get("args").([]interface{})
	args := make([]string, len(resourceArgs))

	for _, arg := range resourceArgs {
		args = append(args, arg.(string))
	}

	var binPath string
	var suffix string

	binDir := resource.Get("serverless_bin_dir").(string)
	configDir := resource.Get("config_dir").(string)

	if binDir == "" {
		binPath = filepath.Join(configDir, "node_modules", ".bin")
	} else {
		binPath = binDir
	}

	if runtime.GOOS == "windows" {
		suffix = ".cmd"
	}

	binPath = filepath.Join(binPath, fmt.Sprintf("serverless%s", suffix))

	return serverless{
		binDir:     binDir,
		binPath:    binPath,
		configDir:  configDir,
		packageDir: resource.Get("package_dir").(string),
		stage:      resource.Get("stage").(string),
		hash:       resource.Get("package_hash").(string),
		args:       args,
	}
}

type serverlessConfig struct {
	Service string
}

func getServerlessConfig(configDir string, serverlessBinDir string) ([]byte, error) {
	cmd := exec.Command(getServerlessBin(configDir, serverlessBinDir), "print", "--format", "json")
	cmd.Dir = configDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return []byte{}, fmt.Errorf("%v\n%w", string(output), err)
	}

	return output, nil
}

// Code Review Comment
// What all does configJson hold in it? Is it just a JSON block with a 'service' key?
// This function seems a little bit useless or redundant
func getServiceName(configJson []byte) (string, error) {
	config := serverlessConfig{}
	err := json.Unmarshal([]byte(configJson), &config)

	return config.Service, err
}

// Create a hash of the Serverless config and the Serverless zip archive.
// Note that dirhash.HashZip ignores all zip metadata and correctly hashes
// contents of the archive.
func hashServerlessDir(
	configDir string,
	packagePath string,
	serviceName string,
	configJson []byte,
) (string, error) {
	absolutePackagePath := filepath.Join(configDir, packagePath)
	zipPath := filepath.Join(absolutePackagePath, fmt.Sprintf("%s.zip", serviceName))

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return "", err
	}

	configHashBytes := sha256.Sum256(configJson)
	configHash := hex.EncodeToString(configHashBytes[:])

	return strings.Join([]string{zipHash, configHash}, "-"), nil
}

func getServerlessBin(configDir string, binPath string) string {
	if binPath == "" {
		binPath = filepath.Join(configDir, "node_modules", ".bin")
	}
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

	cmd := exec.Command(getServerlessBin(params.configDir, params.serverlessBinDir), stringArgs...)
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
				Optional: true,
				Default:  "",
			},
			// The directory where the Serverless package lives. In the CLI, this defaults to
			// .serverless, but we default to .terraform-serverless to avoid an issue where
			// the CLI deletes the .serverless directory after deploy, even with --package.
			// Note that the provider requires out-of-band packaging. Users should package
			// their code with `sls package --package .terraform-serverless`.
			//
			// NOTE: the path you provide must be RELATIVE to your `config_dir` since the
			// --package flag in the CLI does not support absolute paths.
			"package_dir": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  ".terraform-serverless",
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

		// Only trigger a deploy if either the Serverless config or Serverless zip archive has changed.
		// `sls package` isn't deterministic according to experiments, so in practive this means that
		// we only deploy after the user has run `sls package` again.
		CustomizeDiff: customdiff.ComputedIf("package_hash", func(d *schema.ResourceDiff, meta interface{}) bool {
			configDir := d.Get("config_dir").(string)
			packageDir := d.Get("package_dir").(string)
			serverlessBinDir := d.Get("serverless_bin_dir").(string)
			currentHash := d.Get("package_hash").(string)

			configJson, err := getServerlessConfig(configDir, serverlessBinDir)
			if err != nil {
				return false
			}

			hash, err := hashServerlessDir(configDir, packageDir, d.Id(), configJson)
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

	hash, err := hashServerlessDir(configDir, packageDir, id, configJson)
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

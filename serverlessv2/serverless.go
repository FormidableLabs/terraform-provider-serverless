package serverlessv2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/mod/sumdb/dirhash"
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
	config     map[string]interface{}
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

func (s *serverless) loadServerlessConfig() error {
	config := make(map[string]interface{})

	cmd := exec.Command(s.binPath, "print", "--format", "json")
	cmd.Dir = s.configDir

	output, err := cmd.CombinedOutput()

	if err != nil {
		fmt.Errorf("%v\n%w", string(output), err)
	}

	if err = json.Unmarshal(output, &config); err != nil {
		return err
	}

	s.config = config

	return nil
}

func (s *serverless) rehash() (changed bool, err error) {
	serviceName := s.config["service"].(string) // TODO: possibly check 2nd return for existence + handle err
	zipPath := filepath.Join(s.configDir, s.packageDir, fmt.Sprintf("%s.zip", serviceName))

	configJSON, err := json.Marshal(s.config)

	if err != nil {
		return changed, err
	}

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)

	if err != nil {
		return changed, err
	}

	configHashBytes := sha256.Sum256(configJSON)
	configHash := hex.EncodeToString(configHashBytes[:])

	hash := fmt.Sprintf("%s-%s", zipHash, configHash)

	if hash != s.hash {
		changed = true
	}

	s.hash = hash

	return changed, nil
}

func newServerless(resource getter) (*serverless, error) {
	resourceArgs := resource.Get("args").([]interface{})
	args := make([]string, len(resourceArgs))

	for _, arg := range resourceArgs {
		args = append(args, arg.(string))
	}

	binDir := resource.Get("serverless_bin_dir").(string)
	configDir := resource.Get("config_dir").(string)

	s := &serverless{
		binDir:     binDir,
		binPath:    buildBinPath(configDir, binDir),
		configDir:  configDir,
		packageDir: resource.Get("package_dir").(string),
		stage:      resource.Get("stage").(string),
		hash:       resource.Get("package_hash").(string),
		args:       args,
	}

	if err := s.loadServerlessConfig(); err != nil {
		return nil, err
	}

	if _, err := s.rehash(); err != nil {
		return nil, err
	}

	return s, nil
}

func buildBinPath(configDir, binDir string) string {
	var binPath string
	var suffix string

	if binDir == "" {
		binPath = filepath.Join(configDir, "node_modules", ".bin")
	} else {
		binPath = binDir
	}

	if runtime.GOOS == "windows" {
		suffix = ".cmd"
	}

	binPath = filepath.Join(binPath, fmt.Sprintf("serverless%s", suffix))

	return binPath
}

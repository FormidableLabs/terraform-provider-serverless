package serverless

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	"golang.org/x/mod/sumdb/dirhash"
)

const (
	deploy = iota
	pkg
	build
	remove
)

type getter interface {
	Get(key string) interface{}
}

type Serverless struct {
	binDir     string
	binPath    string
	configDir  string
	config     map[string]interface{}
	packageDir string
	stage      string
	hash       string
	args       []string
}

func (s Serverless) Deploy() error {
	if err := s.exec(deploy); err != nil {
		return err
	}

	return nil
}

func (s Serverless) Package() error {
	if err := s.exec(pkg); err != nil {
		return err
	}

	return nil
}

func (s Serverless) Remove() error {
	if err := s.exec(remove); err != nil {
		return err
	}

	return nil
}

func (s Serverless) exec(command int) error {
	args := []string{
		getCmdString(command), "-s",
		s.stage,
	}

	if command == deploy || command == pkg {
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

func (s *Serverless) loadServerlessConfig() error {
	config := make(map[string]interface{})

	cmd := exec.Command(s.binPath, "print", "--format", "json")
	cmd.Dir = s.configDir

	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("%v\n%w", string(output), err)
	}

	if err = json.Unmarshal(output, &config); err != nil {
		return err
	}

	s.config = config

	return nil
}

func (s *Serverless) Hash() (changed bool, err error) {
	serviceName, ok := s.config["service"].(string)

	if !ok {
		return false, errors.New("service name was not found in serverless config")
	}

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

func NewServerless(resource getter) (*Serverless, error) {
	resourceArgs := resource.Get("args").([]interface{})
	args := make([]string, len(resourceArgs))

	for _, arg := range resourceArgs {
		args = append(args, arg.(string))
	}

	binDir := resource.Get("serverless_bin_dir").(string)
	configDir := resource.Get("config_dir").(string)

	s := &Serverless{
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

	if _, err := s.Hash(); err != nil {
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

func getCmdString(cmd int) string {
	switch cmd {
	case deploy:
		return "deploy"
	case pkg:
		return "package"
	case build:
		return "build"
	case remove:
		return "remove"
	default:
		return ""
	}
}

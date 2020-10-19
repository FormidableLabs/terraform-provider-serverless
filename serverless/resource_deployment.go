package serverless

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/hashicorp/terraform-plugin-sdk/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

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
			// .serverless, but this can be overriden (for example to .terraform-serverless
			// to avoid an issue where the CLI deletes the .serverless directory after deploy,
			// even with --package.)
			// Note that the provider requires out-of-band packaging.
			// So in case a custom package_dir is given, users should package
			// their code with `sls package --package .terraform-serverless`.
			//
			// NOTE: the path you provide must be RELATIVE to your `config_dir` since the
			// --package flag in the CLI does not support absolute paths.
			"package_dir": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
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
			"env": &schema.Schema{
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
			},
			"package_hash": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"aws_config": &schema.Schema{
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"account_id": {
							Type:     schema.TypeString,
							Required: true,
						},
						"caller_arn": {
							Type:     schema.TypeString,
							Required: true,
						},
						"caller_user": {
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
		},

		// Only trigger a deploy if either the Serverless config or Serverless zip archive has changed.
		// `sls package` isn't deterministic according to experiments, so in practice this means that
		// we only deploy after the user has run `sls package` again.
		CustomizeDiff: customdiff.ComputedIf("package_hash", func(d *schema.ResourceDiff, meta interface{}) bool {
			serverless, err := NewServerless(d)

			if err != nil {
				return false
			}

			changed, err := serverless.Hash()

			if err != nil {
				return false
			}

			return changed
		}),
	}
}

func resourceDeploymentCreate(d *schema.ResourceData, m interface{}) error {
	serverless, err := NewServerless(d)

	if err != nil {
		return err
	}

	serviceName, ok := serverless.config["service"].(string)

	if !ok {
		return errors.New("service name was not found in serverless config")
	}

	d.SetId(serviceName)

	if err := d.Set("package_hash", serverless.hash); err != nil {
		return err
	}

	if err := serverless.Deploy(); err != nil {
		return err
	}

	return resourceDeploymentRead(d, m)
}

func resourceDeploymentRead(d *schema.ResourceData, m interface{}) error {
	id := d.Id()
	stage := d.Get("stage").(string)

	sess := session.Must(session.NewSession())
	creds, awsErr := loadAWSCredentials(d)
	if awsErr != nil {
		return awsErr
	}

	cf := cloudformation.New(sess, &aws.Config{Credentials: creds})

	_, err := cf.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: aws.String(strings.Join([]string{id, stage}, "-")),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "ValidationError" && strings.Contains(aerr.Message(), "does not exist") {
				d.SetId("")
				return err
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
		"env",
		"serverless_bin_dir",
		"package_hash",
	)

	if shouldChange {
		return resourceDeploymentCreate(d, m)
	}

	return resourceDeploymentRead(d, m)
}

func resourceDeploymentDelete(d *schema.ResourceData, m interface{}) error {
	serverless, err := NewServerless(d)

	if err != nil {
		return err
	}

	if err := serverless.Remove(); err != nil {
		return err
	}

	d.SetId("")

	return nil
}

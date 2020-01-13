package serverlessv2

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/hashicorp/terraform-plugin-sdk/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceServerless() *schema.Resource {
	return &schema.Resource{
		Create: resourceServerlessCreate,
		Read:   resourceServerlessRead,
		// Update: resourceDeploymentUpdate,
		// Delete: resourceDeploymentDelete,

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
			serverless, err := newServerless(d)

			if err != nil {
				return false
			}

			changed, err := serverless.rehash()

			if err != nil {
				return false
			}

			return changed
		}),
	}
}

func resourceServerlessCreate(d *schema.ResourceData, m interface{}) error {
	serverless, err := newServerless(d)

	if err != nil {
		return err
	}

	serviceName := serverless.config["service"].(string) // TODO: possibly check 2nd return for existence + handle err

	d.SetId(serviceName)

	if err := d.Set("package_hash", serverless.hash); err != nil {
		return err
	}

	if err := serverless.run("deploy"); err != nil {
		return err
	}

	return resourceServerlessRead(d, m)
}

func resourceServerlessRead(d *schema.ResourceData, m interface{}) error {
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

package serverless

import (
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
)

func loadAWSCredentials(resource getter) (*credentials.Credentials, error) {
	configs := resource.Get("aws_config").([]interface{})
	if len(configs) == 0 || configs[0] == nil {
		return nil, nil
	}
	awsConfig := configs[0].(map[string]interface{})

	accountId := awsConfig["account_id"].(string)
	callerArn := awsConfig["caller_arn"].(string)

	re := regexp.MustCompile(`.*assumed-role/([\w+=,.@-]+)/.*`)
	var role string
	if result := re.FindStringSubmatch(callerArn); result != nil {
		role = result[1]
	} else {
		return nil, fmt.Errorf("Could not parse role name from callerArn %s", callerArn)
	}

	roleArn := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, role)

	sess := session.Must(session.NewSession())
	return stscreds.NewCredentials(sess, roleArn), nil
}

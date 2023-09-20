package awsx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// MustGetConfig panics if it fails to return a valid aws config.
// Following environmental variables are recognized:
//   - AWS_ENDPOINT
//   - AWS_DEBUG
//   - AWS_REGION
//   - AWS_DEFAULT_REGION
func MustGetConfig() aws.Config {
	config, err := GetConfig(
		os.Getenv("AWS_ENDPOINT"),
		os.Getenv("AWS_DEBUG"),
		os.Getenv("AWS_REGION"),
		os.Getenv("AWS_DEFAULT_REGION"),
	)
	if err != nil {
		log.Fatal("get AWS config: ", err)
	}
	return config
}

// GetConfig returns an instance of aws.Config configured with an AWS region
func GetConfig(
	awsEndpoint,
	awsDebug,
	awsRegion,
	awsDefaultRegion string,
) (aws.Config, error) {
	region := DetectRegion(awsRegion, awsDefaultRegion)

	var optFns = []func(options *config.LoadOptions) error{
		config.WithRegion(region),
	}

	if awsEndpoint != "" {
		optFns = append(optFns, config.WithEndpointResolverWithOptions(newEndpointResolver(awsEndpoint)))
	}

	if awsDebug != "" {
		optFns = append(
			optFns,
			config.WithClientLogMode(aws.LogRequestWithBody|aws.LogResponseWithBody|aws.LogDeprecatedUsage),
		)
	}

	return config.LoadDefaultConfig(
		context.Background(),
		optFns...,
	)
}

// DetectRegion automatically discovers the AWS region from the runtime environment and d
func DetectRegion(awsRegion, awsDefaultRegion string) string {
	region := awsRegion

	if region == "" {
		region = awsDefaultRegion
	}

	if region == "" {
		ctx, to := context.WithTimeout(context.Background(), time.Second)
		defer to()

		// If running in Elastic Container, the AZ can be inferred from the metadata endpoint,
		// otherwise fetch from EC2 metadata endpoint
		var az string
		var err error
		if endpoint, ok := os.LookupEnv("ECS_CONTAINER_METADATA_URI_V4"); ok {
			az, err = lookupAvailabilityZoneFromECSMetadataEndpoint(ctx, endpoint)
		} else {
			az, err = lookupAvailabilityZoneFromEC2MetadataEndpoint(ctx)
		}

		if err == nil {
			region = strings.TrimSpace(az)
			if len(region) > 0 {
				region = region[0 : len(region)-1]
			}
		}
	}

	if region == "" {
		region = "us-east-1"
	}

	return region
}

func lookupAvailabilityZoneFromEC2MetadataEndpoint(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(
		ctx,
		"GET",
		"http://169.254.169.254/latest/meta-data/placement/availability-zone",
		nil,
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	return string(data), readErr
}

// https://docs.aws.amazon.com/AmazonECS/latest/userguide/task-metadata-endpoint-v4-fargate.html
func lookupAvailabilityZoneFromECSMetadataEndpoint(ctx context.Context, endpoint string) (string, error) {
	req, _ := http.NewRequestWithContext(
		ctx,
		"GET",
		endpoint+"/task",
		nil,
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return "", readErr
	}

	var payload map[string]any
	if unmarshalErr := json.Unmarshal(data, &payload); unmarshalErr != nil {
		return "", unmarshalErr
	}

	if v, ok := payload["AvailabilityZone"]; ok {
		return v.(string), nil
	}

	return "", errors.New("couldn't determine AZ from ECS metadata")
}

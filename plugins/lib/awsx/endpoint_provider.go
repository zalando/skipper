package awsx

import (
	"github.com/aws/aws-sdk-go-v2/aws"
)

func newEndpointResolver(endpoint string) aws.EndpointResolverWithOptionsFunc {
	// Create a custom resolver, allowing for compatibility with localstack
	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           endpoint,
				SigningRegion: region,
			}, nil
		},
	)
	return customResolver
}

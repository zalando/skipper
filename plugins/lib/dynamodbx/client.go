package dynamodbx

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func New(cfg aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(cfg)
}

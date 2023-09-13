package main

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type repo struct {
	client *dynamodb.Client
	table  string
}

func NewRepo(table string) *repo {
	cfg, _ := config.LoadDefaultConfig(context.Background())
	client := dynamodb.NewFromConfig(cfg)

	return &repo{
		client: client,
		table:  table,
	}
}

type AttestationModel struct {
	UDID            string
	Challenge       []byte
	CreatedAt       time.Time `dynamodbav:",unixtime"`
	UpdatedAt       time.Time `dynamodbav:",unixtime"`
	Platform        string
	Headers         string
	RequestBody     string
	PlatformSuccess bool   `dynamodbav:",omitempty"`
	NonceSuccess    bool   `dynamodbav:",omitempty"`
	DeviceErrorCode string `dynamodbav:",omitempty"`
	GoogleResponse  string `dynamodbav:",omitempty"`
	MuzzError       string `dynamodbav:",omitempty"`
}

func (d *repo) GetAttestationForUDID(udid string) (*AttestationModel, error) {
	item, err := d.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"UDID": &types.AttributeValueMemberS{
				Value: udid,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if len(item.Item) == 0 {
		return nil, nil
	}

	var am AttestationModel
	err = attributevalue.UnmarshalMap(item.Item, &am)
	if err != nil {
		return nil, err
	}

	return &am, nil
}

func (d *repo) CreateAttestationForUDID(udid string, challenge []byte, platform string, headers http.Header, requestBody string) error {
	headerList := map[string]string{}
	for k, v := range headers {
		headerList[k] = strings.Join(v, ",")
	}
	headersJSON, _ := json.Marshal(headerList)

	_, err := d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(d.table),
		Item: map[string]types.AttributeValue{
			"UDID": &types.AttributeValueMemberS{
				Value: udid,
			},
			"Challenge": &types.AttributeValueMemberB{
				Value: challenge,
			},
			"Platform": &types.AttributeValueMemberS{
				Value: platform,
			},
			"CreatedAt": &types.AttributeValueMemberN{
				Value: strconv.Itoa(int(time.Now().Unix())),
			},
			"UpdatedAt": &types.AttributeValueMemberN{
				Value: strconv.Itoa(int(time.Now().Unix())),
			},
			"Headers": &types.AttributeValueMemberS{
				Value: string(headersJSON),
			},
			"RequestBody": &types.AttributeValueMemberS{
				Value: requestBody,
			},
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (d *repo) UpdateAttestationForUDID(am *AttestationModel) error {
	updateBuilder := expression.UpdateBuilder{}
	updateBuilder.Set(expression.Name("UpdatedAt"), expression.Value(am.UpdatedAt.Unix()))

	if am.PlatformSuccess {
		updateBuilder.Set(expression.Name("PlatformSuccess"), expression.Value(am.PlatformSuccess))
	}

	if am.NonceSuccess {
		updateBuilder.Set(expression.Name("NonceSuccess"), expression.Value(am.NonceSuccess))
	}

	if am.DeviceErrorCode != "" {
		updateBuilder.Set(expression.Name("DeviceErrorCode"), expression.Value(am.DeviceErrorCode))
	}

	if am.GoogleResponse != "" {
		updateBuilder.Set(expression.Name("GoogleResponse"), expression.Value(am.GoogleResponse))
	}

	if am.MuzzError != "" {
		updateBuilder.Set(expression.Name("MuzzError"), expression.Value(am.MuzzError))
	}

	expr, err := expression.NewBuilder().WithUpdate(updateBuilder).Build()
	if err != nil {
		return err
	}

	_, err = d.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(d.table),
		Key: map[string]types.AttributeValue{
			"UDID": &types.AttributeValueMemberS{
				Value: am.UDID,
			},
		},
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	})
	if err != nil {
		return err
	}

	return nil
}

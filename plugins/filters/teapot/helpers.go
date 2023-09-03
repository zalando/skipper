package main

import (
	"crypto/md5"
	"encoding/hex"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var (
	s3client *s3.S3
)

func init() {
	sess := session.Must(session.NewSession())
	s3client = s3.New(sess)
}

func fetchS3File(bucket string, key string) ([]byte, string, error) {
	result, getObjectErr := s3client.GetObject(
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
	)
	if getObjectErr != nil {
		return nil, "", getObjectErr
	}

	body, readErr := io.ReadAll(result.Body)
	defer result.Body.Close()
	if readErr != nil {
		return nil, "", readErr
	}

	hash := md5.New()
	_, _ = hash.Write(body)

	return body, hex.EncodeToString(hash.Sum(nil)), nil
}

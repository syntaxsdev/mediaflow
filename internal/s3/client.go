package s3

import (
	"context"
	"fmt"
	"io"

	utils "mediaflow/internal"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	s3Client *s3.Client
	bucket   string
}

func NewClient(ctx context.Context, region, bucket, accessKey, secretKey string) (*Client, error) {
	var cfg aws.Config
	var err error

	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else {
		// Quit if no credentials are provided
		utils.Shutdown("No AWS credentials provided, exiting...")
		// cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}

	if err != nil {
		return nil, err
	}

	return &Client{
		s3Client: s3.NewFromConfig(cfg),
		bucket:   bucket,
	}, nil
}

func (c *Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	fmt.Println("Getting object", key)
	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func (c *Client) PutObject(ctx context.Context, key string, body io.Reader) error {
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

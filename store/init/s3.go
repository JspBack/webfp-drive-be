package initializer

import (
	"context"
	"fmt"

	"github.com/JspBack/webfp-drive/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func newObjectStore(ctx context.Context, cfg config.Config) (*s3.Client, *s3.PresignClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.R2AccessKeyID,
			cfg.R2SecretAccessKey,
			"",
		)),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load R2 configuration: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.R2BucketName)}); err != nil {
		return nil, nil, fmt.Errorf("healthcheck R2 bucket %q: %w", cfg.R2BucketName, err)
	}
	return client, s3.NewPresignClient(client), nil
}

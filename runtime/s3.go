//go:build !wasm

package runtime

import (
	"context"
	"io"
	"bytes"
	"strings"
	"time"
	"fmt"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go/middleware"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

var (
	defaultS3Client *s3.Client
)

type S3Client struct {
	client *s3.Client
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func S3Init(endpointVal, accessKeyVal, secretKeyVal interface{}) interface{} {
	endpoint := asString(endpointVal)
	accessKey := asString(accessKeyVal)
	secretKey := asString(secretKeyVal)

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("Failed to load S3 config: %s", err.Error())}
	}

	cli := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	defaultS3Client = cli
	return &S3Client{client: cli}
}

// Global Client Functions
func S3Put(bucketVal, keyVal, data interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3PutHelper(defaultS3Client, asString(bucketVal), asString(keyVal), data)
}

func S3Get(bucketVal, keyVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3GetHelper(defaultS3Client, asString(bucketVal), asString(keyVal))
}

func S3Delete(bucketVal, keyVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3DeleteHelper(defaultS3Client, asString(bucketVal), asString(keyVal))
}

func S3List(bucketVal interface{}, args ...interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3ListHelper(defaultS3Client, asString(bucketVal), args...)
}

func S3At(bucketVal, keyVal, timestampVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3AtHelper(defaultS3Client, asString(bucketVal), asString(keyVal), asString(timestampVal))
}

func S3Search(bucketVal, queryVal interface{}, args ...interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3SearchHelper(defaultS3Client, asString(bucketVal), asString(queryVal), args...)
}

func S3CreateBucket(bucketVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3CreateBucketHelper(defaultS3Client, asString(bucketVal))
}

func S3DeleteBucket(bucketVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3DeleteBucketHelper(defaultS3Client, asString(bucketVal))
}

func S3SetBucketVersioning(bucketVal, statusVal interface{}) interface{} {
	if defaultS3Client == nil {
		return [2]interface{}{nil, "S3 client is not initialized. Call s3.init(...) first."}
	}
	return s3SetBucketVersioningHelper(defaultS3Client, asString(bucketVal), asString(statusVal))
}

// Client Object Methods
func (c *S3Client) Put(bucketVal, keyVal, data interface{}) interface{} {
	return s3PutHelper(c.client, asString(bucketVal), asString(keyVal), data)
}

func (c *S3Client) Get(bucketVal, keyVal interface{}) interface{} {
	return s3GetHelper(c.client, asString(bucketVal), asString(keyVal))
}

func (c *S3Client) Delete(bucketVal, keyVal interface{}) interface{} {
	return s3DeleteHelper(c.client, asString(bucketVal), asString(keyVal))
}

func (c *S3Client) List(bucketVal interface{}, args ...interface{}) interface{} {
	return s3ListHelper(c.client, asString(bucketVal), args...)
}

func (c *S3Client) At(bucketVal, keyVal, timestampVal interface{}) interface{} {
	return s3AtHelper(c.client, asString(bucketVal), asString(keyVal), asString(timestampVal))
}

func (c *S3Client) Search(bucketVal, queryVal interface{}, args ...interface{}) interface{} {
	return s3SearchHelper(c.client, asString(bucketVal), asString(queryVal), args...)
}

func (c *S3Client) CreateBucket(bucketVal interface{}) interface{} {
	return s3CreateBucketHelper(c.client, asString(bucketVal))
}

func (c *S3Client) DeleteBucket(bucketVal interface{}) interface{} {
	return s3DeleteBucketHelper(c.client, asString(bucketVal))
}

func (c *S3Client) SetBucketVersioning(bucketVal, statusVal interface{}) interface{} {
	return s3SetBucketVersioningHelper(c.client, asString(bucketVal), asString(statusVal))
}

// Helper middleware for custom query parameters
func addQueryParameter(key, value string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("AddQueryParam_"+key, func(
			ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler,
		) (middleware.BuildOutput, middleware.Metadata, error) {
			if req, ok := in.Request.(*smithyhttp.Request); ok {
				values := req.URL.Query()
				values.Add(key, value)
				req.URL.RawQuery = values.Encode()
			}
			return next.HandleBuild(ctx, in)
		}), middleware.After)
	}
}

// Core Helpers
func s3PutHelper(cli *s3.Client, bucket, key string, data interface{}) interface{} {
	var body io.Reader
	var length int64
	switch v := data.(type) {
	case []byte:
		body = bytes.NewReader(v)
		length = int64(len(v))
	case string:
		body = strings.NewReader(v)
		length = int64(len(v))
	default:
		b, err := json.Marshal(v)
		if err != nil {
			body = strings.NewReader(fmt.Sprint(v))
			length = int64(len(fmt.Sprint(v)))
		} else {
			body = bytes.NewReader(b)
			length = int64(len(b))
		}
	}

	_, err := cli.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(length),
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return "ok"
}

func s3GetHelper(cli *s3.Client, bucket, key string) interface{} {
	resp, err := cli.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return string(data)
}

func s3DeleteHelper(cli *s3.Client, bucket, key string) interface{} {
	_, err := cli.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return "ok"
}

func s3ListHelper(cli *s3.Client, bucket string, args ...interface{}) interface{} {
	var prefix *string
	if len(args) > 0 {
		if p, ok := args[0].(string); ok && p != "" {
			prefix = aws.String(p)
		}
	}

	resp, err := cli.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: prefix,
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}

	var results []interface{}
	for _, obj := range resp.Contents {
		item := map[string]interface{}{
			"key":           aws.ToString(obj.Key),
			"size":          aws.ToInt64(obj.Size),
			"last_modified": aws.ToTime(obj.LastModified).Format(time.RFC3339),
			"etag":          strings.Trim(aws.ToString(obj.ETag), `"`),
		}
		results = append(results, item)
	}
	return results
}

func s3AtHelper(cli *s3.Client, bucket, key, timestamp string) interface{} {
	resp, err := cli.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, addQueryParameter("at", timestamp))
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return string(data)
}

func s3SearchHelper(cli *s3.Client, bucket, query string, args ...interface{}) interface{} {
	maxResults := "10"
	if len(args) > 0 {
		maxResults = fmt.Sprint(args[0])
	}

	resp, err := cli.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}, func(o *s3.Options) {
		o.APIOptions = append(o.APIOptions, addQueryParameter("query", "semantic"))
		o.APIOptions = append(o.APIOptions, addQueryParameter("q", query))
		o.APIOptions = append(o.APIOptions, addQueryParameter("max-results", maxResults))
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}

	var results []interface{}
	for _, obj := range resp.Contents {
		item := map[string]interface{}{
			"key":           aws.ToString(obj.Key),
			"size":          aws.ToInt64(obj.Size),
			"last_modified": aws.ToTime(obj.LastModified).Format(time.RFC3339),
			"etag":          strings.Trim(aws.ToString(obj.ETag), `"`),
		}
		results = append(results, item)
	}
	return results
}

func s3CreateBucketHelper(cli *s3.Client, bucket string) interface{} {
	_, err := cli.CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return "ok"
}

func s3DeleteBucketHelper(cli *s3.Client, bucket string) interface{} {
	_, err := cli.DeleteBucket(context.Background(), &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return "ok"
}

func s3SetBucketVersioningHelper(cli *s3.Client, bucket, status string) interface{} {
	_, err := cli.PutBucketVersioning(context.Background(), &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{
			Status: types.BucketVersioningStatus(status),
		},
	})
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}
	return "ok"
}


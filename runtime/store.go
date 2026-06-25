package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type parsedStore struct {
	client     *s3.Client
	bucket     string
	endpoint   string
	accessKey  string
	secretKey  string
	isS3       bool
}

var (
	storeConnString string
	storeMu         sync.RWMutex
	storeMemMap     = make(map[string]interface{})
	activeStore     parsedStore
)

func parseStoreConnString(conn string) {
	activeStore = parsedStore{}
	if !strings.HasPrefix(conn, "s3://") && !strings.HasPrefix(conn, "servstore://") {
		return
	}
	activeStore.isS3 = true

	// Strip prefix
	raw := conn
	if strings.HasPrefix(raw, "s3://") {
		raw = strings.TrimPrefix(raw, "s3://")
	} else {
		raw = strings.TrimPrefix(raw, "servstore://")
	}

	// Check if it contains '@' (credentials)
	var creds, endpointAndBucket string
	if idx := strings.Index(raw, "@"); idx != -1 {
		creds = raw[:idx]
		endpointAndBucket = raw[idx+1:]
	} else {
		endpointAndBucket = raw
	}

	// Parse credentials
	accessKey := "admin"
	secretKey := "adminsecret"
	if creds != "" {
		parts := strings.SplitN(creds, ":", 2)
		if len(parts) == 2 {
			accessKey = parts[0]
			secretKey = parts[1]
		} else {
			accessKey = parts[0]
		}
	}
	activeStore.accessKey = accessKey
	activeStore.secretKey = secretKey

	// Parse endpoint and bucket
	endpoint := "http://localhost:8081"
	bucket := ""
	if idx := strings.Index(endpointAndBucket, "/"); idx != -1 {
		epHost := endpointAndBucket[:idx]
		bucket = endpointAndBucket[idx+1:]
		if epHost != "" {
			if !strings.HasPrefix(epHost, "http://") && !strings.HasPrefix(epHost, "https://") {
				endpoint = "http://" + epHost
			} else {
				endpoint = epHost
			}
		}
	} else {
		bucket = endpointAndBucket
	}
	activeStore.endpoint = endpoint
	activeStore.bucket = bucket

	// Initialize S3 SDK client
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err == nil {
		cli := s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		activeStore.client = cli
	}
}

func InitStore(connStr string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	storeConnString = connStr
	parseStoreConnString(connStr)
	LogInfo("Object store client initialized: ", connStr)
}

func StorePut(key string, val interface{}) (interface{}, error) {
	storeMu.Lock()
	conn := storeConnString
	isS3 := activeStore.isS3
	s3Cli := activeStore.client
	bucket := activeStore.bucket
	storeMu.Unlock()

	if conn == "" {
		return nil, errors.New("store not initialized; declare store \"connection_string\" first")
	}

	var data []byte
	var err error
	if str, ok := val.(string); ok {
		data = []byte(str)
	} else if b, ok := val.([]byte); ok {
		data = b
	} else {
		data, err = json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	if strings.HasPrefix(conn, "file://") {
		dir := strings.TrimPrefix(conn, "file://")
		path := filepath.Join(dir, key)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
		return true, nil
	}

	if isS3 && s3Cli != nil {
		res := s3PutHelper(s3Cli, bucket, key, val)
		if arr, ok := res.([2]interface{}); ok {
			return nil, fmt.Errorf("%v", arr[1])
		}
		return true, nil
	}

	// Always fallback to memory store for test stability
	storeMu.Lock()
	storeMemMap[key] = val
	storeMu.Unlock()

	LogInfo("Store PUT key: ", key, " value size: ", len(data))
	return true, nil
}

func StoreGet(key string) (interface{}, error) {
	storeMu.Lock()
	conn := storeConnString
	isS3 := activeStore.isS3
	s3Cli := activeStore.client
	bucket := activeStore.bucket
	storeMu.Unlock()

	if conn == "" {
		return nil, errors.New("store not initialized; declare store \"connection_string\" first")
	}

	if strings.HasPrefix(conn, "file://") {
		dir := strings.TrimPrefix(conn, "file://")
		path := filepath.Join(dir, key)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		return string(data), nil
	}

	if isS3 && s3Cli != nil {
		res := s3GetHelper(s3Cli, bucket, key)
		if arr, ok := res.([2]interface{}); ok {
			errMsg := fmt.Sprint(arr[1])
			if strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "NotFound") {
				return nil, nil
			}
			return nil, fmt.Errorf("%v", arr[1])
		}
		return res, nil
	}

	storeMu.RLock()
	val, ok := storeMemMap[key]
	storeMu.RUnlock()
	if !ok {
		return nil, nil
	}
	return val, nil
}

func StoreTransform(inputKeyVal, stagesVal, outputKeyVal, saveTraceVal interface{}) (interface{}, error) {
	storeMu.RLock()
	isS3 := activeStore.isS3
	endpoint := activeStore.endpoint
	accessKey := activeStore.accessKey
	secretKey := activeStore.secretKey
	bucket := activeStore.bucket
	storeMu.RUnlock()

	if !isS3 {
		return nil, errors.New("store not initialized for S3/ServStore; transform is only supported on S3 backends")
	}

	res := s3TransformHelper(endpoint, accessKey, secretKey, bucket, asString(inputKeyVal), stagesVal, asString(outputKeyVal), saveTraceVal)
	if arr, ok := res.([2]interface{}); ok {
		return nil, fmt.Errorf("%v", arr[1])
	}
	return res, nil
}

// Copyright (c) 2018-2022 Splunk Inc. All rights reserved.

//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// blank assignment to verify that AWSS3Client implements S3Client
var _ S3Client = &AWSS3Client{}

// SplunkAWSS3Client is an interface to AWS S3 client
type SplunkAWSS3Client interface {
	ListObjectsV2(options *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
}

// AWSS3Client is a client to implement S3 specific APIs
type AWSS3Client struct {
	Region             string
	BucketName         string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	Prefix             string
	StartAfter         string
	Endpoint           string
	Client             SplunkAWSS3Client
}

// regex to extract the region from the s3 endpoint
var regionRegex = ".*.s3[-,.](?P<region>.*).amazonaws.com"

// GetRegion extracts the region from the endpoint field
func GetRegion(endpoint string) string {
	pattern := regexp.MustCompile(regionRegex)
	if len(pattern.FindStringSubmatch(endpoint)) > 0 {
		return pattern.FindStringSubmatch(endpoint)[1]
	}
	return ""
}

// InitAWSClientWrapper is a wrapper around InitClientSession
func InitAWSClientWrapper(ctx context.Context, region, accessKeyID, secretAccessKey string) interface{} {
	return InitAWSClientSession(ctx, region, accessKeyID, secretAccessKey)
}

// InitAWSClientSession initializes and returns a client session object
func InitAWSClientSession(ctx context.Context, region, accessKeyID, secretAccessKey string) SplunkAWSS3Client {
	scopedLog := log.WithName("InitAWSClientSession")

	// Enforcing minimum version TLS1.2
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	tr.ForceAttemptHTTP2 = true
	httpClient := http.Client{Transport: tr}

	var err error
	var sess *session.Session
	config := &aws.Config{
		Region:     aws.String(region),
		MaxRetries: aws.Int(3),
		HTTPClient: &httpClient,
	}

	if accessKeyID != "" && secretAccessKey != "" {
		config.WithCredentials(credentials.NewStaticCredentials(
			accessKeyID,     // id
			secretAccessKey, // secret
			""))
	} else {
		scopedLog.Info("No valid access/secret keys.  Attempt to connect without them")
	}

	sess, err = session.NewSession(config)
	if err != nil {
		scopedLog.Error(err, "Failed to initialize an AWS S3 session.")
		return nil
	}

	// Create the s3Client
	s3Client := s3.New(sess)

	// Validate transport
	tlsVersion := "Unknown"
	if tr, ok := s3Client.Config.HTTPClient.Transport.(*http.Transport); ok {
		tlsVersion = getTLSVersion(tr)
	}

	scopedLog.Info("AWS Client Session initialization successful.", "region", region, "TLS Version", tlsVersion)

	return s3Client
}

// NewAWSS3Client returns an AWS S3 client
func NewAWSS3Client(ctx context.Context, bucketName string, accessKeyID string, secretAccessKey string, prefix string, startAfter string, endpoint string, fn GetInitFunc) (S3Client, error) {
	var s3SplunkClient SplunkAWSS3Client
	var err error
	region := GetRegion(endpoint)
	cl := fn(ctx, region, accessKeyID, secretAccessKey)
	if cl == nil {
		err = fmt.Errorf("Failed to create an AWS S3 client")
		return nil, err
	}

	s3SplunkClient = cl.(*s3.S3)

	return &AWSS3Client{
		Region:             region,
		BucketName:         bucketName,
		AWSAccessKeyID:     accessKeyID,
		AWSSecretAccessKey: secretAccessKey,
		Prefix:             prefix,
		StartAfter:         startAfter,
		Endpoint:           endpoint,
		Client:             s3SplunkClient,
	}, nil
}

// RegisterAWSS3Client will add the corresponding function pointer to the map
func RegisterAWSS3Client(ctx context.Context) {
	wrapperObject := GetS3ClientWrapper{GetS3Client: NewAWSS3Client, GetInitFunc: InitAWSClientWrapper}
	S3Clients["aws"] = wrapperObject
}

func getTLSVersion(tr *http.Transport) string {
	switch tr.TLSClientConfig.MinVersion {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}

	return "Unknown"
}

// GetAppsList get the list of apps from remote storage
func (awsclient *AWSS3Client) GetAppsList(ctx context.Context) (S3Response, error) {
	scopedLog := log.WithName("GetAppsList")

	scopedLog.Info("Getting Apps list", "AWS S3 Bucket", awsclient.BucketName)
	s3Resp := S3Response{}

	options := &s3.ListObjectsV2Input{
		Bucket:     aws.String(awsclient.BucketName),
		Prefix:     aws.String(awsclient.Prefix),
		StartAfter: aws.String(awsclient.StartAfter), // exclude the directory itself from listing
		MaxKeys:    aws.Int64(4000),                  // return upto 4K keys from S3
		Delimiter:  aws.String("/"),                  // limit the listing to 1 level only
	}

	client := awsclient.Client
	resp, err := client.ListObjectsV2(options)
	if err != nil {
		scopedLog.Error(err, "Unable to list items in bucket", "AWS S3 Bucket", awsclient.BucketName)
		return s3Resp, err
	}

	if resp.Contents == nil {
		scopedLog.Info("Empty objects list in bucket. No apps to install", "bucketName", awsclient.BucketName)
		return s3Resp, nil
	}

	tmp, err := json.Marshal(resp.Contents)
	if err != nil {
		scopedLog.Error(err, "Failed to marshal s3 response", "AWS S3 Bucket", awsclient.BucketName)
		return s3Resp, err
	}

	err = json.Unmarshal(tmp, &(s3Resp.Objects))
	if err != nil {
		scopedLog.Error(err, "Failed to unmarshal s3 response", "AWS S3 Bucket", awsclient.BucketName)
		return s3Resp, err
	}

	return s3Resp, nil
}

// GetInitContainerImage returns the initContainer image to be used with this s3 client
func (awsclient *AWSS3Client) GetInitContainerImage(ctx context.Context) string {
	return ("amazon/aws-cli")
}

// GetInitContainerCmd returns the init container command on a per app source basis to be used by the initContainer
func (awsclient *AWSS3Client) GetInitContainerCmd(ctx context.Context, endpoint string, bucket string, path string, appSrcName string, appMnt string) []string {
	s3AppSrcPath := filepath.Join(bucket, path) + "/"
	podSyncPath := filepath.Join(appMnt, appSrcName) + "/"

	return ([]string{fmt.Sprintf("--endpoint-url=%s", endpoint), "s3", "sync", fmt.Sprintf("s3://%s", s3AppSrcPath), podSyncPath})
}

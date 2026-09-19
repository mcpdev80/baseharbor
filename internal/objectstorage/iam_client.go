package objectstorage

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

const iamAPIVersion = "2010-05-08"

type IAMClient struct {
	endpoint    string
	client      *http.Client
	credentials application.ObjectStorageCredentials
}

type IAMAccessKey struct {
	AccessKeyID     string
	SecretAccessKey string
}

func NewIAMClient(endpoint string, client *http.Client, credentials AdminCredentials) (*IAMClient, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("SeaweedFS IAM endpoint is required")
	}
	if client == nil {
		return nil, errors.New("SeaweedFS IAM HTTP client is required")
	}
	if strings.TrimSpace(credentials.AccessKeyID) == "" || strings.TrimSpace(credentials.SecretAccessKey) == "" {
		return nil, errors.New("SeaweedFS IAM admin credentials are required")
	}
	return &IAMClient{
		endpoint: endpoint,
		client:   client,
		credentials: application.ObjectStorageCredentials{
			AccessKeyID:     credentials.AccessKeyID,
			SecretAccessKey: credentials.SecretAccessKey,
		},
	}, nil
}

func (c *IAMClient) CreateUser(ctx context.Context, user string) error {
	values := url.Values{"Action": {"CreateUser"}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	if status == http.StatusConflict && iamErrorCode(body) == "EntityAlreadyExists" {
		return nil
	}
	return iamStatusError("create user", status, body)
}

func (c *IAMClient) CreateAccessKey(ctx context.Context, user string) (IAMAccessKey, error) {
	values := url.Values{"Action": {"CreateAccessKey"}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return IAMAccessKey{}, err
	}
	if status != http.StatusOK {
		return IAMAccessKey{}, iamStatusError("create access key", status, body)
	}
	var response struct {
		Result struct {
			AccessKey struct {
				AccessKeyID     string `xml:"AccessKeyId"`
				SecretAccessKey string `xml:"SecretAccessKey"`
			} `xml:"AccessKey"`
		} `xml:"CreateAccessKeyResult"`
	}
	if err := xml.Unmarshal(body, &response); err != nil {
		return IAMAccessKey{}, errors.New("decode SeaweedFS IAM CreateAccessKey response")
	}
	result := IAMAccessKey{
		AccessKeyID:     strings.TrimSpace(response.Result.AccessKey.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(response.Result.AccessKey.SecretAccessKey),
	}
	if result.AccessKeyID == "" || result.SecretAccessKey == "" {
		return IAMAccessKey{}, errors.New("SeaweedFS IAM CreateAccessKey response is incomplete")
	}
	return result, nil
}

func (c *IAMClient) ListAccessKeys(ctx context.Context, user string) ([]string, error) {
	values := url.Values{"Action": {"ListAccessKeys"}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, iamStatusError("list access keys", status, body)
	}
	var response struct {
		Result struct {
			Metadata []struct {
				AccessKeyID string `xml:"AccessKeyId"`
			} `xml:"AccessKeyMetadata>member"`
		} `xml:"ListAccessKeysResult"`
	}
	if err := xml.Unmarshal(body, &response); err != nil {
		return nil, errors.New("decode SeaweedFS IAM ListAccessKeys response")
	}
	keys := make([]string, 0, len(response.Result.Metadata))
	for _, item := range response.Result.Metadata {
		if key := strings.TrimSpace(item.AccessKeyID); key != "" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (c *IAMClient) DeleteAccessKey(ctx context.Context, user, accessKeyID string) error {
	values := url.Values{"AccessKeyId": {accessKeyID}, "Action": {"DeleteAccessKey"}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	code := iamErrorCode(body)
	if status == http.StatusNotFound || code == "NoSuchEntity" {
		return nil
	}
	return iamStatusError("delete access key", status, body)
}

func (c *IAMClient) PutUserPolicy(ctx context.Context, user, policyName, policyDocument string) error {
	values := url.Values{"Action": {"PutUserPolicy"}, "PolicyDocument": {policyDocument}, "PolicyName": {policyName}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return iamStatusError("put user policy", status, body)
	}
	return nil
}

func (c *IAMClient) DeleteUserPolicy(ctx context.Context, user, policyName string) error {
	values := url.Values{"Action": {"DeleteUserPolicy"}, "PolicyName": {policyName}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	code := iamErrorCode(body)
	if status == http.StatusNotFound || code == "NoSuchEntity" {
		return nil
	}
	return iamStatusError("delete user policy", status, body)
}

func (c *IAMClient) DeleteUser(ctx context.Context, user string) error {
	values := url.Values{"Action": {"DeleteUser"}, "UserName": {user}, "Version": {iamAPIVersion}}
	status, body, err := c.query(ctx, values)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	code := iamErrorCode(body)
	if status == http.StatusNotFound || code == "NoSuchEntity" {
		return nil
	}
	return iamStatusError("delete user", status, body)
}

func (c *IAMClient) query(ctx context.Context, values url.Values) (int, []byte, error) {
	payload := []byte(values.Encode())
	return signedAWSRequest(ctx, c.client, c.endpoint, "iam", http.MethodPost, "/", "application/x-www-form-urlencoded; charset=utf-8", c.credentials, payload)
}

func iamErrorCode(body []byte) string {
	var response struct {
		Error struct {
			Code string `xml:"Code"`
		} `xml:"Error"`
	}
	if xml.Unmarshal(body, &response) != nil {
		return ""
	}
	return strings.TrimSpace(response.Error.Code)
}

func iamStatusError(operation string, status int, body []byte) error {
	code := iamErrorCode(body)
	if code == "" {
		return fmt.Errorf("SeaweedFS IAM %s failed with HTTP %d", operation, status)
	}
	return fmt.Errorf("SeaweedFS IAM %s failed with HTTP %d (%s)", operation, status, code)
}

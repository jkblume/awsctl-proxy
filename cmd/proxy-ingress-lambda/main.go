package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
)

// ProxyRequest represents the incoming request from the local proxy
type ProxyRequest struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	Query         string              `json:"query"`
	PrivateApiUrl string              `json:"privateApiUrl"`
}

// ProxyResponse represents the response to send back
type ProxyResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
}

// Handler is the main Lambda function handler
func Handler(ctx context.Context, request ProxyRequest) (*ProxyResponse, error) {
	// Get the private API endpoint from the request
	apiEndpoint := request.PrivateApiUrl
	if apiEndpoint == "" {
		return &ProxyResponse{
			StatusCode: 400,
			Body:       "Missing required privateApiUrl in request",
		}, nil
	}

	// Construct the full URL
	url := fmt.Sprintf("%s%s", apiEndpoint, request.Path)
	if request.Query != "" {
		url = fmt.Sprintf("%s?%s", url, request.Query)
	}

	// Create HTTP client with timeout and skip TLS verification
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Skip certificate verification
		},
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: httpTransport,
	}

	// Create the request
	var bodyReader io.Reader
	if request.Body != "" {
		bodyBytes, err := base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			return &ProxyResponse{
				StatusCode: 400,
				Body:       fmt.Sprintf("failed to decode base64 body: %v", err),
			}, nil
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, request.Method, url, bodyReader)
	if err != nil {
		return &ProxyResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("failed to create HTTP request: %v", err),
		}, nil
	}

	// Set headers from the original request
	for key, values := range request.Headers {
		// Skip host header as it will be set automatically
		lowerKey := strings.ToLower(key)
		if lowerKey == "host" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make the request to the private API Gateway
	resp, err := client.Do(req)
	if err != nil {
		return &ProxyResponse{
			StatusCode: 502,
			Body:       fmt.Sprintf("failed to call private API: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ProxyResponse{
			StatusCode: 500,
			Body:       fmt.Sprintf("failed to read API response: %v", err),
		}, nil
	}

	// Copy response headers
	responseHeaders := make(map[string][]string)
	for key, values := range resp.Header {
		responseHeaders[key] = values
	}

	// Always encode response body as base64
	responseBody := base64.StdEncoding.EncodeToString(respBody)

	// Return the proxied response
	return &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    responseHeaders,
		Body:       responseBody,
	}, nil
}

func main() {
	lambda.Start(Handler)
}

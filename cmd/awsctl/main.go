package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
)

// ProxyRequest represents the request to send to Lambda
type ProxyRequest struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	Query         string              `json:"query"`
	PrivateApiUrl string              `json:"privateApiUrl"`
}

// ProxyResponse represents the response from Lambda
type ProxyResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
}

type Server struct {
	lambdaClient       *lambda.Client
	lambdaFunctionName string
	verbose            bool
}

func NewProxyServer(functionName, region, profile string, verbose bool) (*Server, error) {
	ctx := context.Background()

	// Load AWS configuration
	var awsConfigOptions []func(*config.LoadOptions) error

	// Set region
	if region != "" {
		awsConfigOptions = append(awsConfigOptions, config.WithRegion(region))
	}

	// Set profile if specified
	if profile != "" {
		awsConfigOptions = append(awsConfigOptions, config.WithSharedConfigProfile(profile))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, awsConfigOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	// Create Lambda client
	lambdaClient := lambda.NewFromConfig(awsCfg)

	return &Server{
		lambdaClient:       lambdaClient,
		lambdaFunctionName: functionName,
		verbose:            verbose,
	}, nil
}

func (s *Server) invokeLambda(ctx context.Context, request ProxyRequest) (*ProxyResponse, error) {
	// Marshal the request to JSON
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	if s.verbose {
		log.Printf("Invoking Lambda function %s with payload: %s", s.lambdaFunctionName, string(requestJSON))
	}

	// Invoke Lambda function
	result, err := s.lambdaClient.Invoke(ctx, &lambda.InvokeInput{
		FunctionName: &s.lambdaFunctionName,
		Payload:      requestJSON,
		LogType:      "Tail", // Include logs in response
	})

	if err != nil {
		return nil, fmt.Errorf("invoke Lambda: %w", err)
	}

	// Check if Lambda returned an error
	if result.FunctionError != nil {
		return nil, fmt.Errorf("lambda function error: %s", *result.FunctionError)
	}

	// Parse Lambda response
	var lambdaResp ProxyResponse
	if err := json.Unmarshal(result.Payload, &lambdaResp); err != nil {
		return nil, fmt.Errorf("unmarshal Lambda response: %w", err)
	}

	if s.verbose && result.LogResult != nil {
		log.Printf("Lambda logs: %s", *result.LogResult)
	}

	return &lambdaResp, nil
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	if s.verbose {
		log.Printf("Received %s request to %s", r.Method, r.URL.Path)
	}

	// Get the path parameter which contains everything after /api_url/
	path := r.PathValue("path")
	if path == "" {
		http.Error(w, "Missing path", http.StatusBadRequest)
		return
	}

	// Split on /proxy/ to separate the encoded API URL from the actual path
	parts := strings.Split(path, "/proxy/")
	if len(parts) != 2 {
		http.Error(w, "Invalid path format. Expected: /api_url/<encoded-api-url>/proxy/<path>", http.StatusBadRequest)
		return
	}

	encodedApiUrl := parts[0]
	apiPath := "/" + parts[1]

	// Decode the API URL
	privateApiUrl, err := url.QueryUnescape(encodedApiUrl)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to decode API URL: %v", err), http.StatusBadRequest)
		return
	}

	if s.verbose {
		log.Printf("Target API URL: %s", privateApiUrl)
		log.Printf("API Path: %s", apiPath)
	}

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Convert headers to map[string][]string
	headers := make(map[string][]string)
	for key, values := range r.Header {
		headers[key] = values
	}

	// Encode body as base64 to handle binary data
	bodyEncoded := base64.StdEncoding.EncodeToString(bodyBytes)

	// Prepare proxy request
	proxyReq := ProxyRequest{
		Method:        r.Method,
		Path:          apiPath,
		Headers:       headers,
		Body:          bodyEncoded,
		Query:         r.URL.RawQuery,
		PrivateApiUrl: privateApiUrl,
	}

	// Invoke Lambda function
	ctx := r.Context()
	lambdaResp, err := s.invokeLambda(ctx, proxyReq)
	if err != nil {
		log.Printf("Lambda invocation error: %v", err)
		http.Error(w, fmt.Sprintf("Lambda invocation failed: %v", err), http.StatusBadGateway)
		return
	}

	// Set response headers
	for key, values := range lambdaResp.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(lambdaResp.StatusCode)

	// Decode base64 response body
	responseBody, err := base64.StdEncoding.DecodeString(lambdaResp.Body)
	if err != nil {
		log.Printf("Failed to decode base64 response: %v", err)
		responseBody = []byte(lambdaResp.Body)
	}

	if _, err := w.Write(responseBody); err != nil {
		log.Printf("Failed to write response: %v", err)
	}

	if s.verbose {
		log.Printf("Response: %d", lambdaResp.StatusCode)
	}
}

func runProxy() {
	// Command line flags
	var (
		functionName = flag.String("function", "awsctl-proxy-ingress-lambda", "Lambda function name (required)")
		region       = flag.String("region", "eu-central-1", "AWS region")
		profile      = flag.String("profile", "", "AWS profile to use")
		port         = flag.Int("port", 8001, "Local proxy port")
		verbose      = flag.Bool("verbose", true, "Enable verbose logging")
	)

	flag.Parse()

	// Create proxy server
	proxy, err := NewProxyServer(*functionName, *region, *profile, *verbose)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}

	// Create HTTP server with path parameters
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api_url/{path...}", proxy.handler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	fmt.Println(fmt.Sprintf("Starting to serve on http://localhost:%d", *port))
	fmt.Println(fmt.Sprintf("Proxying requests to lambda function: %s", proxy.lambdaFunctionName))
	fmt.Println(fmt.Sprintf("AWS Region: %s", *region))
	if *profile != "" {
		fmt.Println(fmt.Sprintf("AWS Profile: %s", *profile))
	}
	fmt.Println(fmt.Sprintf("Usage: http://localhost:%d/api_url/<url-encoded-internal-api-url>/proxy/<path>", *port))

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: awsctl <command>")
		fmt.Println("Commands:")
		fmt.Println("  proxy    Start the local proxy server")
		os.Exit(1)
	}

	command := os.Args[1]

	// Remove the command from os.Args so flag.Parse() works correctly
	os.Args = append(os.Args[:1], os.Args[2:]...)

	switch command {
	case "proxy":
		runProxy()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands:")
		fmt.Println("  proxy    Start the local proxy server")
		os.Exit(1)
	}
}

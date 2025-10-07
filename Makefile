install_awsctl:
	cd cmd/awsctl && go install -ldflags="-s -w" .

build_lambda:
	cd cmd/proxy-ingress-lambda && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap && zip function.zip bootstrap

deploy_lambda: build_lambda
	cd terraform/example/project_huk && terraform init && terraform apply

run_awsctl_proxy:
	awsctl proxy -function awsctl-proxy-ingress-lambda -port 8001 -region eu-central-1 -verbose

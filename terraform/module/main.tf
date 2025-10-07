data "aws_vpc" "vpc" {
  id = var.vpc_id
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  lambda_name          = "awsctl-proxy-ingress-lambda"
  lambda_zip_file_path = "${path.module}/../../cmd/proxy-ingress-lambda/function.zip"
}

resource "aws_iam_role" "this" {
  name = "${local.lambda_name}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "vpc_access" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
  role       = aws_iam_role.this.name
}

resource "aws_iam_role_policy_attachment" "basic_execution" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
  role       = aws_iam_role.this.name
}

resource "aws_iam_role_policy" "this" {
  name = "${local.lambda_name}-invoke-policy"
  role = aws_iam_role.this.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${data.aws_region.current.id}:${data.aws_caller_identity.current.account_id}:*"
      }
    ]
  })
}

resource "aws_lambda_function" "this" {
  function_name = local.lambda_name
  role          = aws_iam_role.this.arn
  handler       = "bootstrap"
  architectures = ["arm64"]
  runtime       = "provided.al2023"
  timeout       = 30
  memory_size   = 128

  filename         = local.lambda_zip_file_path
  source_code_hash = filebase64sha256(local.lambda_zip_file_path)

  vpc_config {
    subnet_ids         = var.vpc_subnet_ids
    security_group_ids = [aws_security_group.this.id]
  }
}
resource "aws_security_group" "this" {
  name        = local.lambda_name
  description = "Allow http/https traffic to vpc ips"
  vpc_id      = var.vpc_id

  egress {
    description = "HTTPS traffic"
    from_port   = 443
    to_port     = 443
    protocol    = "TCP"
    cidr_blocks = [data.aws_vpc.vpc.cidr_block]
  }
  egress {
    description = "HTTP traffic"
    from_port   = 80
    to_port     = 80
    protocol    = "TCP"
    cidr_blocks = [data.aws_vpc.vpc.cidr_block]
  }
}


resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/lambda/${aws_lambda_function.this.function_name}"
  retention_in_days = 14
}

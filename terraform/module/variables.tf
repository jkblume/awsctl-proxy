variable "vpc_subnet_ids" {
  description = "List of VPC subnet IDs where the Lambda function will be deployed"
  type        = list(string)
}

variable "vpc_id" {
  description = "VPC Id to deploy Lambda in"
  type        = string
}

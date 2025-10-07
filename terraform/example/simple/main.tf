terraform {
  required_version = ">= 1.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "< 7.0"
    }
  }
}

provider "aws" {
  region  = var.aws_region
  profile = var.aws_profile_name
}

module "proxy-ingress-lambda" {
  source         = "../../module"
  vpc_id         = var.vpc_id
  vpc_subnet_ids = var.vpc_subnet_ids
}

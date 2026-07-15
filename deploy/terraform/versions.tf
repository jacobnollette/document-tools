terraform {
  required_version = ">= 1.7.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }

  # Remote state is intentionally not configured yet. Before the first apply,
  # add an S3 backend here (bucket + DynamoDB lock table) so state is shared:
  #
  # backend "s3" {
  #   bucket         = "<state-bucket>"
  #   key            = "document-tools/terraform.tfstate"
  #   region         = "us-east-1"
  #   dynamodb_table = "<lock-table>"
  # }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = "document-tools"
      ManagedBy = "terraform"
    }
  }
}

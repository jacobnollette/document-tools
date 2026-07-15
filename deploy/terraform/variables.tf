variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Base name for all resources."
  type        = string
  default     = "document-tools"
}

variable "image_tag" {
  description = "Container image tag to deploy (set by the deploy pipeline)."
  type        = string
  default     = "latest"
}

variable "container_port" {
  description = "Port the server listens on inside the container."
  type        = number
  default     = 8080
}

variable "desired_count" {
  description = "Number of running tasks. Keep at 1 while storage is a single EFS-backed data directory."
  type        = number
  default     = 1
}

variable "cpu" {
  description = "Fargate task CPU units (256 = 0.25 vCPU)."
  type        = number
  default     = 256
}

variable "memory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 512
}

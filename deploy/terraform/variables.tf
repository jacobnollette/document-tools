variable "docker_host" {
  description = "Docker daemon to deploy to. Use ssh://user@host to target the home-lab host remotely."
  type        = string
  default     = "unix:///var/run/docker.sock"
}

variable "name" {
  description = "Base name for containers, network, and volumes."
  type        = string
  default     = "document-tools"
}

variable "image" {
  description = "App container image (pushed by the build pipeline)."
  type        = string
  default     = "ghcr.io/jacobnollette/document-tools"
}

variable "image_tag" {
  description = "App image tag to deploy."
  type        = string
  default     = "latest"
}

variable "registry_address" {
  description = "Container registry host, for pulling the app image."
  type        = string
  default     = "ghcr.io"
}

variable "registry_username" {
  description = "Registry username. Leave empty for anonymous pulls (public images)."
  type        = string
  default     = ""
}

variable "registry_password" {
  description = "Registry password or token. Only used when registry_username is set."
  type        = string
  default     = ""
  sensitive   = true
}

variable "data_root" {
  description = <<-EOT
    Host directory for all persistent state — point this at the shared
    storage mount (Ceph/NFS/SMB). Two subdirectories are used:
    <data_root>/app (config + document files) and <data_root>/postgres.
  EOT
  type        = string
}

variable "app_port" {
  description = "Host port the web interface is published on."
  type        = number
  default     = 8080
}

variable "db_user" {
  description = "PostgreSQL user, also pre-filled into the first-run installer."
  type        = string
  default     = "documents"
}

variable "db_password" {
  description = "PostgreSQL password, also pre-filled into the first-run installer."
  type        = string
  sensitive   = true
}

variable "db_name" {
  description = "PostgreSQL database name."
  type        = string
  default     = "documents"
}

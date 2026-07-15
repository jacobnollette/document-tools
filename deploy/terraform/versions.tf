terraform {
  required_version = ">= 1.7.0"

  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
    }
  }
}

# Points at the home-lab Docker host. Local socket by default; set
# docker_host to an ssh:// URL to apply from another machine, e.g.
# ssh://user@dockerhost.lab.
provider "docker" {
  host = var.docker_host

  dynamic "registry_auth" {
    for_each = var.registry_username != "" ? [1] : []
    content {
      address  = var.registry_address
      username = var.registry_username
      password = var.registry_password
    }
  }
}

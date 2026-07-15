output "app_url" {
  description = "Web interface on the Docker host."
  value       = "http://<docker-host>:${var.app_port} (replace with the host's address)"
}

output "app_container" {
  description = "App container name."
  value       = docker_container.app.name
}

output "db_container" {
  description = "PostgreSQL container name."
  value       = docker_container.postgres.name
}

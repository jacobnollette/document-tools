output "app_url" {
  description = "Public URL of the application (HTTP for now)."
  value       = "http://${aws_lb.main.dns_name}"
}

output "ecr_repository_url" {
  description = "Push target for container images."
  value       = aws_ecr_repository.app.repository_url
}

output "ecs_cluster_name" {
  description = "ECS cluster running the service."
  value       = aws_ecs_cluster.main.name
}

output "ecs_service_name" {
  description = "ECS service name (used by the deploy pipeline to force new deployments)."
  value       = aws_ecs_service.app.name
}

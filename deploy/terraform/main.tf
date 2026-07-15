# Home-lab stack: the app plus PostgreSQL on one Docker host, with all
# persistent state on the shared storage mount (var.data_root).

resource "docker_network" "internal" {
  name = var.name
}

resource "docker_image" "postgres" {
  name = "postgres:16-alpine"
}

resource "docker_container" "postgres" {
  name    = "${var.name}-db"
  image   = docker_image.postgres.image_id
  restart = "unless-stopped"

  env = [
    "POSTGRES_USER=${var.db_user}",
    "POSTGRES_PASSWORD=${var.db_password}",
    "POSTGRES_DB=${var.db_name}",
  ]

  networks_advanced {
    name = docker_network.internal.name
  }

  volumes {
    host_path      = "${var.data_root}/postgres"
    container_path = "/var/lib/postgresql/data"
  }

  healthcheck {
    test     = ["CMD-SHELL", "pg_isready -U ${var.db_user} -d ${var.db_name}"]
    interval = "10s"
    timeout  = "5s"
    retries  = 5
  }
}

resource "docker_image" "app" {
  name = "${var.image}:${var.image_tag}"
}

resource "docker_container" "app" {
  name    = var.name
  image   = docker_image.app.image_id
  restart = "unless-stopped"

  # DB_* pre-fill the first-run installer form; they can still be edited
  # in the browser before installing.
  env = [
    "DB_HOST=${docker_container.postgres.name}",
    "DB_PORT=5432",
    "DB_USER=${var.db_user}",
    "DB_PASSWORD=${var.db_password}",
    "DB_NAME=${var.db_name}",
    "DB_SSLMODE=disable",
  ]

  networks_advanced {
    name = docker_network.internal.name
  }

  ports {
    internal = 8080
    external = var.app_port
  }

  volumes {
    host_path      = "${var.data_root}/app"
    container_path = "/data"
  }

  healthcheck {
    test     = ["CMD-SHELL", "wget -q -O /dev/null http://localhost:8080/api/healthz || exit 1"]
    interval = "30s"
    timeout  = "5s"
    retries  = 3
  }

  depends_on = [docker_container.postgres]
}

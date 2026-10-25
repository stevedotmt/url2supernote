locals {
  services = [
    "artifactregistry.googleapis.com",
    "drive.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
  ]

  secrets = {
    TELEGRAM_BOT_TOKEN      = "${var.service_name}-telegram-bot-token"
    TELEGRAM_WEBHOOK_SECRET = "${var.service_name}-telegram-webhook-secret"
    GOOGLE_CLIENT_SECRET    = "${var.service_name}-google-client-secret"
    GOOGLE_REFRESH_TOKEN    = "${var.service_name}-google-refresh-token"
  }

  image       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.ghcr.repository_id}/${var.image_name}@${var.image_digest}"
  service_url = "https://${var.service_name}-${data.google_project.this.number}.${var.region}.run.app"
  webhook_url = "${local.service_url}/telegram/webhook"
}

data "google_project" "this" {
  project_id = var.project_id
}

resource "google_project_service" "this" {
  for_each = toset(local.services)

  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "ghcr" {
  repository_id = "ghcr"
  location      = var.region
  format        = "DOCKER"
  mode          = "REMOTE_REPOSITORY"
  description   = "Pull-through mirror of ghcr.io; Cloud Run cannot pull from ghcr.io directly."

  remote_repository_config {
    docker_repository {
      custom_repository {
        uri = "https://ghcr.io"
      }
    }
  }

  depends_on = [google_project_service.this]
}

resource "google_service_account" "runtime" {
  account_id   = var.service_name
  display_name = "url2supernote runtime"
}

resource "google_secret_manager_secret" "app" {
  for_each = local.secrets

  secret_id = each.value

  replication {
    auto {}
  }

  depends_on = [google_project_service.this]
}

resource "google_secret_manager_secret_iam_member" "runtime" {
  for_each = google_secret_manager_secret.app

  secret_id = each.value.id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.runtime.member
}

resource "google_cloud_run_v2_service" "this" {
  name                 = var.service_name
  location             = var.region
  ingress              = "INGRESS_TRAFFIC_ALL"
  invoker_iam_disabled = true
  deletion_protection  = false

  template {
    service_account = google_service_account.runtime.email
    timeout         = "120s"

    scaling {
      min_instance_count = 0
      max_instance_count = 1
    }

    containers {
      image = local.image
      args  = ["serve"]

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }

      env {
        name  = "TELEGRAM_MODE"
        value = "webhook"
      }
      env {
        name  = "TELEGRAM_WEBHOOK_URL"
        value = local.webhook_url
      }
      env {
        name  = "TELEGRAM_OWNER_CHAT_ID"
        value = var.telegram_owner_chat_id
      }
      env {
        name  = "SYNC_ENGINE"
        value = "googledrive"
      }
      env {
        name  = "DESTINATION"
        value = var.destination
      }
      env {
        name  = "GOOGLE_CLIENT_ID"
        value = var.google_client_id
      }

      dynamic "env" {
        for_each = google_secret_manager_secret.app
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value.secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }

  depends_on = [
    google_project_service.this,
    google_secret_manager_secret_iam_member.runtime,
  ]
}

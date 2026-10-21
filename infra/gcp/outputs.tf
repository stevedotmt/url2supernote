output "service_url" {
  description = "Public URL of the Cloud Run service."
  value       = google_cloud_run_v2_service.this.uri
}

output "webhook_url" {
  description = "URL the service registers with Telegram."
  value       = local.webhook_url
}

output "secret_ids" {
  description = "Secrets to populate with `gcloud secrets versions add` before the first full apply."
  value       = { for name, secret in google_secret_manager_secret.app : name => secret.secret_id }
}

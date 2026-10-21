variable "project_id" {
  description = "Google Cloud project that hosts the service."
  type        = string
}

variable "region" {
  description = "Region for Cloud Run and the Artifact Registry mirror."
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name of the Cloud Run service; also prefixes the secrets."
  type        = string
  default     = "url2supernote"
}

variable "image_name" {
  description = "Image path on ghcr.io."
  type        = string
  default     = "stevedotmt/url2supernote"
}

variable "image_tag" {
  description = "Release to deploy, e.g. v0.1.0."
  type        = string
}

variable "telegram_owner_chat_id" {
  description = "The Owner's Telegram chat ID, from `url2supernote telegram whoami`."
  type        = string

  validation {
    condition     = can(regex("^-?[0-9]+$", var.telegram_owner_chat_id))
    error_message = "telegram_owner_chat_id must be an integer."
  }
}

variable "destination" {
  description = "Destination folder path on Google Drive."
  type        = string
  default     = "Supernote/INBOX"
}

variable "google_client_id" {
  description = "Client ID of the Owner's Google OAuth client."
  type        = string
}

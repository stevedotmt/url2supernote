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

variable "image_digest" {
  description = "Digest of the release image to deploy, from the GitHub Release notes, e.g. sha256:0123…."
  type        = string

  validation {
    condition     = can(regex("^sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be a sha256 digest such as sha256:<64 hex characters>."
  }
}

variable "telegram_owner_chat_id" {
  description = "The Owner's Telegram chat ID, from `url2supernote telegram whoami`."
  type        = string

  validation {
    condition     = can(regex("^[1-9][0-9]*$", var.telegram_owner_chat_id))
    error_message = "telegram_owner_chat_id must be the positive ID of your private chat with the bot."
  }
}

variable "google_drive_folder_id" {
  description = "ID of the Destination folder on Google Drive, from `url2supernote auth google`."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9_-]+$", var.google_drive_folder_id))
    error_message = "google_drive_folder_id must be the folder ID printed by `url2supernote auth google`."
  }
}

variable "google_client_id" {
  description = "Client ID of the Owner's Google OAuth client."
  type        = string
}

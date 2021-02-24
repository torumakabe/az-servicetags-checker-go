variable "house_keeping_rg" {
  type = string
}

variable "house_keeping_location" {
  type = string
}

variable "house_keeping_storage_account" {
  type = string
}

variable "log_analytics_workspace_name" {
  type = string
}

variable "log_analytics_workspace_rg" {
  type = string
}

variable "alert_actiongroup_name" {
  type = string
}

variable "alert_actiongroup_rg" {
  type = string
}

variable "github_token" {
  type      = string
  sensitive = true
}

variable "azure_credentials" {
  type      = string
  sensitive = true
}

variable "registry_login_server" {
  type      = string
  sensitive = true
}

variable "registry_username" {
  type      = string
  sensitive = true
}

variable "registry_password" {
  type      = string
  sensitive = true
}

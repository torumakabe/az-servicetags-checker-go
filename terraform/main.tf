terraform {
  required_version = "~> 0.14.7"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 2.48"
    }

    github = {
      source  = "integrations/github"
      version = "~> 4.3"
    }

  }
}

provider "azurerm" {
  features {}
}

data "azurerm_subscription" "current" {
}

data "azurerm_log_analytics_workspace" "default" {
  name                = var.log_analytics_workspace_name
  resource_group_name = var.log_analytics_workspace_rg
}

data "azurerm_monitor_action_group" "email" {
  name                = var.alert_actiongroup_name
  resource_group_name = var.alert_actiongroup_rg
}

resource "azurerm_resource_group" "house_keeping" {
  name     = var.house_keeping_rg
  location = var.house_keeping_location
}

resource "azurerm_storage_account" "house_keeping" {
  name                     = var.house_keeping_storage_account
  resource_group_name      = azurerm_resource_group.house_keeping.name
  location                 = azurerm_resource_group.house_keeping.location
  account_tier             = "Standard"
  account_replication_type = "ZRS"
}

resource "azurerm_storage_container" "servicetags" {
  name                  = "servicetags"
  storage_account_name  = azurerm_storage_account.house_keeping.name
  container_access_type = "private"
}

resource "azurerm_storage_container" "servicetags_report" {
  name                  = "servicetags-report"
  storage_account_name  = azurerm_storage_account.house_keeping.name
  container_access_type = "private"
}

resource "azurerm_user_assigned_identity" "house_keeping" {
  resource_group_name = azurerm_resource_group.house_keeping.name
  location            = azurerm_resource_group.house_keeping.location

  name = "mi-house-keeping"
}

resource "azurerm_role_assignment" "servicetags_checker_subscription" {
  scope                = data.azurerm_subscription.current.id
  role_definition_name = "Reader"
  principal_id         = azurerm_user_assigned_identity.house_keeping.principal_id
}

resource "azurerm_role_assignment" "servicetags_checker_container_servicetags" {
  scope                = azurerm_storage_container.servicetags.resource_manager_id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.house_keeping.principal_id
}

resource "azurerm_role_assignment" "servicetags_checker_container_report" {
  scope                = azurerm_storage_container.servicetags_report.resource_manager_id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = azurerm_user_assigned_identity.house_keeping.principal_id
}

resource "azurerm_monitor_scheduled_query_rules_alert" "servicetags_checker" {
  name                = "servicetags_checker"
  resource_group_name = azurerm_resource_group.house_keeping.name
  location            = azurerm_resource_group.house_keeping.location

  action {
    action_group  = [data.azurerm_monitor_action_group.email.id]
    email_subject = "Network Service Tags has been changed"
  }

  data_source_id = data.azurerm_log_analytics_workspace.default.id
  description    = "Alert when Service tag has been changed."
  enabled        = true
  # Count all requests with server error result code grouped into 5-minute bins
  query       = <<-QUERY
  ContainerInstanceLog_CL
  | where Message contains "Service tag has been changed."
  QUERY
  severity    = 3
  frequency   = 1440
  time_window = 1440
  trigger {
    operator  = "GreaterThan"
    threshold = 0
  }
}

provider "github" {
  token = var.github_token
}

data "github_actions_public_key" "servicetags_checker" {
  repository = "az-servicetags-checker-go"
}

resource "github_actions_secret" "azure_credentials" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "AZURE_CREDENTIALS"
  plaintext_value = var.azure_credentials
}

resource "github_actions_secret" "subscription_id" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "SUBSCRIPTION_ID"
  plaintext_value = data.azurerm_subscription.current.subscription_id
}

resource "github_actions_secret" "resource_group" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "RESOURCE_GROUP"
  plaintext_value = azurerm_resource_group.house_keeping.name
}

resource "github_actions_secret" "log_analytics_workspace_id" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "LOG_ANALYTICS_WORKSPACE_ID"
  plaintext_value = data.azurerm_log_analytics_workspace.default.workspace_id
}

resource "github_actions_secret" "log_analytics_workspace_key" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "LOG_ANALYTICS_WORKSPACE_KEY"
  plaintext_value = data.azurerm_log_analytics_workspace.default.primary_shared_key
}

resource "github_actions_secret" "user_assigned_identity" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "USER_ASSIGNED_IDENTITY"
  plaintext_value = azurerm_user_assigned_identity.house_keeping.id
}

resource "github_actions_secret" "storage_account" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "SERVICETAGS_CHECK_STORAGE_ACCOUNT_NAME"
  plaintext_value = azurerm_storage_account.house_keeping.name
}

resource "github_actions_secret" "registry_login_server" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "REGISTRY_LOGIN_SERVER"
  plaintext_value = var.registry_login_server
}

resource "github_actions_secret" "registry_login_username" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "REGISTRY_USERNAME"
  plaintext_value = var.registry_username
}

resource "github_actions_secret" "registry_password" {
  repository      = "az-servicetags-checker-go"
  secret_name     = "REGISTRY_PASSWORD"
  plaintext_value = var.registry_password
}

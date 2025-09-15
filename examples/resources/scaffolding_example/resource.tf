terraform {
  required_providers {
    pipefy = {
      source  = "terraform.local/pipefy/pipefy"
      version = "0.0.1"
    }
  }
}

provider "pipefy" {
  client_id     = var.pipefy_client_id
  client_secret = var.pipefy_client_secret
}

variable "pipefy_client_id" {
  description = "Pipefy Service Account Client ID"
  type        = string
}

variable "pipefy_client_secret" {
  description = "Pipefy Service Account Client Secret"
  type        = string
  sensitive   = true
}


variable "organization_id" {
  description = "Pipefy Organization ID"
  type        = number
}

resource "pipefy_pipe" "test" {
  name            = "Terraform Test Pipe updated"
  organization_id = var.organization_id
}

resource "pipefy_phase" "backlog" {
  pipe_id = pipefy_pipe.test.id
  name    = "Backlog"
}

resource "pipefy_field" "title" {
  phase_id = pipefy_phase.backlog.id
  type     = "short_text"
  label    = "Title from terraform"
  required = true
}

output "pipe_id" { value = pipefy_pipe.test.id }
output "phase_id" { value = pipefy_phase.backlog.id }
output "field_id" { value = pipefy_field.title.id }

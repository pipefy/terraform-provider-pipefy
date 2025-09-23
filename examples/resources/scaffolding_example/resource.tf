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


resource "pipefy_field" "translation" {
  phase_id = pipefy_phase.backlog.id
  type     = "short_text"
  label    = "Translation"
  required = true
}

resource "pipefy_automation" "example_ai" {
  name           = "Example AI Generation"
  event_id       = "card_created"
  action_id      = "generate_with_ai"
  event_repo_id  = pipefy_pipe.test.id
  action_repo_id = pipefy_pipe.test.id
  active         = true

  # action_params and condition must be JSON strings
  action_params = jsonencode({
    aiParams = {
      value    = "Translate to german: %%{${pipefy_field.title.internal_id}}"
      fieldIds = [pipefy_field.translation.internal_id]
    }
  })

  # no filtering
  condition = jsonencode({
    expressions = [{
      structure_id  = 0
      field_address = ""
      operation     = ""
      value         = ""
    }]
    expressions_structure = [[0]]
  })
}


output "pipe_id" { value = pipefy_pipe.test.id }
output "phase_id" { value = pipefy_phase.backlog.id }
output "field_id" { value = pipefy_field.title.internal_id }
output "translation_field_id" { value = pipefy_field.translation.internal_id }
output "automation_id" { value = pipefy_automation.example_ai.id }

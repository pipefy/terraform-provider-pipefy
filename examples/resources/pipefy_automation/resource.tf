resource "pipefy_pipe" "test" {
  name            = "Terraform Test Pipe updated"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "backlog" {
  pipe_id = pipefy_pipe.test.id
  name    = "translation_phase"
}

resource "pipefy_field" "title" {
  phase_id = pipefy_phase.backlog.id
  type     = "short_text"
  label    = "input"
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
  event_id       = "field_updated"
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

  event_params = {
    trigger_field_ids = [pipefy_field.title.internal_id]
  }

  # conditions to trigger the automation
  condition = jsonencode({
    expressions = [{
      structure_id  = 0
      field_address = ""
      operation     = ""
      value         = ""
    }]
    expressions_structure = [[0]]
  })

  # Optional JSON schema describing the automation's structured response.
  response_schema = jsonencode({
    type = "object"
    properties = {
      translation = { type = "string" }
    }
  })
}

# A recurring (scheduler) automation: every day at 09:30 it selects matching
# cards and moves them. scheduler_cron uses standard crontab syntax, and
# search_for lists the selection conditions in order.
resource "pipefy_automation" "daily_move" {
  name           = "Move follow-up cards daily"
  event_id       = "scheduler"
  action_id      = "move_multiple_cards"
  event_repo_id  = pipefy_pipe.test.id
  action_repo_id = pipefy_pipe.test.id
  active         = true

  scheduler_frequency = "daily"

  scheduler_cron = {
    minute       = "30"
    hour         = "9"
    day_of_month = "*"
    month        = "*"
    day_of_week  = "*"
  }

  search_for = [
    {
      field     = pipefy_field.title.internal_id
      id        = "match-follow-ups"
      operation = "eq"
      value     = "Follow up"
    },
  ]

  # move_multiple_cards moves the selected cards to this phase.
  action_params = jsonencode({
    to_phase_id = pipefy_phase.backlog.id
  })
}

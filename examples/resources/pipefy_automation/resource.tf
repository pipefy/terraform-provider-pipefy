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
      value    = "Translate to german: %%{${pipefy_field.input.internal_id}}"
      fieldIds = [pipefy_field.translation.internal_id]
    }
  })

  event_params = jsonencode({
    triggerFieldIds = [pipefy_field.input.internal_id]
  })

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
}

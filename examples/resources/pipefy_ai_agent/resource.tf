resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "triage" {
  pipe_id = pipefy_pipe.example.id
  name    = "Triage"
}

resource "pipefy_phase" "ready" {
  pipe_id = pipefy_pipe.example.id
  name    = "Ready"
}

resource "pipefy_field" "title" {
  phase_id = pipefy_pipe.example.start_form_phase_id
  type     = "short_text"
  label    = "Title"
  required = true
}

resource "pipefy_field" "summary" {
  phase_id = pipefy_phase.triage.id
  type     = "long_text"
  label    = "Summary"
}

resource "pipefy_ai_agent" "triage" {
  pipe_id     = pipefy_pipe.example.id
  name        = "Triage assistant"
  instruction = "Route and enrich cards based on their content."
  active      = true

  behaviors = [
    {
      name        = "Route new cards"
      event_id    = "card_created"
      instruction = "Move the card to the Ready phase when it is ready for work."

      actions = [
        {
          name                 = "Move to Ready"
          action_type          = "move_card"
          destination_phase_id = pipefy_phase.ready.id
        },
      ]
    },
    {
      name        = "Rewrite title"
      event_id    = "field_updated"
      instruction = "Improve the card title when the summary changes."

      event_params = {
        trigger_field_ids = [pipefy_field.summary.internal_id]
      }

      actions = [
        {
          name        = "Update title"
          action_type = "update_card"
          pipe_id     = pipefy_pipe.example.id

          fields = [
            {
              field_id   = pipefy_field.title.internal_id
              input_mode = "fill_with_ai"
            },
          ]
        },
      ]
    },
    {
      name        = "Open follow-up"
      event_id    = "card_moved"
      instruction = "Create a follow-up card when work reaches Ready."

      event_params = {
        to_phase_id = pipefy_phase.ready.id
      }

      actions = [
        {
          name        = "Create follow-up"
          action_type = "create_card"
          pipe_id     = pipefy_pipe.example.id

          fields = [
            {
              field_id   = pipefy_field.title.internal_id
              input_mode = "fixed_value"
              value      = "Follow-up"
            },
          ]
        },
      ]
    },
  ]
}

resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "backlog" {
  pipe_id = pipefy_pipe.example.id
  name    = "Backlog"
  index   = 1
}

resource "pipefy_webhook" "example" {
  pipe_id = pipefy_pipe.example.id
  name    = "My Webhook"
  url     = "https://example.com/webhook"
  actions = ["card.move"]

  headers = {
    "Authorization" = "Bearer secret-token"
  }

  # Only fire when a card moves out of the Backlog phase.
  # filters require exactly one action.
  filters = "{\"from_phase_id\":[${pipefy_phase.backlog.id}]}"
}

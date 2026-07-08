resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "in_progress" {
  pipe_id = pipefy_pipe.example.id
  name    = "In progress"
}

resource "pipefy_webhook" "moves_from_phase" {
  pipe_id = pipefy_pipe.example.id
  name    = "Moves out of In progress"
  url     = "https://example.com/webhooks/pipefy"
  actions = ["card.move"]

  headers = jsonencode({
    Authorization = "Bearer <TOKEN>"
  })

  # Only one action can be configured when filters are used. Phase IDs are
  # numeric, so the phase's id (a string) is converted with tonumber.
  filters = jsonencode({
    from_phase_id = [tonumber(pipefy_phase.in_progress.id)]
  })
}

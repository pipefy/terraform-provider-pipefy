resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "example" {
  pipe_id = pipefy_pipe.example.id
  name    = "Backlog"
}

resource "pipefy_field" "example" {
  phase_id = pipefy_phase.example.id
  type     = "short_text"
  label    = "Title"
  required = true
}

resource "pipefy_field" "priority" {
  phase_id = pipefy_phase.example.id
  type     = "select"
  label    = "Priority"
  options  = ["Low", "Medium", "High"]
}

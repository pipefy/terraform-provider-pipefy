resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_label" "urgent" {
  pipe_id = pipefy_pipe.example.id
  name    = "Urgent"
  color   = "#FF0000"
}

resource "pipefy_label" "blocked" {
  pipe_id = pipefy_pipe.example.id
  name    = "Blocked"
  color   = "#FFA500"
}
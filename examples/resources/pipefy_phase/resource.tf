resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "example" {
  pipe_id     = pipefy_pipe.example.id
  name        = "Backlog"
  description = "Work waiting to be triaged"
  index       = 1
  color       = "blue"
}

resource "pipefy_phase" "done" {
  pipe_id                              = pipefy_pipe.example.id
  name                                 = "Done"
  done                                 = true
  lateness_time                        = 86400
  can_receive_card_directly_from_draft = false
}

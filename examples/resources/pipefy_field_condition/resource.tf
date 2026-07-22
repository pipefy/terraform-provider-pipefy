resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"
}

resource "pipefy_phase" "example" {
  pipe_id = pipefy_pipe.example.id
  name    = "Intake"
}

resource "pipefy_field" "type" {
  phase_id = pipefy_phase.example.id
  type     = "select"
  label    = "Request type"
  options  = ["Standard", "Other"]
}

resource "pipefy_field" "priority" {
  phase_id = pipefy_phase.example.id
  type     = "select"
  label    = "Priority"
  options  = ["Low", "Medium", "High"]
}

resource "pipefy_field" "details" {
  phase_id = pipefy_phase.example.id
  type     = "long_text"
  label    = "Please describe"
}

# Show the "Please describe" field only when "Request type" is "Other"
# AND "Priority" is "High". Both expressions share one group in
# expressions_structure ([["0", "1"]]), so they are ANDed together.
resource "pipefy_field_condition" "show_details" {
  phase_id = pipefy_phase.example.id
  name     = "Show details for high-priority Other requests"

  condition = {
    expressions = [
      {
        structure_id  = "0"
        field_address = pipefy_field.type.internal_id
        operation     = "equals"
        value         = "Other"
      },
      {
        structure_id  = "1"
        field_address = pipefy_field.priority.internal_id
        operation     = "equals"
        value         = "High"
      }
    ]
    expressions_structure = [["0", "1"]]
  }

  actions = [
    {
      action_id      = "show"
      phase_field_id = pipefy_field.details.internal_id
    }
  ]
}

# Reusing the same fields with an OR grouping: hide "Priority" when either
# "Request type" is "Standard" OR "Priority" itself is "Low". Each expression
# sits in its own group in expressions_structure ([["0"], ["1"]]), so the
# condition holds when any group is true.
resource "pipefy_field_condition" "hide_priority" {
  phase_id = pipefy_phase.example.id
  name     = "Hide priority for standard or low requests"

  condition = {
    expressions = [
      {
        structure_id  = "0"
        field_address = pipefy_field.type.internal_id
        operation     = "equals"
        value         = "Standard"
      },
      {
        structure_id  = "1"
        field_address = pipefy_field.priority.internal_id
        operation     = "equals"
        value         = "Low"
      }
    ]
    expressions_structure = [["0"], ["1"]]
  }

  actions = [
    {
      action_id      = "hide"
      phase_field_id = pipefy_field.priority.internal_id
    }
  ]
}
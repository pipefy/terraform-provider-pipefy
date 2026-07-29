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
# AND "Priority" is "High". all_of ANDs its comparisons together.
resource "pipefy_field_condition" "show_details" {
  phase_id = pipefy_phase.example.id
  name     = "Show details for high-priority Other requests"

  condition = {
    all_of = [
      { field = pipefy_field.type.internal_id, operation = "equals", value = "Other" },
      { field = pipefy_field.priority.internal_id, operation = "equals", value = "High" },
    ]
  }

  actions = [
    { field = pipefy_field.details.internal_id, when_true = "show", when_false = "hide" },
  ]
}

# Reusing the same fields with an OR condition: hide "Priority" when either
# "Request type" is "Standard" OR "Priority" itself is "Low". any_of ORs its
# entries together.
resource "pipefy_field_condition" "hide_priority" {
  phase_id = pipefy_phase.example.id
  name     = "Hide priority for standard or low requests"

  condition = {
    any_of = [
      { field = pipefy_field.type.internal_id, operation = "equals", value = "Standard" },
      { field = pipefy_field.priority.internal_id, operation = "equals", value = "Low" },
    ]
  }

  actions = [
    { field = pipefy_field.priority.internal_id, when_true = "hide" },
  ]
}

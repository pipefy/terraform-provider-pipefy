resource "pipefy_table" "example" {
  name            = "Example Table"
  organization_id = "<ORG_ID>"
}

resource "pipefy_table_field" "example" {
  table_id          = pipefy_table.example.id
  type              = "short_text"
  label             = "Name"
  required          = true
  unique            = true
  description       = "The record's display name"
  help              = "Enter a short, clear name"
  minimal_view      = true
  custom_validation = "min:3"
}

resource "pipefy_table_field" "priority" {
  table_id = pipefy_table.example.id
  type     = "select"
  label    = "Priority"
  options  = ["Low", "Medium", "High"]
}
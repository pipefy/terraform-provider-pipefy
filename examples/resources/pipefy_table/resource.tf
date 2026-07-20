resource "pipefy_table" "example" {
  name            = "Example Table"
  organization_id = "<ORG_ID>"

  description   = "Tracks example records"
  authorization = "write"
  icon          = "briefing"
  color         = "purple"
}
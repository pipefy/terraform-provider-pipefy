resource "pipefy_pipe" "example" {
  name            = "Example Pipe"
  organization_id = "<ORG_ID>"

  icon   = "rocket"
  color  = "purple"
  public = false

  only_admin_can_remove_cards   = true
  only_assignees_can_edit_cards = false

  preferences = {
    inbox_email_enabled = false
    main_tab_views      = ["PreviousPhases", "Comments"]
  }

  sla = {
    time = 7
    unit = "days"
  }
}

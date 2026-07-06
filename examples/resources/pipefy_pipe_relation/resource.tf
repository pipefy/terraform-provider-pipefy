resource "pipefy_pipe" "orders" {
  name            = "Orders"
  organization_id = "<ORG_ID>"
}

resource "pipefy_pipe" "fulfillment" {
  name            = "Fulfillment"
  organization_id = "<ORG_ID>"
}

resource "pipefy_field" "priority" {
  phase_id = pipefy_pipe.fulfillment.start_form_phase_id
  type     = "short_text"
  label    = "Priority"
}

resource "pipefy_pipe_relation" "orders_to_fulfillment" {
  parent_id = pipefy_pipe.orders.id
  child_id  = pipefy_pipe.fulfillment.id
  name      = "Fulfillment tasks"

  can_create_new_items       = true
  can_connect_existing_items = false

  all_children_must_be_done_to_finish_parent = true

  auto_fill_field_enabled = true
  own_field_maps = [
    {
      field_id   = pipefy_field.priority.internal_id
      input_mode = "fixed_value"
      value      = "High"
    }
  ]
}

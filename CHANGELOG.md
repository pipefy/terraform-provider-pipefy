## 0.1.0 (Unreleased)

BREAKING CHANGES:

* `resource/pipefy_automation`: `active` is now `Required` (previously `Optional`), so active/inactive intent is explicit.
* `resource/pipefy_automation`: `event_params` and `condition` are now typed nested blocks instead of JSON strings. Configurations using `jsonencode(...)` for either must be rewritten to block form (see the resource example). `action_params` remains a JSON string. State written by an earlier version where `event_params` or `condition` was set cannot be upgraded automatically. After rewriting the config, refresh the state without recreating the automation:

  ```sh
  terraform state rm pipefy_automation.<name>
  terraform import pipefy_automation.<name> <id>
  ```

  A later `apply` re-sends `event_params` and `action_params` from config because both are write-only; this is expected and non-destructive.

FEATURES:

ENHANCEMENTS:

* `resource/pipefy_field`: Add `description`, `help`, `editable`, `minimal_view`, `custom_validation`, and `index` attributes.
* `resource/pipefy_automation`: Add `scheduler_frequency`, `scheduler_cron`, `search_for`, and `response_schema` attributes.
* `resource/pipefy_automation`: `Read` now refreshes state (name, active, event/action ids and repos, and the new attributes plus `condition`), so changes made outside Terraform are detected. `search_for` and `condition` are managed in full: an empty or omitted block clears them on the server. `event_params` and `action_params` are write-only and not read back, so drift in them is not detected.

BUG FIXES:

* `resource/pipefy_field`: `Read` now refreshes `label` and `required`, so changes made outside Terraform are detected. `required` is now `Optional` + `Computed` to support this without a perpetual diff; existing state upgrades without a spurious change.
* `resource/pipefy_field`: fix import. The import ID is now `phase_id/field_uuid` (previously a bare field id, which could not be read back), and `type` is refreshed on read so an imported field does not plan a spurious replacement.
* `resource/pipefy_automation`: `Read` previously discarded the fetched automation, so out-of-band changes never surfaced in `terraform plan`; it now maps the API response into state.

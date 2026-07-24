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

* `resource/pipefy_ai_agent`: New resource to manage AI agents with typed behaviors and the supported actions `move_card`, `update_card`, and `create_card`. Covered by headless unit tests; live acceptance tests (`make testacc`) are deferred.

ENHANCEMENTS:

* `resource/pipefy_field`: Add `description`, `help`, `editable`, `minimal_view`, `custom_validation`, and `index` attributes.
* `resource/pipefy_automation`: Add `scheduler_frequency`, `scheduler_cron`, `search_for`, and `response_schema` attributes.
* `resource/pipefy_automation`: `Read` now refreshes state (name, active, event/action ids and repos, and the new attributes plus `condition`), so changes made outside Terraform are detected. `search_for` and `condition` are managed in full: omitting the block clears them on the server. `event_params` and `action_params` are write-only and not read back, so drift in them is not detected.
* `resource/pipefy_automation`: `condition` expressions and `search_for` entries reject a blank `field_address`/`field`, `operation`, `structure_id`, or `id` at plan time. A `condition` expression whose `field_address` and `operation` are empty is dropped by the API, which would otherwise surface as a perpetual diff; the `search_for` checks guard the same class of invalid input, since a blank field id or operation can never be valid. `structure_id` and `id` are caller-assigned handles referenced elsewhere in the config, so a blank one is never meaningful either.

BUG FIXES:

* `resource/pipefy_field`: `Read` now refreshes `label` and `required`, so changes made outside Terraform are detected. `required` is now `Optional` + `Computed` to support this without a perpetual diff; existing state upgrades without a spurious change.
* `resource/pipefy_field`: fix import. The import ID is now `phase_id/field_uuid` (previously a bare field id, which could not be read back), and `type` is refreshed on read so an imported field does not plan a spurious replacement.
* `resource/pipefy_automation`: `Read` previously discarded the fetched automation, so out-of-band changes never surfaced in `terraform plan`; it now maps the API response into state.
* `resource/pipefy_automation`: removing `response_schema` from config now clears it on the server. The API keeps an omitted `responseSchema`, so `Update` sends an explicit null; previously the next refresh restored the old value and every `plan` proposed the same removal.
* `resource/pipefy_automation`: `scheduler_frequency` and `scheduler_cron` are now required at plan time while `event_id` is `scheduler`. Neither can be cleared: the API rejects a null for either on that event and keeps the previous value, so omitting one after it was set could never be applied and left `plan` proposing the same removal on every run. Existing configurations that applied cleanly are unaffected, since the API requires both to create such an automation. Importing one does change: `terraform import` of a `scheduler` automation now needs both attributes present in config before the first `plan`, which otherwise fails validation instead of rendering a diff to fill in. The same applies to the `state rm` + `import` migration above when the automation uses the `scheduler` event.

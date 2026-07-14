## 0.1.0 (Unreleased)

FEATURES:

ENHANCEMENTS:

* `resource/pipefy_field`: Add `description`, `help`, `editable`, `minimal_view`, `custom_validation`, and `index` attributes.

BUG FIXES:

* `resource/pipefy_field`: `Read` now refreshes `label` and `required`, so changes made outside Terraform are detected. `required` is now `Optional` + `Computed` to support this without a perpetual diff; existing state upgrades without a spurious change.
* `resource/pipefy_field`: fix import. The import ID is now `phase_id/field_uuid` (previously a bare field id, which could not be read back), and `type` is refreshed on read so an imported field does not plan a spurious replacement.

# AGENTS.md

Development guide for `terraform-provider-pipefy`, aimed at both humans and AI coding agents. `CLAUDE.md` is a symlink to this file.

## Project overview

A Terraform provider that manages [Pipefy](https://www.pipefy.com) resources through the Pipefy GraphQL API (`https://api.pipefy.com/graphql`). It is built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and speaks Terraform plugin protocol 6.0.

Module path: `github.com/pipefy/terraform-provider-pipefy`. Registry source: `pipefy/pipefy`.

It exposes four resources (`pipefy_pipe`, `pipefy_phase`, `pipefy_field`, `pipefy_automation`, all importable) and two data sources (`pipefy_pipe`, `pipefy_phase`).

## Setup and requirements

- Go >= 1.24 (pinned to 1.24.0 in `.tool-versions` and `go.mod`)
- Terraform >= 1.0
- `golangci-lint` for linting

```shell
git clone https://github.com/pipefy/terraform-provider-pipefy
cd terraform-provider-pipefy
go mod download
make build
```

## Essential commands

All tasks run through the `GNUmakefile`. Use these targets rather than ad hoc `go` invocations.

| Command         | What it does                                                       |
| --------------- | ------------------------------------------------------------------ |
| `make build`    | Compile all packages (`go build -v ./...`)                         |
| `make install`  | Build and install the provider into `$GOPATH/bin`                  |
| `make generate` | Regenerate registry docs via tfplugindocs (`cd tools; go generate ./...`) |
| `make fmt`      | Format Go code (`gofmt -s -w -e .`)                                |
| `make lint`     | Run `golangci-lint run`                                            |
| `make test`     | Run unit tests                                                     |
| `make testacc`  | Run acceptance tests (`TF_ACC=1`) against the live API             |

The `docs/` directory is generated. Run `make generate` after any schema change; never hand-edit files under `docs/`.

## Project layout

```
internal/provider/
  provider.go              # Provider config, auth, resource/data-source registration
  resources/               # pipefy_pipe, pipefy_phase, pipefy_field, pipefy_automation
  datasources/             # pipefy_pipe, pipefy_phase
  client/api_client.go     # Pipefy GraphQL HTTP client
  locks/                   # Mutex helpers for serializing API mutations
examples/                  # Example .tf per resource and data source (feeds the docs)
docs/                      # Generated reference docs (do not edit by hand)
tools/                     # tfplugindocs tooling for `make generate`
main.go                    # Provider entrypoint
```

## Conventions

- Terraform attribute names are snake_case; resource type names are prefixed `pipefy_`.
- Name every GraphQL operation with a descriptive PascalCase name and a `_tf` suffix: `Get*` for queries, `Create*` / `Update*` / `Delete*` for mutations, naming the entity and the action (`query GetPipe_tf($id:ID!){ ... }`, `mutation CreateLabel_tf(...)`). The `_tf` marker identifies the request as provider traffic in server-side traces and logs. Each request carries one operation, so the name in the document is enough; no separate `operationName` field is needed.
- Register every resource and data source in `provider.go` via the `Resources()` and `DataSources()` constructor lists.
- Each resource has a matching example at `examples/resources/<type>/resource.tf` and an `import.sh`; data sources have `examples/data-sources/<type>/data-source.tf`. These examples are embedded into the generated docs.
- Auth lives entirely in `provider.go`'s `Configure`. The provider accepts a static `token` (env `PIPEFY_TOKEN`) or a service account `client_id` + `client_secret` (env `PIPEFY_CLIENT_ID` / `PIPEFY_CLIENT_SECRET`); exactly one mode must be configured.

## How to add a resource

1. Implement schema, `Create`, `Read`, `Update`, `Delete`, and `ImportState` in `internal/provider/resources/resource_<name>.go`.
2. Add its constructor to the `Resources()` list in `internal/provider/provider.go`.
3. Add `examples/resources/pipefy_<name>/resource.tf` and `import.sh`.
4. Add an acceptance test `internal/provider/resource_<name>_test.go`.
5. Run `make generate` to refresh `docs/`, then `make testacc` to verify.

## Testing

- Unit tests: `make test`.
- Acceptance tests are gated on `TF_ACC=1` (use `make testacc`). They create real Pipefy resources and may incur cost.
- Export credentials before running acceptance tests: `PIPEFY_TOKEN`, or `PIPEFY_CLIENT_ID` and `PIPEFY_CLIENT_SECRET`.

## Gotchas

- One of the two auth modes is required; the provider errors at configure time if neither is set.
- Single-tenant deployments must set both `endpoint` and `token_url` to the tenant domain.

## Never

- Commit API tokens, client secrets, or other credentials.
- Hand-edit generated files under `docs/`; change the schema and run `make generate` instead.

## API Interaction and Resource Validation

To ensure the provider remains maintainable as the SaaS platform evolves, adhere to the following guidelines regarding resource attributes and validation:

### 1. Attribute Validation Strategy
- **Decision Framework:**
  - **Use Enums:** Only when the attribute values are canonical and highly stable. This provides superior DX (IDE autocomplete, instant feedback) while accepting the occasional need for provider updates.
  - **Use Strings (Free-Text):** When values are volatile or frequently updated by the SaaS API. This avoids breaking changes in the provider.
  - **Use Regex for Strings:** Even for volatile strings, implement a a high level validation with Regex if the data format is predictable. This prevents malformed data from reaching the API while maintaining flexibility.
- **Reusable Validators:** Do not reinvent the wheel for standard validations. Create and maintain a library of "generic" validators (e.g., `IsOneOf`, `MatchesRegex`, `IsUUID`) within the provider's internal package. These should be reused across multiple resources to ensure consistent behavior and standardized error messages across the entire provider.
- **Dynamic Discovery:** When users need to verify valid options for volatile attributes, encourage the use of Data Sources that fetch the current list directly from the API.
- **Error Handling:** If an input is invalid, rely on the backend API’s response to inform the user. When implementing the provider, ensure that API error messages are surfaced clearly to the user, mapping generic HTTP errors to actionable feedback.

### 2. Documentation Standards
When defining resource attributes that are subject to backend change:
- **Do not include static lists of values in descriptions**, as these will quickly go stale.
- **Use "Evergreen" Descriptions:** Point users to the official SaaS API documentation for the source of truth regarding valid values.
  - Pipefy's official API documentation can be found at: https://developers.pipefy.com/reference
- **Example format:**
  > `action`: (String) The event that triggers this webhook. Supported values are defined by the SaaS platform. Please refer to [Link to API Documentation] for the current list of available actions.

### 3. Plan-Time vs. Apply-Time
- **Plan-Time:** Only perform client-side validation for static, deterministic constraints (e.g., regex patterns for name formats, field length, or logical exclusivity between two local fields).
- **Apply-Time:** Defer checks for existence, permissions, and dynamic business logic to the Apply phase (API calls). This minimizes the risk of the provider being out-of-sync with the SaaS environment.
# TRex Generator Output Reference

Complete list of files produced by each generator in `scripts/`.

## 1. Entity Generator (`scripts/generator.go`)

Generates a full CRUD entity with event-driven controllers from `--kind KindName`.

| # | Generated File | Description |
|---|---|---|
| 1 | `pkg/api/{kind}.go` | API model struct and patch request |
| 2 | `pkg/api/presenters/{kind}.go` | Presenter conversion functions |
| 3 | `pkg/handlers/{kind}.go` | HTTP handlers (create, get, list, patch, delete) |
| 4 | `pkg/services/{kind}.go` | Business logic with OnUpsert/OnDelete event handlers |
| 5 | `pkg/dao/{kind}.go` | Data access layer |
| 6 | `pkg/dao/mocks/{kind}.go` | Mock DAO for unit testing |
| 7 | `pkg/db/migrations/YYYYMMDDHHMM_add_{kinds}.go` | Database migration |
| 8 | `test/integration/{kinds}_test.go` | Integration test suite |
| 9 | `test/factories/{kinds}.go` | Test data factories |
| 10 | `openapi/openapi.{kinds}.yaml` | OpenAPI sub-specification |
| 11 | `plugins/{kinds}/plugin.go` | Plugin with routes, controllers, presenters, service locator |

**Modified files:** `cmd/trex/main.go`, `pkg/db/migrations/migration_structs.go`, `openapi/openapi.yaml`

---

## 2. SDK Generator (`scripts/sdk-generator/`)

Generates typed client libraries from OpenAPI specs. Auto-discovers resources from `$ref` entries.

### Go SDK (`--go-out`)

| # | Generated File | Template | Scope |
|---|---|---|---|
| 1 | `types/base.go` | `go/base.go.tmpl` | Once |
| 2 | `types/list_options.go` | `go/list_options.go.tmpl` | Once |
| 3 | `client/client.go` | `go/http_client.go.tmpl` | Once |
| 4 | `client/iterator.go` | `go/iterator.go.tmpl` | Once |
| 5 | `types/{resource}.go` | `go/types.go.tmpl` | Per resource |
| 6 | `client/{resource}_api.go` | `go/client.go.tmpl` | Per resource |

### Python SDK (`--python-out`)

| # | Generated File | Template | Scope |
|---|---|---|---|
| 1 | `__init__.py` | `python/__init__.py.tmpl` | Once |
| 2 | `_base.py` | `python/base.py.tmpl` | Once |
| 3 | `client.py` | `python/http_client.py.tmpl` | Once |
| 4 | `_iterator.py` | `python/iterator.py.tmpl` | Once |
| 5 | `{resource}.py` | `python/types.py.tmpl` | Per resource |
| 6 | `_{resource}_api.py` | `python/client.py.tmpl` | Per resource |

### TypeScript SDK (`--ts-out`)

| # | Generated File | Template | Scope |
|---|---|---|---|
| 1 | `src/index.ts` | `ts/index.ts.tmpl` | Once |
| 2 | `src/base.ts` | `ts/base.ts.tmpl` | Once |
| 3 | `src/client.ts` | `ts/main_client.ts.tmpl` | Once |
| 4 | `src/{resource}.ts` | `ts/types.ts.tmpl` | Per resource |
| 5 | `src/{resource}_api.ts` | `ts/client.ts.tmpl` | Per resource |

**Example (3 resources):** 10 Go + 10 Python + 9 TypeScript = **29 files**

---

## 3. CLI Generator (`scripts/cli-generator/`)

Generates a complete Cobra-based CLI project from OpenAPI specs.

### Static Files (once per project)

| # | Generated File | Template | Description |
|---|---|---|---|
| 1 | `cmd/{binary}/main.go` | `cmd/main.go.tmpl` | Root command wiring |
| 2 | `cmd/{binary}/login/cmd.go` | `cmd/login.go.tmpl` | Login with --token, --url |
| 3 | `cmd/{binary}/logout/cmd.go` | `cmd/logout.go.tmpl` | Logout, clear credentials |
| 4 | `cmd/{binary}/version/cmd.go` | `cmd/version.go.tmpl` | Version display |
| 5 | `cmd/{binary}/completion/cmd.go` | `cmd/completion.go.tmpl` | Shell completion |
| 6 | `cmd/{binary}/config/cmd.go` | `cmd/config.go.tmpl` | Config display |
| 7 | `cmd/{binary}/list/cmd.go` | `cmd/list.go.tmpl` | List group command |
| 8 | `cmd/{binary}/get/cmd.go` | `cmd/get.go.tmpl` | Get group command |
| 9 | `cmd/{binary}/create/cmd.go` | `cmd/create.go.tmpl` | Create group command |
| 10 | `pkg/config/config.go` | `pkg/config.go.tmpl` | Config load/save/location |
| 11 | `pkg/config/token.go` | `pkg/token.go.tmpl` | JWT token parsing |
| 12 | `pkg/connection/connection.go` | `pkg/connection.go.tmpl` | HTTP client with auth |
| 13 | `pkg/dump/dump.go` | `pkg/dump.go.tmpl` | Colorized JSON output |
| 14 | `pkg/output/printer.go` | `pkg/printer.go.tmpl` | Pager-aware writer |
| 15 | `pkg/output/table.go` | `pkg/table.go.tmpl` | Dynamic column table renderer |
| 16 | `pkg/output/terminal.go` | `pkg/terminal.go.tmpl` | Terminal detection |
| 17 | `pkg/arguments/arguments.go` | `pkg/arguments.go.tmpl` | Common CLI flag helpers |
| 18 | `pkg/urls/urls.go` | `pkg/urls.go.tmpl` | API path constants |
| 19 | `pkg/info/info.go` | `pkg/info.go.tmpl` | Version info |
| 20 | `go.mod` | `gomod.tmpl` | Go module definition |

### Per-Resource Files (3 per resource)

| # | Generated File | Template | Description |
|---|---|---|---|
| 21+ | `cmd/{binary}/list/{plural}/cmd.go` | `cmd/list_resource.go.tmpl` | List with table/JSON, pagination, search |
| 22+ | `cmd/{binary}/get/{resource}/cmd.go` | `cmd/get_resource.go.tmpl` | Get by ID with JSON dump |
| 23+ | `cmd/{binary}/create/{resource}/cmd.go` | `cmd/create_resource.go.tmpl` | Create with auto-generated flags |

**Example (3 resources):** 20 static + 9 per-resource = **29 files**

---

## 4. Console Plugin Generator (`scripts/console-plugin-generator/`)

Generates a complete OpenShift Console dynamic plugin project from OpenAPI specs.
Produces a React/PatternFly application with webpack module federation, deployment manifests, and console extension registration.

### Static Files (once per project)

| # | Generated File | Template | Description |
|---|---|---|---|
| 1 | `package.json` | `package.json.tmpl` | npm package with console SDK, PatternFly, webpack deps |
| 2 | `tsconfig.json` | `tsconfig.json.tmpl` | TypeScript configuration |
| 3 | `webpack.config.ts` | `webpack.config.ts.tmpl` | Webpack 5 with ConsoleRemotePlugin |
| 4 | `console-extensions.json` | `console-extensions.json.tmpl` | Console extension registration (routes, nav) |
| 5 | `Dockerfile` | `Dockerfile.tmpl` | Multi-stage build (nodejs → nginx) |
| 6 | `src/index.ts` | `src/index.ts.tmpl` | Entry point |
| 7 | `src/utils/api.ts` | `src/utils/api.ts.tmpl` | API client factory with per-resource CRUD methods |
| 8 | `src/hooks/useApiAuth.ts` | `src/hooks/useApiAuth.ts.tmpl` | React hook for OpenShift console token |
| 9 | `src/components/App.tsx` | `src/components/App.tsx.tmpl` | React Router with routes per resource |
| 10 | `src/components/ResourceNav.tsx` | `src/components/ResourceNav.tsx.tmpl` | PatternFly Nav sidebar |
| 11 | `deploy/consoleplugin.yaml` | `deploy/consoleplugin.yaml.tmpl` | ConsolePlugin CR (`console.openshift.io/v1`) |
| 12 | `deploy/deployment.yaml` | `deploy/deployment.yaml.tmpl` | Deployment with TLS cert + nginx mounts |
| 13 | `deploy/service.yaml` | `deploy/service.yaml.tmpl` | Service with serving-cert annotation |
| 14 | `deploy/nginx-configmap.yaml` | `deploy/nginx.configmap.yaml.tmpl` | nginx.conf for TLS on port 9443 |

### Per-Resource Files (3 per resource)

| # | Generated File | Template | Description |
|---|---|---|---|
| 15+ | `src/components/{Resource}ListPage.tsx` | `src/components/ListPage.tsx.tmpl` | List page with PatternFly Table, Pagination, SearchInput |
| 16+ | `src/components/{Resource}DetailsPage.tsx` | `src/components/DetailsPage.tsx.tmpl` | Details page with DescriptionList, optional Delete |
| 17+ | `src/components/{Resource}CreatePage.tsx` | `src/components/CreatePage.tsx.tmpl` | Create form with auto-generated PatternFly fields |

**Example (3 resources):** 14 static + 9 per-resource = **23 files**

---

## File Count Summary

| Generator | Static Files | Per-Resource Files | Total (3 resources) |
|---|---|---|---|
| Entity | 0 + 3 modified | 11 per kind | 11 |
| SDK (Go) | 4 | 2 per resource | 10 |
| SDK (Python) | 4 | 2 per resource | 10 |
| SDK (TypeScript) | 3 | 2 per resource | 9 |
| CLI | 20 | 3 per resource | 29 |
| Console Plugin | 14 | 3 per resource | 23 |
| **Total** | **45** | **23 per resource** | **92** |

---

## Makefile Targets

| Target | Description |
|---|---|
| `make generate-sdk` | Generate Go + Python + TypeScript SDKs |
| `make generate-sdk-go` | Generate Go SDK only |
| `make generate-sdk-python` | Generate Python SDK only |
| `make generate-sdk-ts` | Generate TypeScript SDK only |
| `make generate-cli` | Generate CLI project |
| `make generate-console-plugin` | Generate OpenShift Console dynamic plugin |
| `make generate-all` | Generate SDK + CLI + Console Plugin |
| `make generate-clean` | Remove all generated output |

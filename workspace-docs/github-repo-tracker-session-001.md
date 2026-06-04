# GitHub Repo Tracker - Sesión 001

## Fecha
2026-06-04

## Resumen
Implementación del GitHub Repo Tracker como nueva opción dentro del menú Applications.

## Archivos Creados

### `internal/db/git.go`
- Struct `GitHubRepo` con todos los campos: id, url, name, organization, description, language, stars, updated_at, first_commit, size_bytes, category, source, notes, added_at
- `migrateGitHubRepos()` - creación de tabla e índices
- `UpsertGitHubRepo(r)` - INSERT ... ON CONFLICT(url) DO UPDATE
- `GetGitHubRepos(filter, sortBy, asc, limit, offset)` - SELECT con LIKE filter, sort validado, paginación
- `CountGitHubRepos(filter)` - SELECT COUNT(*)
- `DeleteGitHubRepo(id)` - DELETE por id
- `BulkInsertGitHubRepos(repos)` - INSERT OR IGNORE en transacción batch

### `internal/apps/git.go`
- `ParseGitHubURL(raw)` - extrae owner/repo de URLs de GitHub
- `FetchRepoMetadata(rawURL)` - consulta GitHub API (v3) para obtener metadatos del repo
- `BulkImportCSV(filepath, database)` - importa repos desde CSV con columnas: url, name, organization, description, language, category, notes, stars
- `NormalizeGitHubURL(raw)` - normaliza URLs a formato `https://github.com/owner/repo`

## Archivos Modificados

### `internal/db/db.go`
- Añadida llamada a `d.migrateGitHubRepos()` en `migrate()`

### `internal/tui/model.go`
- Nuevo estado: `stateGitHubRepos`
- Nuevo tipo: `githubRepoSortField` con 5 campos: sortRepoByName, sortRepoByStars, sortRepoByUpdated, sortRepoByAdded, sortRepoByCategory
- Nuevos campos en Model: githubRepoItems, githubRepoFiltered, githubRepoCursor, githubRepoFilter, githubRepoSortField, githubRepoSortAsc, githubRepoInput, githubRepoInputMode, githubRepoShowAddedCol, githubRepoPreviewVP, githubRepoPreviewFocus
- Nuevo item en appsItems: "GitHub Repo Tracker"
- Nuevo textinput y viewport inicializados en New()

### `internal/tui/update.go`
- Nueva message type: `githubReposMsg`
- Nueva función: `fetchGitHubReposCmd(database)` - carga repos desde DB
- Nueva función: `applyGitHubRepoFilter(m)` - filtra y ordena in-memory
- Nueva función: `updateGitHubRepoPreview(m)` - renderiza preview panel
- Nuevo case en dispatchAppsCmd para "GitHub Repo Tracker"
- Nuevo case en Update para githubReposMsg
- Nuevo bloque de key handling para stateGitHubRepos (filtro, navegación, sort, toggle columna, refresh)
- Añadido stateGitHubRepos en global back/quit y delegate updates

### `internal/tui/view.go`
- Nuevo case stateGitHubRepos en View()
- Nueva función: `renderGitHubRepos()` - layout side-by-side con tabla + preview
- Nueva función: `sortLabelGH()` - indicadores de sort en headers
- Nueva función: `githubRepoTableRows()` - renderiza filas de tabla con columnas configurables

## Columnas de la Tabla
| Columna | Ancho | Descripción |
|---------|-------|-------------|
| NAME | 28 | Nombre del repo |
| ORG | 20 | Organización/owner |
| CATEGORY | 14 | Categoría definida por usuario |
| LANGUAGE | 14 | Lenguaje principal |
| STARS | 8 | Estrellas |
| UPDATED | 12 | Última actualización |
| ADDED | 12 | Fecha de adición (toggleable con 'a') |

## Key Bindings
| Tecla | Acción |
|-------|--------|
| ↑/↓ o j/k | Navegar filas |
| ←/→ o h/l | Cambiar columna de ordenamiento |
| / | Activar filtro de texto |
| a | Toggle columna "Added" |
| r | Reset filtros + refresh |
| tab | Cambiar foco entre tabla y preview |
| q/esc | Volver a Applications menu |

## Decisiones Técnicas

1. **Patrón de preview**: Mismo patrón side-by-side que dotfiles y VSCode, con viewport scrollable para contenido truncado
2. **Fetch de GitHub API**: En `internal/apps/git.go`, usando endpoint v3, con soporte de GITHUB_TOKEN para rate limiting
3. **Import CSV**: Columnas libres (solo 'url' es obligatoria), normalización automática de URLs
4. **Las opciones de add/edit/delete/bulk import no están implementadas** - solo se muestra la tabla, filtrado, ordenamiento y preview
5. **Added column toggle**: Por defecto oculta, se muestra con tecla 'a'

## Pendientes para Próxima Sesión
- Implementar "Add repo" (enter/a) con input de URL y fetch opcional de API
- Implementar "Edit notes" para el repo seleccionado
- Implementar "Delete repo" (d)
- Implementar "Bulk import" desde CSV (i)
- Implementar "Edit category"
- Opción de fetch metadata desde GitHub API para repos existentes

---

## Sesión 002 — Mejoras de UX y filtros

### Fecha
2026-06-04

### Resumen
Mejoras en las interacciones del GitHub Repo Tracker: sort con `s`, filtros dropdown con lista, y `r` para refresh.

### Cambios en Key Bindings

| Tecla | Antes | Ahora |
|-------|-------|-------|
| `s` | (sin función) | Toggle sort order (asc/desc) |
| `s/o` | Toggle sort order | ~~Eliminado~~ |
| `r` | Reset filters | **Refresh** — recarga datos desde BD + limpia filtros |
| `c` | Cycle category | **Open category dropdown list** (↑/↓ navigate, enter select, esc cancel) |
| `L` | Cycle language | **Open language dropdown list** (↑/↓ navigate, enter select, esc cancel) |

### Nuevos componentes de UI

- **Category dropdown**: Usa `charm.land/bubbles/v2/list` para mostrar opciones seleccionables
- **Language dropdown**: Mismo patrón que category
- Ambos incluyen opción "(none)" para limpiar el filtro

### Archivos modificados

- `internal/tui/model.go` — Añadidos `githubRepoCatList`, `githubRepoLangList`, `githubRepoCatListMode`, `githubRepoLangListMode`
- `internal/tui/update.go` — Añadido import `list`, prioridad de dropdowns, key handling mejorado, `fetchGitHubReposCmd` con refresh
- `internal/tui/view.go` — Actualizados key hints

### Nuevas prioridades de input

1. Text filter input (`/`)
2. Category list dropdown (`c`)
3. Language list dropdown (`L`)
4. Stars threshold input (`>` / `<`)
5. Date range input (`D`)
6. Preview panel focused (`tab`)
7. Table navigation / filters

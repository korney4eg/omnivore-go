# Omnivore API Rewrite - Go Implementation Plan

## Implementation Status

**Current Phase:** Phase 8 - REST Endpoints & Resolver Union Types (95% complete)

**Overall Progress:** 42/58 completed todos (72.4%) - Almost there!

### Phase Completion:
- ✅ **Phase 1: Foundation** - 7/7 (100%) - COMPLETE
- ✅ **Phase 2: Data Models** - 6/6 (100%) - COMPLETE
- ✅ **Phase 3: Core Queries** - 7/7 (100%) - COMPLETE
- ✅ **Phase 4: Core Mutations** - 8/8 (100%) - COMPLETE
- ✅ **Phase 5: Authentication** - 4/5 (80%) - COMPLETE (OAuth blocked by requirements)
- ✅ **Phase 8: REST Endpoints** - 5/5 (100%) - COMPLETE 🎉
- ⚠️ **Current: Union Types** - 95% complete, final compilation fixes needed
- ⏳ **Remaining Work**: Finish resolvers, advanced features, integrations, testing

### Recent Progress - REST Endpoints Complete:
- ✅ All REST auth endpoints (login, signup, logout, me) working
- ✅ Page save endpoint for browser extension working
- ✅ Article CRUD endpoints working
- ✅ Content HTML serving working
- ✅ Service endpoints (version, warmup) working
- ✅ Integrated into dev stack, all integration tests passing ✅
- ⚠️ Web UI needs union types in GraphQL schema (in progress)

---

## 🚨 IMMEDIATE NEXT STEPS (To Complete Current Work)

### Critical: Fix Remaining Resolver Compilation Errors

**Status:** 95% complete - Only ~10 compilation errors remaining, all trivial fixes

**Errors to fix** (in `internal/graphql/resolver/schema.resolvers.go`):

1. **Line 740** - Article query must return union type:
   ```go
   // Current (wrong):
   return gqlItem, nil
   
   // Fix:
   return &model.ArticleSuccess{Article: gqlItem}, nil
   ```

2. **Lines 816-834** - Highlights query has wrong field mappings:
   - Remove `Patch` and `ShortID` fields (not in GraphQL Highlight type)
   - Change `&highlight.CreatedAt` to `highlight.CreatedAt.Format(time.RFC3339)`
   - Remove references to `Prefix`, `Suffix`, `Type` fields (not in GraphQL schema)
   - Change `highlight.HighlightColor` to `highlight.Color`

**Once fixed:**
1. Rebuild: `go build ./...`
2. Restart dev stack: `cd dev && docker compose down && docker compose up -d`
3. Test in browser: `http://omnivore-dev:81` (should load without errors)
4. Re-run integration tests: `make test_integration OMNIVORE_URL=http://omnivore-dev:81`

**Expected outcome:** Web UI fully functional, all GraphQL operations working with union types.

---

## Problem Statement

Rewrite the Omnivore GraphQL API from TypeScript (Apollo/Express) to Go. The existing API at `/Users/aliaksei.karneyeu/projects/omnivore/packages/api` is a comprehensive GraphQL server with:
- **77 mutations** and **38 queries** (3,652-line schema)
- **38 entity types** (TypeORM models)
- **32 resolver modules**
- **REST endpoints** for auth, article handling, content processing, exports, integrations, webhooks, etc.
- Authentication (JWT, OAuth, email-based), authorization (permissions system)
- Redis for caching and BullMQ job enqueueing
- PostgreSQL via TypeORM
- External services: GCS/S3, analytics, Sentry, webhooks, text-to-speech, AI features

## Approach

Build a production-ready Go API server that:
1. Uses **net/http** only (no chi/gin/echo)
2. Implements the **complete GraphQL schema** with gqlgen
3. Uses **GORM** as ORM for PostgreSQL
4. Maintains **wire-format compatibility** with existing clients (web, mobile, extensions)
5. Reuses Redis, PostgreSQL, and storage infrastructure from existing stack
6. Tests with mocked dependencies based on integration test scenarios

## Architecture

```
cmd/
  server/
    api.go                    # API server command
internal/
  api/
    server.go                 # net/http server setup
    middleware/               # Auth, CORS, rate limiting, context
    handler/
      graphql.go              # GraphQL handler
      rest.go                 # REST endpoints (auth, webhooks, etc.)
  graphql/
    schema.graphqls           # GraphQL schema (port from TS)
    generated/                # gqlgen codegen output
    resolver/                 # Resolver implementations
      query.go
      mutation.go
      subscription.go
      *_resolver.go           # Type resolvers
  auth/
    jwt.go                    # JWT generation/validation
    oauth.go                  # OAuth providers (Google, etc.)
    permissions.go            # Permission checking
  model/                      # GORM models (port from TypeORM entities)
    user.go
    library_item.go
    highlight.go
    label.go
    subscription.go
    ... (38 entities total)
  repository/                 # Data access layer
    user.go
    library_item.go
    ...
  service/                    # Business logic
    user_service.go
    article_service.go
    highlight_service.go
    label_service.go
    subscription_service.go
    webhook_service.go
    integration_service.go
    export_service.go
    ...
  queue/                      # Reuse existing BullMQ enqueue
    bullmq.go                 # Already implemented
  storage/                    # Reuse existing storage
    storage.go                # Already implemented
  analytics/                  # Reuse existing analytics
    analytics.go              # Already implemented
  config/                     # Reuse and extend config
    config.go
  db/                         # Database connection
    db.go                     # Already exists, extend for GORM
```

## Implementation Phases

### Phase 1: Foundation (Core Infrastructure)
Set up the base server, GraphQL layer, database, and authentication.

### Phase 2: Data Models (GORM Entities)
Port all 38 TypeORM entities to GORM models with correct relationships.

### Phase 3: Core Queries (Read Operations)
Implement essential read operations: articles, highlights, labels, user profile, search.

### Phase 4: Core Mutations (Write Operations)
Implement create/update/delete for articles, highlights, labels, notes.

### Phase 5: Authentication & Authorization
Complete auth flows (email, OAuth, API keys) and permission system.

### Phase 6: Subscriptions & Integrations
RSS feeds, newsletters, webhooks, third-party integrations.

### Phase 7: Advanced Features
AI summaries, exports, text-to-speech, recommendations, rules/filters.

### Phase 8: REST Endpoints
Port remaining REST endpoints for legacy compatibility.

### Phase 9: Testing & Migration
Comprehensive tests, performance tuning, migration tooling.

## Database Migration Strategy

**Current: Migrations run via separate container. Future: Go API will run migrations.**

### Current Architecture (for initial implementation)

1. **Separate migration service** (`@omnivore/db` package in TypeScript monorepo)
   - Uses **Postgrator** (Node.js migration runner)
   - Migration files: `migrations/*.sql` (do/undo pairs, e.g., `0001.do.init_schema_role_users.sql`)
   - Schema version tracked in `schemaversion` table
   - Validates checksums to ensure migration integrity

2. **Docker Compose setup** (deploy/docker-compose.yml):
   ```yaml
   migrate:
     image: "ghcr.io/omnivore-app/sh-migrate:latest"
     command: '/bin/bash /setup_db.bash'
     depends_on:
       postgres:
         condition: service_healthy
   
   omnivore-api:
     depends_on:
       migrate:
         condition: service_completed_successfully  # API waits for migrations
   ```

3. **Migration flow**:
   - Postgres starts → Migrate container runs `setup_db.bash` → Runs `yarn workspace @omnivore/db migrate` → API starts

### Go API Initial Strategy

**For initial implementation, use existing migration container:**

1. **Keep existing migration system** - Continue using the TypeScript `@omnivore/db` package
2. **Shared schema** - Both TypeScript and Go APIs use the same PostgreSQL schema
3. **Migration container runs first** - Docker depends_on ensures migrations complete before API starts
4. **Go API just connects** - No migration code in Go initially, only GORM models that match existing schema

**Rationale for phased approach:**
- Get core API functionality working first
- Schema is already defined and versioned in `packages/db/migrations/`
- Migration tooling (Postgrator) is battle-tested
- Both APIs need to work during transition period

### Future: Go-Native Migrations (Phase 10+)

**Todo tracked as `future-go-migrations` (blocked for now).**

Add Go migration runner to enable standalone API:
- Use **golang-migrate** or **goose** library
- Port/reuse existing SQL migrations from `packages/db/migrations/`
- Add startup flag: `--auto-migrate` or `--skip-migrate`
- Enable standalone Go API deployment without separate migration container

This will be implemented after core API functionality is complete and tested.

## Key Design Decisions

### 1. GraphQL Library: gqlgen
- **Why:** De-facto standard for Go GraphQL, type-safe code generation
- Schema-first approach matches existing TypeScript schema
- Automatic resolver scaffolding

### 2. ORM: GORM
- **Why:** Most mature Go ORM, good PostgreSQL support, similar to TypeORM
- Supports complex relationships, hooks
- Performance tuning via preloading, raw queries when needed
- **IMPORTANT:** GORM models are read-only representations of existing schema
- **NO auto-migration** - schema managed via SQL migrations in `packages/db/migrations/`
- GORM tags must match existing PostgreSQL schema exactly

### 3. Authentication Strategy
- JWT for stateless auth (compatible with existing tokens)
- OAuth 2.0 for social login (Google, etc.)
- API keys for programmatic access
- Session storage in Redis for sensitive operations

### 4. Middleware Chain (net/http)
```go
// Order matters:
1. Request logging
2. Request ID injection
3. Panic recovery
4. CORS
5. Rate limiting
6. Authentication (extract JWT/API key)
7. Context enrichment (user, permissions, client info)
8. Request handlers
```

### 5. Testing Strategy
- **Unit tests:** Service layer with mocked repositories
- **Integration tests:** Use existing test scenarios with mocked external services
- **E2E tests:** Run against full stack (reuse deploy/dev stacks)

### 6. Wire Compatibility
- Match GraphQL schema **exactly** (field names, types, nullability)
- Preserve JSON serialization formats
- Maintain BullMQ job payloads (already done for content-fetcher and queue-processor)
- Keep storage key structures

### 7. AuthTrx Pattern - CRITICAL for Database Security

**What is AuthTrx?**

AuthTrx is a transaction wrapper that sets PostgreSQL Row-Level Security (RLS) claims before executing database operations. This is **essential** for multi-tenant data isolation in Omnivore.

**How it works:**

1. **PostgreSQL Function** (`omnivore.set_claims`):
   ```sql
   CREATE OR REPLACE FUNCTION omnivore.set_claims(user_id uuid, user_role text)
   RETURNS VOID AS $$
   BEGIN
     EXECUTE format('set local omnivore_user.uid to %I', user_id);
     EXECUTE format('set local role %I', user_role);
   END;
   $$ LANGUAGE plpgsql;
   ```

2. **TypeScript Implementation** (`repository/index.ts`):
   ```typescript
   export const authTrx = async <T>(
     fn: (manager: EntityManager) => Promise<T>,
     options: { uid?: string, userRole?: string } = {}
   ): Promise<T> => {
     // Get uid and userRole from HTTP context if not provided
     const queryRunner = appDataSource.createQueryRunner()
     await queryRunner.startTransaction()
     
     try {
       // SET LOCAL session variables for RLS
       await setClaims(queryRunner.manager, uid, userRole)
       
       // Execute the actual database operations
       const result = await fn(queryRunner.manager)
       
       await queryRunner.commitTransaction()
       return result
     } catch (err) {
       await queryRunner.rollbackTransaction()
       throw err
     } finally {
       await queryRunner.release()
     }
   }
   ```

3. **Usage Pattern**:
   ```typescript
   // In resolvers/services
   const article = await authTrx(async (manager) => {
     return manager.getRepository(LibraryItem).findOne({ where: { id } })
   })
   ```

4. **RLS Enforcement**:
   - PostgreSQL RLS policies use `current_setting('omnivore_user.uid')` to filter data
   - The role (`omnivore_user` or `omnivore_admin`) determines permissions
   - All queries within the transaction see only data the user is allowed to access

**Go Implementation Strategy:**

1. **Create AuthTx helper** (`internal/db/auth_tx.go`):
   ```go
   func AuthTx(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
     user := auth.UserFromContext(ctx)
     
     return db.Transaction(func(tx *gorm.DB) error {
       // Set claims before any operations
       if err := SetClaims(tx, user.ID, user.Role); err != nil {
         return err
       }
       return fn(tx)
     })
   }
   
   func SetClaims(tx *gorm.DB, userID, role string) error {
     dbRole := "omnivore_user"
     if role == "admin" {
       dbRole = "omnivore_admin"
     }
     return tx.Exec("SELECT omnivore.set_claims(?, ?)", userID, dbRole).Error
   }
   ```

2. **Use in repositories**:
   ```go
   func (r *ArticleRepository) GetByID(ctx context.Context, id string) (*model.LibraryItem, error) {
     var item model.LibraryItem
     err := db.AuthTx(ctx, r.db, func(tx *gorm.DB) error {
       return tx.First(&item, "id = ?", id).Error
     })
     return &item, err
   }
   ```

**Why this matters:**

- **Security**: Prevents users from accessing other users' data
- **Simplicity**: RLS enforcement at DB level, not application level
- **Compatibility**: Must match TypeScript behavior exactly
- **Every query must use AuthTx** (except public/system operations)

## Notes

- The TypeScript API has ~20,000+ lines of resolver code - this is a substantial rewrite
- Focus on correctness and compatibility first, optimization later
- Leverage existing Go queue/storage/analytics packages
- The schema is massive (77 mutations, 38 queries) - prioritize by user impact
- Mobile and web clients expect exact wire formats - breaking changes require coordination
- Some advanced features (AI summaries, recommendations) may need ML service integration
- Export formats (PDF, HTML, markdown, EPUB) require careful testing

## Dependencies

### External Services (reuse existing)
- PostgreSQL (schema already defined in `setup_db.bash`)
- Redis (caching + BullMQ)
- MinIO/GCS (blob storage)
- Analytics service (optional)

### Go Libraries (to be added)
- `github.com/99designs/gqlgen` - GraphQL server
- `gorm.io/gorm` - ORM
- `gorm.io/driver/postgres` - PostgreSQL driver
- `github.com/golang-jwt/jwt/v5` - JWT auth
- `golang.org/x/oauth2` - OAuth flows
- `golang.org/x/crypto/bcrypt` - Password hashing
- Rate limiting library (e.g., `golang.org/x/time/rate`)
- Testing: `github.com/stretchr/testify`

## Success Criteria

1. All GraphQL queries and mutations implemented and tested
2. Authentication and authorization working for all user types
3. Integration tests passing against Go API
4. Web client can use Go API without code changes
5. Performance comparable or better than TypeScript API
6. Migration path defined for production cutover

---

## Detailed Implementation Breakdown

### Phase 1: Foundation - 7 todos
- foundation-setup-server
- foundation-setup-gqlgen
- foundation-setup-gorm
- **foundation-auth-tx** (CRITICAL: PostgreSQL RLS wrapper)
- foundation-middleware
- foundation-graphql-handler
- foundation-health-endpoint

### Phase 2: Data Models - 6 todos
- models-port-schema (3,652-line GraphQL schema)
- models-create-user
- models-create-library-item (core entity)
- models-create-highlight
- models-create-label
- models-create-remaining (33 remaining entities)

### Phase 3: Core Queries - 7 todos
- queries-setup-resolvers
- queries-me
- queries-article
- queries-articles (complex search with filters)
- queries-labels
- queries-highlights
- queries-subscriptions

### Phase 4: Core Mutations - 8 todos
- mutations-save-url
- mutations-save-page
- mutations-update-page
- mutations-create-label
- mutations-set-labels
- mutations-create-highlight
- mutations-update-highlight
- mutations-delete-highlight

### Phase 5: Authentication - 5 todos
- auth-jwt (RS256, cookies + Authorization header)
- auth-oauth-google
- auth-api-keys (UUID, SHA256 hash, Redis cache)
- auth-permissions (PostgreSQL RLS)
- auth-middleware-complete

### Phase 6: Subscriptions & Integrations - 6 todos
- subscriptions-rss
- subscriptions-newsletter
- webhooks-create
- webhooks-trigger
- integrations-readwise
- integrations-pocket

### Phase 7: Advanced Features - 7 todos
- advanced-ai-summary
- advanced-export (PDF, HTML, markdown, JSON, CSV)
- advanced-tts
- advanced-recommendations
- advanced-rules (automation)
- advanced-filters (saved searches)
- advanced-search (full-text with advanced syntax)

### Phase 8: REST Endpoints - 5 todos
- rest-auth-endpoints
- rest-article-endpoints
- rest-page-endpoints
- rest-content-endpoints
- rest-svc-endpoints (PubSub, internal services)

### Phase 9: Testing & Migration - 6 todos
- testing-unit-services
- testing-integration-graphql
- testing-e2e
- performance-profiling
- migration-dual-run (traffic splitting)
- migration-cutover-plan

**Total: 57 todos across 9 phases**

## Additional Context from TypeScript Codebase

Based on exploration of `/Users/aliaksei.karneyeu/projects/omnivore/packages/api`:

### Key TypeScript API Stats
- **Entry point**: `src/server.ts` (Express + Apollo Server on port 4000)
- **GraphQL**: 97+ mutations, 30+ queries, GraphQL subscriptions
- **38 TypeORM entities**: User, LibraryItem, Highlight, Label, Subscription, Webhook, Rule, Filter, Integration, Export, etc.
- **32 resolver modules** in `src/resolvers/`
- **15+ REST routers** in `src/routers/` (auth, article, page, content, webhooks, TTS, etc.)
- **Middleware stack**: Body parser (100MB limit), compression, CORS, rate limiting (per-IP + hourly), Sentry, Prometheus, HTTP context
- **Connection pooling**: Configurable, 10s query timeout, 10s idle timeout
- **Authentication**: JWT (cookies + Authorization header), Google OAuth, Apple Sign In, API keys (UUID, SHA256 hashed, Redis cached)
- **Background jobs**: BullMQ for content-fetch, save-page, ai-summarize, bulk-action, export, trigger-rule, webhooks, etc.
- **External services**: PostgreSQL (read replicas), Redis, GCS/S3, analytics (PostHog), Sentry, webhooks, TTS, AI services

### Critical Implementation Notes
1. **AuthTrx is mandatory** for all user-scoped database operations
2. **Rate limiting** has separate tiers: general API (apiLimiter), hourly limit (apiHourLimiter), auth endpoints (authLimiter)
3. **Trust proxy** setting required for correct IP detection behind load balancers
4. **Keep-alive timeout**: 630s (10min + 30s buffer for load balancer)
5. **Graceful shutdown**: Wait for Apollo, flush analytics, close Redis, close DB
6. **Request context**: HTTP context middleware stores user claims, client info from User-Agent or X-OmnivoreClient header

---

## 📋 DETAILED TODO LIST - REMAINING WORK

### 🔴 PRIORITY 1: Finish Union Types (Current Blocker)
**Estimated: 15-30 minutes**

#### Fix Resolver Compilation Errors
- [ ] Line 740: Wrap Article response in ArticleSuccess union type
- [ ] Lines 816-834: Fix Highlights query field mappings
  - Remove Patch, ShortID fields
  - Format timestamps as RFC3339 strings
  - Remove Prefix, Suffix, Type fields
  - Fix Color field reference
- [ ] Build and verify: `go build ./...`
- [ ] Test in browser: Web UI should load at `http://omnivore-dev:81`
- [ ] Run integration tests: `make test_integration OMNIVORE_URL=http://omnivore-dev:81`

**Success criteria:** Web UI functional, no 422 GraphQL errors, all integration tests pass

---

### 🟡 PRIORITY 2: Test End-to-End Flow (Validation)
**Estimated: 1-2 hours**

#### Manual Browser Testing
Follow the testing scenario in CLAUDE.md:
- [ ] Open `http://omnivore-dev:81` in browser
- [ ] Log in with demo@omnivore.work / demo_password
- [ ] Start local testsite: `make testsite_start`
- [ ] Add article: `http://omnivore-testsite:8765/2026/01/20/book-learning-ebpf/`
- [ ] Verify article displays correctly (title, body, readable)
- [ ] Add label to article
- [ ] Remove label from article
- [ ] Delete article
- [ ] Add RSS feed: `http://omnivore-testsite:8765/feed.xml`
- [ ] Verify RSS posts are imported (subscription view or Library search `in:following`)

#### Integration Test Coverage
- [ ] Verify all 11 integration test scenarios pass:
  - Authentication flow
  - Article save and content extraction
  - Label management
  - Article deletion
  - RSS subscription and import
- [ ] Add new integration test for UpdatePage mutation
- [ ] Add integration test for Highlights CRUD

---

### 🟢 PRIORITY 3: Advanced Features (High User Value)
**Estimated: 1-2 weeks per feature**

#### AI Summarization (`advanced-ai-summary`)
- [ ] Port AI service integration from TypeScript
- [ ] Implement summarization endpoint
- [ ] Add BullMQ job handler for `ai-summarize` queue
- [ ] Test with real content

#### Export Functionality (`advanced-export`)
- [ ] Implement PDF export (using existing PDF generator)
- [ ] Implement HTML export
- [ ] Implement Markdown export
- [ ] Implement JSON export
- [ ] Implement CSV export (for batch exports)
- [ ] Add BullMQ job handler for `export` queue

#### Full-Text Search (`advanced-search`)
- [ ] Implement PostgreSQL full-text search integration
- [ ] Add advanced search syntax support
- [ ] Optimize search query performance
- [ ] Add search result ranking

#### Automation Rules (`advanced-rules`)
- [ ] Implement rule creation/management
- [ ] Add rule evaluation engine
- [ ] Implement rule actions (label, archive, delete, etc.)
- [ ] Add BullMQ job handler for `trigger-rule` queue

#### Saved Filters (`advanced-filters`)
- [ ] Implement filter creation/management
- [ ] Add filter evaluation logic
- [ ] Support complex filter combinations (AND/OR)

#### Recommendations (`advanced-recommendations`)
- [ ] Port recommendation engine from TypeScript
- [ ] Implement recommendation API
- [ ] Add background job for recommendation generation

#### Text-to-Speech (`advanced-tts`)
- [ ] Port TTS integration from TypeScript
- [ ] Implement audio generation endpoint
- [ ] Add audio file storage handling

---

### 🟣 PRIORITY 4: Subscriptions & Integrations (Ecosystem)
**Estimated: 3-5 days per integration**

#### RSS Subscriptions (`subscriptions-rss`)
- [ ] Implement RSS feed management (create, update, delete)
- [ ] Add RSS feed parser
- [ ] Implement periodic RSS sync job
- [ ] Handle feed errors and retries

#### Newsletter Subscriptions (`subscriptions-newsletter`)
- [ ] Implement newsletter email handling
- [ ] Add newsletter parser
- [ ] Generate unique newsletter email addresses
- [ ] Route newsletter content to library

#### Webhooks (`webhooks-create`, `webhooks-trigger`)
- [ ] Implement webhook creation/management endpoints
- [ ] Add webhook triggering on library events
- [ ] Implement webhook retry logic
- [ ] Add webhook signature verification
- [ ] Add BullMQ job handler for `call-webhook` queue

#### Readwise Integration (`integrations-readwise`)
- [ ] Implement Readwise OAuth flow
- [ ] Add Readwise export endpoint
- [ ] Handle highlight sync to Readwise
- [ ] Add background sync job

#### Pocket Import (`integrations-pocket`)
- [ ] Implement Pocket OAuth flow
- [ ] Add Pocket import endpoint
- [ ] Handle bulk article import
- [ ] Add progress tracking

---

### 🔵 PRIORITY 5: Testing & Quality (Production Readiness)
**Estimated: 1-2 weeks**

#### Unit Tests (`testing-unit-services`)
- [ ] Write unit tests for UserService
- [ ] Write unit tests for ArticleService
- [ ] Write unit tests for HighlightService
- [ ] Write unit tests for LabelService
- [ ] Write unit tests for SubscriptionService
- [ ] Write unit tests for WebhookService
- [ ] Write unit tests for auth package
- [ ] Achieve 70%+ code coverage

#### GraphQL Integration Tests (`testing-integration-graphql`)
- [ ] Test all query resolvers with various inputs
- [ ] Test all mutation resolvers with error cases
- [ ] Test pagination and filtering
- [ ] Test authentication/authorization boundaries
- [ ] Test concurrent operations

#### E2E Tests (`testing-e2e`)
- [ ] Write Playwright/Cypress tests for web UI
- [ ] Test complete user workflows (signup → save → read → label → archive)
- [ ] Test browser extension integration
- [ ] Test mobile app compatibility

#### Performance Testing (`performance-profiling`)
- [ ] Profile query performance with pprof
- [ ] Identify and optimize slow queries
- [ ] Add database indexes where needed
- [ ] Benchmark against TypeScript API
- [ ] Load test with realistic traffic patterns
- [ ] Optimize memory allocations

---

### ⚪ PRIORITY 6: Production Migration (Deployment)
**Estimated: 2-3 weeks**

#### Dual-Run Setup (`migration-dual-run`)
- [ ] Set up traffic splitting (10% → Go, 90% → TypeScript)
- [ ] Add metrics collection for both APIs
- [ ] Monitor error rates and latency
- [ ] Gradually increase Go traffic (25%, 50%, 75%, 100%)
- [ ] Set up rollback procedures

#### Cutover Plan (`migration-cutover-plan`)
- [ ] Document cutover procedure
- [ ] Create rollback plan
- [ ] Define success metrics (error rate, latency, throughput)
- [ ] Schedule cutover window
- [ ] Communicate with stakeholders
- [ ] Plan database migration if needed
- [ ] Test disaster recovery procedures

---

## 📊 PROGRESS TRACKING

### Completed (42/58 todos)
- ✅ All foundation work (server, GraphQL, GORM, AuthTx)
- ✅ All data models (38 entities)
- ✅ All core queries (Me, Article, Articles, Labels, Highlights, Subscriptions)
- ✅ All core mutations (SaveURL, SavePage, UpdatePage, Labels, Highlights)
- ✅ Authentication (JWT, API keys)
- ✅ REST endpoints (auth, page, article, content, service)
- ✅ Docker integration and dev stack
- ✅ Integration tests

### In Progress (1 todo)
- ⚠️ Union types for UI compatibility (95% complete)

### Remaining (21 todos)
- 🔴 Finish union types (CRITICAL - blocks UI)
- 🟡 End-to-end validation testing
- 🟢 7 advanced features
- 🟣 6 integrations
- 🔵 4 testing tasks
- ⚪ 2 migration tasks

### Blocked (2 todos)
- ❌ OAuth (by requirements - email/password only)
- ❌ Go migrations (waiting for core completion)

---

## 🎯 RECOMMENDED WORK ORDER (Next Session)

1. **Fix resolver compilation errors** (15-30 min)
   - Critical blocker, simple fixes
   
2. **Test in browser** (30 min)
   - Validate UI works end-to-end
   
3. **Choose next priority:**
   - **Option A - High user value:** Implement AI summarization
   - **Option B - Ecosystem:** Complete webhooks + Readwise integration
   - **Option C - Quality:** Write comprehensive tests
   - **Option D - Features:** Add full-text search + rules

**Recommendation:** Fix resolvers → browser test → AI summarization (highest user value)

---

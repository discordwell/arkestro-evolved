# Project Audit: Arkestro-Evolved / Evochain

Date: March 14, 2026

This audit covers three phases:

1. Understanding the current state of the project
2. Competitive analysis of the procurement / sourcing landscape
3. A build decision based on both

## Phase 1: Current State

### Executive Summary

This repository is a compact, polished demo of an AI-themed sourcing product, not a market-ready procurement application.

What it is:

- A single-binary Go web app with server-rendered HTML and SQLite storage
- Roughly 4.4k lines across Go, HTML, and CSS
- A local-first sourcing-event simulator with deterministic pricing heuristics
- A credible demo for line items, suppliers, quote rounds, awards, and replay/backtest concepts

What it is not:

- A multi-user SaaS product
- A secure production procurement system
- A real AI or model-backed negotiation platform
- A source-to-pay suite, orchestration layer, or enterprise integration product

Commit history also makes the intent clear: the entire repo was created in one burst on February 11, 2026 and rebranded to Evochain the same day.

### What Exists Today

The product scope is narrow but coherent:

- Supplier management with tags, risk score, and performance score
- Event creation and status changes
- Line items with category, quantity, baseline, modeled target, and currency
- Quote capture across rounds
- Lowest-quote and weighted best-value awards
- CSV replay import for historical sourcing rounds
- Quote feature extraction
- Copilot decisioning: `award_now`, `counter_at`, `add_supplier`
- Backtest views comparing modeled recommendation vs actual outcome
- JSON endpoints for event copilot and backtest data
- Docker packaging and optional base-path hosting

The core implementation is concentrated in:

- `internal/httpui/ui.go`
- `internal/httpui/copilot.go`
- `internal/copilot/engine.go`
- `internal/store/store.go`

### Architecture

Current architecture is intentionally small:

- UI: Go `html/template`
- App server: `net/http`
- Persistence: SQLite via `modernc.org/sqlite`
- Static assets: embedded CSS/templates
- Packaging: Dockerfile builds a single Linux binary

There is no frontend framework, no external API client layer, no job queue, no message bus, no auth subsystem, no integration adapter layer, and no background processing.

### Product Reality Check

The app looks more advanced than it is because the UI and terminology are polished. Underneath, the "AI" is mostly deterministic heuristics:

- Baseline modeling is hash-based
- Target pricing uses a simple discount formula based on baseline, quantity, and supplier count
- Copilot decisions are score-based heuristics over quote gaps, slopes, volatility, risk, and performance

That is acceptable for a demo, but it means the current product has no defensible model, no learning loop, and no real enterprise data advantage.

### Engineering Maturity

The repo is healthy for a demo:

- `go test ./...` passes
- Dockerfile is present
- Happy-path coverage exists across app, store, and copilot behavior

But maturity is still early:

- `internal/httpui` coverage is only 28.4%
- `internal/store` coverage is 46.5%
- `internal/predict` has no direct tests
- `cmd/arkessro` has no tests
- There is no operational telemetry, metrics, or structured audit logging
- There is no data migration framework beyond manual column checks

### Strategic Gaps

Against any real procurement product, the repo is missing almost every table stake:

- No users, orgs, roles, or permissions
- No authentication or SSO
- No tenant isolation
- No approvals or workflow engine
- No attachments, contracts, policy engine, or intake layer
- No supplier onboarding
- No ERP / P2P / CLM / GRC integrations
- No messaging, portal, or supplier collaboration workflow
- No explainability or model governance beyond surface badges
- No compliance posture for enterprise procurement

### Concrete Findings

#### 1. Replay import is exposed in the UI but appears broken in the HTTP path

The event page posts replay files as `multipart/form-data`, but `handleEventPost` only calls `r.ParseForm()` before switching on `action`. For multipart requests, this means the hidden `action=import_replay_csv` field is not reliably available before `FormFile` access, and in local verification the import route returned `unknown action`.

Relevant code:

- `web/templates/event.html` lines 166-177
- `internal/httpui/ui.go` lines 391-402 and 446-462

This is important because replay import is one of the few features that feels strategically differentiating.

#### 2. `GET /events/:id` mutates persistent state

Viewing an event can write modeled baselines and updated targets into the database:

- `internal/httpui/ui.go` lines 274-293

That is convenient for a demo, but it is bad product behavior for a system that may later need auditability, reproducibility, or user trust.

#### 3. The app has no auth, no multitenancy, and public JSON endpoints

All UI and API routes are registered directly without any auth or tenancy layer:

- `internal/httpui/ui.go` lines 71-103
- `internal/httpui/ui.go` lines 612-690

As written, this is a single shared dataset behind an unauthenticated web UI.

#### 4. Data model is too thin for procurement workflow

The schema only covers suppliers, events, line items, quotes, and awards:

- `internal/store/store.go` lines 23-78

That is enough for a demo, but not enough for serious intake, sourcing governance, sourcing collaboration, or enterprise reporting.

#### 5. The deployed public hostname may not currently be healthy

Local probes on March 14, 2026 showed:

- `http://evochain.cornerstonesaas.com` returns a `308` redirect to HTTPS
- `https://evochain.cornerstonesaas.com` failed TLS negotiation from both `curl` and `openssl s_client`

That suggests the public deployment surface may be misconfigured even before product-level concerns are addressed.

### Bottom Line for Phase 1

This project is a strong concept demo and a weak product foundation.

Its best current asset is not "procurement orchestration." It is the replay/backtest framing around sourcing decisions.

## Phase 2: Competitive Analysis

### Market Direction

The procurement market has moved beyond narrow "predictive sourcing" stories.

The winning narrative in 2025-2026 is:

- Agentic procurement orchestration
- Autonomous sourcing and negotiations
- AI-native source-to-pay platforms
- Deep integration into ERP / P2P / contract / risk stacks
- Human-in-the-loop automation with enterprise controls

In other words, vendors are not selling a quoting simulator. They are selling embedded automation across the operating system of procurement.

### Competitive Map

#### Arkestro

Closest conceptual neighbor.

Current positioning:

- Predictive Procurement Platform / PPO
- Supplier recommendations
- Real-time supplier collaboration
- Dynamic award modeling
- New "Arkestro Labs" and "Opportunities" capabilities
- Stronger category-specific story in logistics and enterprise direct procurement

Implication:

If this repo continues as a general "predictive procurement" product, it runs directly at a much more mature incumbent in the same narrative lane.

Sources:

- https://arkestro.com/predictive-procurement/
- https://arkestro.com/press-releases/arkestro-earns-two-prestigious-2024-future-of-sourcing-awards/
- https://arkestro.com/press-releases/arkestro-announces-arkestro-labs-powerful-new-procurement-ai-offerings-at-optimal-25/

#### Keelvar

Competes on sourcing optimization plus autonomous sourcing.

Current positioning:

- Autonomous Sourcing handling up to 100% of tactical event workload
- Recorded bids and negotiations
- Pattern recognition over prior events
- Kai AI assistant for event generation, category assignment, spend forecasting, and sourcing strategy setup

Implication:

Keelvar owns the "AI helps run sourcing events" lane more credibly than this repo does today.

Sources:

- https://www.keelvar.com/documents/what-is-autonomous-sourcing
- https://support.keelvar.com/hc/en-us/articles/23002616054940-Kai

#### Fairmarkit

Competes on autonomous sourcing, especially for repetitive and tail spend.

Current positioning:

- Requisition-to-PO workflow automation
- Supplier matching and bid aggregation
- Autonomous sourcing for repetitive spend
- Explicit source-to-pay integration story
- Public March 10, 2026 integration announcement with Zip

Implication:

Fairmarkit is already occupying the "turn repetitive sourcing into automated workflow" position and is connecting it to orchestration platforms.

Sources:

- https://www.fairmarkit.com/autonomous-sourcing
- https://www.fairmarkit.com/blog/fairmarkit-partners-with-zip-to-power-source-to-pay-with-autonomous-sourcing

#### Zip

Zip now defines the orchestration category around procurement intake and AI agents.

Current positioning:

- Agentic procurement orchestration
- 50+ procurement AI agents
- AI-powered front door / intake layer
- Workflow engine and integration ecosystem
- Strong scale narrative: customers, supplier network, AI insights

Implication:

If Evochain tries to become a front door, workflow product, or orchestration layer, it is entering Zip's strongest category with a codebase that has none of the required foundations.

Sources:

- https://ziphq.com/about
- https://ziphq.com/blog/introducing-agentic-procurement-orchestration
- https://ziphq.com/ai
- https://ziphq.com/products/procurement-concierge
- https://ziphq.com/capabilities/workflow-engine

#### Levelpath

Represents the AI-native procurement platform wave.

Current positioning:

- Intake, sourcing, contracts, and project pipeline on one AI-native platform
- AI agents
- Integration layer
- Permissioning
- Mobile access

Implication:

Modern challengers are not stopping at sourcing recommendations. They are spanning procurement workflows end-to-end.

Sources:

- https://www.levelpath.com/
- https://www.levelpath.com/platform

#### Pactum

Owns the autonomous negotiation wedge.

Current positioning:

- AI agents that execute supplier negotiations at scale
- Payment terms, discounts, price lists, and post-sourcing negotiation flows
- Embedded into enterprise systems

Implication:

If the bet is negotiation automation specifically, Pactum is the direct benchmark, and the current repo is nowhere near that level of specialization or execution depth.

Sources:

- https://pactum.com/

#### Incumbents: SAP, Coupa, JAGGAER

Large suites are also repositioning around AI-native and agentic language.

Current positioning examples:

- SAP: next-gen SAP Ariba as an AI-native source-to-pay suite with front door, assistants, sourcing, supplier management, and invoice automation
- Coupa: Coupa AI and Coupa Navi across spend management, sourcing, procurement, invoicing, payments, and supply chain with community-scale data
- JAGGAER: JAI as a procurement-native orchestrator of AI agents across sourcing, contracting, supplier management, planning, risk, and compliance

Implication:

Even the legacy platform layer is moving aggressively. A small standalone product must be sharply focused to avoid getting flattened.

Sources:

- https://www.sap.com/products/spend-management/smart-source-to-pay-procurement-software.html
- https://www.coupa.com/platform/ai/
- https://www.jaggaer.com/solutions/jai

### What the Market Says Indirectly

The competitor set reveals several truths:

1. Orchestration is becoming the control layer
2. Sourcing is becoming automated and embedded
3. Negotiation is becoming agentic
4. Procurement AI is being sold with integrations, governance, and scale
5. Point products need a wedge that is clearly narrower and sharper than "AI procurement"

### Where This Repo Actually Has White Space

The current repo has one angle that still feels interesting:

- Historical replay and backtest of sourcing decisions

Most competitors market automation and outcomes. Fewer products visibly start by helping a team ingest past sourcing rounds, simulate alternatives, and prove where strategy, supplier mix, or negotiation timing would have changed outcomes.

That is not enough by itself yet. But it is the best raw material in this codebase.

## Phase 3: What It Makes Sense to Build

### Decision

Do not build this into a general procurement platform.

Do not build a Zip competitor.

Do not build a Fairmarkit clone.

Do not build a full AI sourcing suite.

Build this as a **sourcing replay and decision intelligence workbench** that plugs into existing procurement systems.

### Product Thesis

The product should help procurement teams answer:

- Which past sourcing events underperformed?
- Where did we leave savings on the table?
- Which suppliers should have been invited earlier?
- When should we have awarded versus countered?
- Which categories or lanes are most suitable for automation?
- What policy or negotiation strategy changes would improve outcomes next quarter?

That is a better fit for the current codebase and a better market position.

### Why This Direction Fits the Repo

The repo already has the beginnings of the right primitives:

- Event replay import
- Quote-by-round history
- Supplier-scoring concepts
- Line-item recommendations
- Backtesting
- Best-value weighting

Those are analytics / simulation / decision-support primitives, not orchestration primitives.

Trying to turn them into an execution suite would mean rebuilding the entire product category from scratch.

### Recommended Product Shape

Position it as:

- a sourcing intelligence layer
- a procurement strategy simulator
- or a negotiation replay lab

for teams already running SAP Ariba, Coupa, Zip, Fairmarkit, JAGGAER, or manual spreadsheet-based sourcing.

### What to Build First

#### Stage 1: Make the current differentiator real

Goal: turn the demo into a trustworthy replay product.

Build:

- Working, robust imports from CSV plus one real system export format
- Authentication, orgs, and saved workspaces
- Immutable event snapshots for replay
- Explainable decision traces for every recommendation
- Better backtest reporting: expected vs actual, supplier invite counterfactuals, award timing counterfactuals, confidence calibration
- Category and supplier benchmark views
- Executive summary export for procurement leaders

Do not build:

- full supplier portal
- contract lifecycle management
- intake workflow engine
- invoice / payment automation

#### Stage 2: Become an overlay on top of existing systems

Goal: fit into how procurement teams already operate.

Build:

- Connectors for common exports and APIs from SAP Ariba, Coupa, Zip, Fairmarkit, and generic ERP/P2P data
- Scheduled ingest of sourcing events
- Portfolio view across categories
- "opportunity scoring" for which future events deserve human attention vs automation
- Human-review workflow for recommendation approval

This is where the product becomes strategically useful.

#### Stage 3: Add constrained execution, not full orchestration

Goal: close the loop carefully.

Build:

- Suggested supplier invite lists
- Suggested negotiation targets and counter ranges
- Draft supplier communication artifacts
- Approval-ready recommended awards
- Optional push-back into an external system after human signoff

Avoid owning the full workflow if another system already does.

### What Not to Build

Unless there is a very large, explicit commitment to this project, avoid:

- full source-to-pay
- generalized intake-to-pay
- workflow/orchestration as the primary wedge
- broad agent marketplace claims
- contract repository
- invoice automation
- payments
- end-user requester experience

Those are already occupied by vendors with far more product depth and enterprise readiness.

### Success Criteria for This Direction

This strategy is working if the product can eventually show:

- measurable savings-leakage detection from historical events
- better award decisions than current manual practice in selected categories
- shorter time for category managers to review a sourcing event
- strong adoption as an overlay rather than a rip-and-replace system
- a credible path to being the "decision layer" inside existing procurement stacks

### Final Recommendation

This project should not remain a dinosaur-themed procurement demo forever, but it also should not try to become a full procurement suite.

The most sensible path is:

1. Fix the replay/backtest foundation
2. Reframe the product around sourcing replay, counterfactual analysis, and decision intelligence
3. Integrate into existing procurement systems instead of replacing them
4. Add narrowly scoped execution only after the analytics wedge is defensible

If there is no appetite to pursue that narrower wedge, the better decision is to archive the project rather than keep expanding a demo into a category where the market has already moved much further than the codebase.

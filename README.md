# Arkessro (oneshot build)

This repo is a clean-room, minimal "predictive procurement" demo app inspired by Arkestro's public product positioning.

It supports:
- Suppliers (with basic profiles: capability tags, risk, performance)
- Sourcing events
- Line items (category + baseline + auto-predicted target price)
- "Smart baselining" (leave baseline blank to model one)
- Supplier quotes (manual or simulated)
- Quote-feature extraction per quote:
  - gap to target / walk-away
  - round-over-round improvement slope
  - supplier risk / performance
  - quote volatility + outlier flags
- Counter-or-Award copilot actions:
  - `award_now`
  - `counter_at_$X`
  - `add_supplier`
- Historical replay CSV import (for real past rounds)
- Backtest view: compare modeled recommendation outcome vs actual outcome
- Best-value award modeling (weights: cost vs risk vs performance)
- One-click award to lowest quote per line item

## Requirements
- Go (tested with `go1.25.x`)

## Quickstart

```bash
make dev
```

Then open: `http://127.0.0.1:8080`

## Demo Flow

1. Add a few suppliers on `/suppliers` and set:
   - capability tags (comma-separated)
   - risk score (0-100)
   - performance score (0-100)
2. Create an event on `/events/new`
3. Add line items:
   - set a category like `metals, aluminum` so supplier recommendations can match
   - leave baseline blank to see modeled baselines
4. Click `Simulate Supplier Round` to generate quotes quickly
5. Tune `Best Value Weights` to see awards shift from "lowest price" toward "best value"
6. Use `Import Historical Replay CSV` on an event to load past quote rounds and awards
7. Review:
   - `Negotiation Guidance` for the copilot decision per line item
   - `Copilot Backtest` for modeled vs actual outcomes on imported rounds

## Replay CSV

Import from the event page (`/events/{id}`) with:
- Required columns: `line_item`, `supplier`, `round`, `unit_price`
- Optional columns: `category`, `quantity`, `unit`, `baseline`, `target`, `supplier_email`, `supplier_tags`, `supplier_risk`, `supplier_performance`, `award`

Sample file:
- `examples/replay_sample.csv`

Example `award` values treated as true:
- `yes`, `true`, `1`, `awarded`

## API

Event-scoped copilot endpoints:
- `GET /api/events/{id}/copilot`
- `GET /api/events/{id}/copilot/backtest`

## Build

```bash
make build
./arkessro -db ./data/dev.db -addr 127.0.0.1:8080
```

Optional subpath hosting (e.g. behind reverse proxy at `/cheapchain`):

```bash
./arkessro -db ./data/dev.db -addr 127.0.0.1:8080 -base-path /cheapchain
```

## Test

```bash
make test
```

## Notes
- Data is stored in `./data/dev.db` by default.
- This is intentionally small and local-first, suitable for a fast "oneshot build" exercise.

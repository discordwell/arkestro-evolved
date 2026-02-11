# Arkessro (oneshot build)

This repo is a clean-room, minimal "predictive procurement" demo app inspired by Arkestro's public product positioning.

It supports:
- Suppliers (with basic profiles: capability tags, risk, performance)
- Sourcing events
- Line items (category + baseline + auto-predicted target price)
- "Smart baselining" (leave baseline blank to model one)
- Supplier quotes (manual or simulated)
- Quote anomaly flags (simple heuristics)
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

## Build

```bash
make build
./arkessro -db ./data/dev.db -addr 127.0.0.1:8080
```

## Test

```bash
make test
```

## Notes
- Data is stored in `./data/dev.db` by default.
- This is intentionally small and local-first, suitable for a fast "oneshot build" exercise.

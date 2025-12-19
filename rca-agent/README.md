# rca-agent

A FastAPI-based root cause analysis agent.

## Setup

```bash
uv sync
```

## Run

```bash
uvicorn src.main:app --reload
```

## Endpoints

- `GET /` - Health check
- `POST /analyze` - Analysis endpoint

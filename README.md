# WebFP Drive — Backend

## Setup

```bash
cp .env.example .env
```

Create the PostgreSQL schema once (tables are created automatically on startup):

```sql
CREATE SCHEMA IF NOT EXISTS webfp;
```

## Run

```bash
go run ./cmd
```

API: http://localhost:8080  
Swagger: http://localhost:8080/swagger/index.html

## Test

```bash
go test ./...
```

# Order Processing System

An event-driven order processing pipeline built with Serv — demonstrates modular architecture, pub/sub messaging, background workers, and async notifications.

## Architecture

```
┌──────────┐     publish      ┌──────────┐     publish      ┌────────────┐
│  API     │ ──────────────→  │  Worker  │ ──────────────→  │  Notifier  │
│ (routes) │  "orders.new"    │(subscribe)│ "notifications"  │(subscribe) │
└──────────┘                  └──────────┘                  └────────────┘
     │                              │                              │
     └──────────────────────────────┴──────────────────────────────┘
                                    │
                              ┌─────┴─────┐
                              │  SQLite   │
                              │  (orders) │
                              └───────────┘
```

## Flow

1. **API** receives `POST /api/orders` with customer + item details
2. Order is saved to DB with status `received`
3. Event `orders.new` is published
4. **Worker** picks up the event, processes payment, marks as `paid`
5. Worker publishes `notifications.send`
6. **Notifier** picks up the event, sends confirmation, updates DB

## Quick Start

```bash
serv run showcase/order-system/main.srv --watch
```

API starts on http://localhost:4000

## Usage

```bash
# Place an order
curl -X POST http://localhost:4000/api/orders \
  -H "Content-Type: application/json" \
  -d '{"customer": "Alice", "item": "widget", "quantity": 3}'

# Watch the logs — you'll see the full pipeline execute:
#   [API] Order #1 created: 3x widget for Alice
#   [Worker] Processing order #1...
#   [Worker] Order #1 paid successfully
#   [Notifier] Sending order_confirmed to Alice: ...
#   [Notifier] ✓ Notification sent for order #1

# Check order status
curl http://localhost:4000/api/orders/1

# View dashboard
curl http://localhost:4000/api/dashboard

# List recent orders
curl http://localhost:4000/api/orders
```

## Project Structure

```
order-system/
├── main.srv              — Server setup, infra declarations, scheduler
├── models/
│   └── order.srv         — Order struct, methods, price catalog
├── handlers/
│   ├── api.srv           — HTTP route handlers (CRUD)
│   ├── worker.srv        — Order processing subscriber
│   └── notifier.srv      — Notification subscriber
├── config.yml            — Runtime configuration
└── README.md
```

## Switching to a Real Broker

The system uses `broker "in-memory"` by default (zero dependencies). To use a real message broker, change one line in `main.srv`:

```serv
// Pick one:
broker "nats://localhost:4222"
broker "kafka://localhost:9092"
broker "amqp://guest:guest@localhost:5672"
```

No other code changes needed — `publish` and `subscribe` work identically across all brokers.

## Features Demonstrated

| Feature | File | Usage |
|---------|------|-------|
| Modular imports | `main.srv` | `import "handlers/api.srv"` |
| Pub/Sub | `api.srv` → `worker.srv` → `notifier.srv` | Event-driven pipeline |
| Database + Migrations | `main.srv`, `api.srv` | SQLite with schema versioning |
| Caching | `api.srv` | 10s TTL on order list |
| Structs & Methods | `models/order.srv` | `Order.summary()`, `getPrice()` |
| Scheduled Jobs | `main.srv` | Retry stuck orders, daily summary |
| JSON parsing | `api.srv`, `worker.srv` | Request/event body parsing |
| F-string interpolation | Throughout | `f"Order #{orderId} created"` |
| Pattern matching | `models/order.srv` | Price catalog via `match` |

## What This Shows

This is the kind of system Serv was designed for: **event-driven microservice architecture** expressed in ~150 lines of declarative code. The infrastructure concerns (server, database, broker, cache) are declared once at the top, and the business logic flows naturally through publish/subscribe.

The equivalent in Go/Java would require: a web framework, a message broker client library, connection pool management, graceful shutdown handling, structured logging setup, and 500+ lines of boilerplate — all of which Serv handles automatically.
